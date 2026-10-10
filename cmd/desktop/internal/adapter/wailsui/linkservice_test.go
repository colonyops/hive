package wailsui

import (
	"bytes"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/internal/domain/deeplink"
)

func countLinkOpens(t *testing.T) *int {
	t.Helper()
	var count int
	orig := emitLinkOpen
	emitLinkOpen = func() { count++ }
	t.Cleanup(func() { emitLinkOpen = orig })
	return &count
}

func TestLinkService_HoldsTheLatestLinkUntilTaken(t *testing.T) {
	opens := countLinkOpens(t)
	links := NewLinkService(zerolog.Nop())
	assert.Nil(t, links.Take())

	require.NoError(t, links.deliver("hive://code/first", linkFromOS))
	require.NoError(t, links.deliver("hive://inbox/work/7", linkFromSecondInstance))
	assert.Equal(t, 2, *opens)

	assert.Equal(t, &deeplink.Location{Area: deeplink.AreaInbox, Profile: "work", Item: 7}, links.Take())
	assert.Nil(t, links.Take())
}

func TestUI_OpenLinkToastsALinkThatDoesNotParse(t *testing.T) {
	opens := countLinkOpens(t)
	toasts := make(chan NotificationToast, 1)
	swapEmitNotificationToast(t, toasts)

	ui := &UI{logger: zerolog.Nop(), links: NewLinkService(zerolog.Nop())}
	ui.openLink("hive://flows/work", linkFromOS)

	assert.Zero(t, *opens)
	assert.Nil(t, ui.links.Take())
	toast := <-toasts
	assert.Equal(t, "Could not open link", toast.Title)
	assert.Equal(t, "error", toast.Severity)
}

func TestLinkArg(t *testing.T) {
	link, ok := linkArg([]string{"/usr/local/bin/hive-desktop", "HIVE://code/web"})
	assert.True(t, ok)
	assert.Equal(t, "HIVE://code/web", link)

	_, ok = linkArg([]string{"/usr/local/bin/hive-desktop"})
	assert.False(t, ok)
}

func TestLinkService_LogsEachStep(t *testing.T) {
	countLinkOpens(t)
	var buf bytes.Buffer
	links := NewLinkService(zerolog.New(&buf))

	require.NoError(t, links.deliver("hive://code/web?window=@2", linkFromSecondInstance))
	links.Take()
	require.Error(t, links.deliver("hive://flows/x", linkFromOS))

	out := buf.String()
	assert.Contains(t, out, `"message":"received hive:// link"`)
	assert.Contains(t, out, `"source":"second-instance"`)
	assert.Contains(t, out, `"link":{"area":"code","slug":"web","window":"@2"}`)
	assert.Contains(t, out, `"message":"frontend took hive:// link"`)
	assert.Contains(t, out, `"message":"ignoring hive:// link"`)
	assert.Contains(t, out, `"cmp":"links"`)
}
