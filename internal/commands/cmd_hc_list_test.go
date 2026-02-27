package commands

import (
	"bytes"
	"context"
	"testing"

	"github.com/colonyops/hive/internal/core/hc"
	"github.com/colonyops/hive/internal/hive"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v3"
)

type fakeHCListStore struct {
	hc.Store
	filter hc.ListFilter
	items  []hc.Item
}

func (f *fakeHCListStore) ListItems(_ context.Context, filter hc.ListFilter) ([]hc.Item, error) {
	f.filter = filter
	return f.items, nil
}

func TestHCListCmd_SessionFlagWiresToFilter(t *testing.T) {
	store := &fakeHCListStore{items: []hc.Item{{ID: "hc-1234", Title: "Task", Type: hc.ItemTypeTask, Status: hc.StatusOpen}}}
	appSvc := hive.NewHCService(store, nil, zerolog.Nop())

	var buf bytes.Buffer
	cmd := NewHCCmd(&Flags{}, &hive.App{HC: appSvc})
	app := &cli.Command{Name: "hive", Writer: &buf}
	cmd.Register(app)

	err := app.Run(context.Background(), []string{"hive", "hc", "list", "--session", "session-1", "--json"})
	require.NoError(t, err)

	assert.Equal(t, "session-1", store.filter.SessionID)
	assert.Contains(t, buf.String(), `"id":"hc-1234"`)
}
