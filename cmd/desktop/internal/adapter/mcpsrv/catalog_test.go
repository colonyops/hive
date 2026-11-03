package mcpsrv_test

import (
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/desktop/internal/adapter/mcpsrv"
	"github.com/colonyops/hive/internal/ops"
	"github.com/colonyops/hive/internal/ops/catalog"
)

// notMigrated lists the tools a server still declares by hand, outside the
// catalog.
var notMigrated = map[string][]string{
	mcpsrv.PathPrefix: {
		"get_status", "read_diagnostics", "list_profiles", "get_flow", "list_feeds", "list_inbox",
		"list_inbox_item_events", "list_item_sessions", "list_sessions", "edit_session_tags", "list_actions", "list_applets", "refresh_sources",
		"list_action_runs", "get_action_run",
		"create_profile", "delete_profile",
		"get_profile_image", "set_profile_image", "clear_profile_image",
		"get_node_image", "set_node_image", "clear_node_image",
		"execute_flow",
		"list_workspaces", "list_schedules", "put_schedule", "remove_schedule",
		"preview_schedule", "schedule_runs",
	},
}

// TestServedToolsAreTheCatalog fails when a catalog operation is not mounted
// on the server its toolset names, or when a server serves a tool the
// catalog does not know.
func TestServedToolsAreTheCatalog(t *testing.T) {
	t.Parallel()
	_, app := testSession(t)
	_, drive := testCanvasSession(t)

	for path, session := range map[string]*mcp.ClientSession{mcpsrv.PathPrefix: app, mcpsrv.DrivePathPrefix: drive} {
		t.Run(path, func(t *testing.T) {
			want := append([]string{}, notMigrated[path]...)
			for _, op := range catalog.All() {
				if info := op.Info(); info.Owner == ops.OwnerDesktop && info.Toolset.MCPPath == path {
					want = append(want, info.ToolName)
				}
			}

			res, err := session.ListTools(t.Context(), nil)
			require.NoError(t, err)
			got := make([]string, 0, len(res.Tools))
			for _, tool := range res.Tools {
				got = append(got, tool.Name)
			}
			assert.ElementsMatch(t, want, got)
		})
	}
}
