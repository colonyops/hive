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

// TestServedToolsAreTheCatalog fails when a catalog operation is not mounted
// on the server its toolset names, or when a server serves a tool the
// catalog does not know.
func TestServedToolsAreTheCatalog(t *testing.T) {
	t.Parallel()
	_, app := testSession(t)
	_, drive := testCanvasSession(t)
	_, chats := testChatsSession(t)

	sessions := map[string]*mcp.ClientSession{mcpsrv.PathPrefix: app, mcpsrv.DrivePathPrefix: drive, mcpsrv.ChatsPathPrefix: chats}
	for path, session := range sessions {
		t.Run(path, func(t *testing.T) {
			var want []string
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
