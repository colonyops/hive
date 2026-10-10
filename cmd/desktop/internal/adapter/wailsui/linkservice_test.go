package wailsui

import (
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
	links := NewLinkService()
	assert.Nil(t, links.Take())

	require.NoError(t, links.deliver("hive://code/first"))
	require.NoError(t, links.deliver("hive://inbox/work/7"))
	assert.Equal(t, 2, *opens)

	assert.Equal(t, &deeplink.Location{Area: deeplink.AreaInbox, Profile: "work", Item: 7}, links.Take())
	assert.Nil(t, links.Take())
}

func TestUI_OpenLinkToastsALinkThatDoesNotParse(t *testing.T) {
	opens := countLinkOpens(t)
	toasts := make(chan NotificationToast, 1)
	swapEmitNotificationToast(t, toasts)

	ui := &UI{logger: zerolog.Nop(), links: NewLinkService()}
	ui.openLink("hive://flows/work")

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
