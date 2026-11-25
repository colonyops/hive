package daemon_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/internal/hive/daemonclient"
)

func toolText(res *mcp.CallToolResult) string {
	var b strings.Builder
	for _, c := range res.Content {
		if text, ok := c.(*mcp.TextContent); ok {
			b.WriteString(text.Text)
		}
	}
	return b.String()
}

func TestDaemonServesEngineOperations(t *testing.T) {
	t.Parallel()
	d := runDaemon(t, t.TempDir(), false)
	client := daemonclient.New(daemonclient.Options{Dialer: daemonclient.UnixDialer(d.socket), Logger: zerolog.Nop(), CheckProtocol: true, ClientVersion: "test"})
	ctx := t.Context()

	created, err := client.CallEngine(ctx, daemonclient.EngineCall{Tool: "task_create", Args: map[string]any{
		"caller": "global", "repo": "acme/site", "title": "Ship", "type": "epic",
		"children": []map[string]any{{"title": "First"}},
	}})
	require.NoError(t, err)
	require.False(t, created.IsError, toolText(created))
	var tree struct {
		Tasks []struct {
			ID string `json:"id"`
		} `json:"tasks"`
	}
	raw, err := json.Marshal(created.StructuredContent)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(raw, &tree))
	require.Len(t, tree.Tasks, 2)

	next, err := client.CallEngine(ctx, daemonclient.EngineCall{Tool: "task_next", Args: map[string]any{"caller": "global", "epicId": tree.Tasks[0].ID}})
	require.NoError(t, err)
	require.False(t, next.IsError, toolText(next))
	assert.Contains(t, toolText(next), `"title":"First"`)

	missing, err := client.CallEngine(ctx, daemonclient.EngineCall{Tool: "task_get", Args: map[string]any{"taskId": "hc-nope"}})
	require.NoError(t, err)
	assert.True(t, missing.IsError)
	assert.True(t, strings.HasPrefix(toolText(missing), "not_found: "), toolText(missing))
}

func TestDaemonLeavesSessionOperationsToSessionService(t *testing.T) {
	t.Parallel()
	d := runDaemon(t, t.TempDir(), false)
	client := daemonclient.New(daemonclient.Options{Dialer: daemonclient.UnixDialer(d.socket), Logger: zerolog.Nop()})

	_, err := client.CallEngine(t.Context(), daemonclient.EngineCall{Tool: "session_list", Args: map[string]any{}})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "session_list")
}
