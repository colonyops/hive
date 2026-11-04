package commands

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/internal/ops"
	"github.com/colonyops/hive/internal/ops/catalog"
	"github.com/colonyops/hive/internal/platform/desktopendpoint"
)

type fakeDesktop struct {
	srv *httptest.Server

	mu    sync.Mutex
	calls map[string][]recordedCall
}

// newFakeDesktop serves one fake tool at /mcp and one at /mcp/drive.
func newFakeDesktop(t *testing.T) *fakeDesktop {
	t.Helper()
	f := &fakeDesktop{calls: map[string][]recordedCall{}}
	mux := http.NewServeMux()
	for path, tool := range map[string]string{"/mcp": "inbox_list", "/mcp/drive": "tab_open"} {
		server := mcp.NewServer(&mcp.Implementation{Name: "fake", Version: "test"}, nil)
		server.AddTool(&mcp.Tool{Name: tool, InputSchema: map[string]any{"type": "object"}},
			func(_ context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				args := map[string]any{}
				require.NoError(t, json.Unmarshal(req.Params.Arguments, &args))
				f.mu.Lock()
				f.calls[path] = append(f.calls[path], recordedCall{Tool: req.Params.Name, Args: args})
				f.mu.Unlock()
				return &mcp.CallToolResult{
					Content:           []mcp.Content{&mcp.TextContent{Text: `{"ok":true}`}},
					StructuredContent: map[string]any{"ok": true, "path": path},
				}, nil
			})
		mux.Handle(path, mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, nil))
	}
	f.srv = httptest.NewServer(mux)
	t.Cleanup(f.srv.Close)
	return f
}

func connectServe(t *testing.T, dataDir string, toolsets ...string) *mcp.ClientSession {
	t.Helper()
	srv, err := newMCPServer(NewDesktopClient(dataDir, "", "test"), "agent-1", toolsets, "test", zerolog.Nop())
	require.NoError(t, err)
	ct, st := mcp.NewInMemoryTransports()
	_, err = srv.Connect(t.Context(), st, nil)
	require.NoError(t, err)
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "agent", Version: "test"}, nil).Connect(t.Context(), ct, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = cs.Close() })
	return cs
}

func TestMCPServeListsSelectedToolsetsWithoutCaller(t *testing.T) {
	want := map[string]ops.Info{}
	for _, op := range catalog.All() {
		if info := op.Info(); info.Toolset.Name == "drive" || info.Toolset.Name == "layout" {
			want[info.ToolName] = info
		}
	}
	require.NotEmpty(t, want)

	cs := connectServe(t, t.TempDir(), "drive", "layout")
	listed, err := cs.ListTools(t.Context(), nil)
	require.NoError(t, err)

	got := map[string]*mcp.Tool{}
	for _, tool := range listed.Tools {
		got[tool.Name] = tool
		raw, err := json.Marshal(tool.InputSchema)
		require.NoError(t, err)
		assert.NotContains(t, string(raw), `"caller"`, tool.Name)
	}
	require.Len(t, got, len(want))
	for name, info := range want {
		require.Contains(t, got, name)
		assert.Equal(t, info.Summary, got[name].Title)
		assert.Equal(t, info.Description(), got[name].Description)
		assert.Equal(t, info.Effect == ops.EffectRead, got[name].Annotations.ReadOnlyHint)
	}
}

func TestMCPServeDefaultsToEveryToolset(t *testing.T) {
	cs := connectServe(t, t.TempDir())
	listed, err := cs.ListTools(t.Context(), nil)
	require.NoError(t, err)

	var desktopOps int
	for _, op := range catalog.All() {
		if op.Info().Owner == ops.OwnerDesktop {
			desktopOps++
		}
	}
	assert.Len(t, listed.Tools, desktopOps)
}

func TestMCPServeForwardsWithCaller(t *testing.T) {
	fake := newFakeDesktop(t)
	dataDir := t.TempDir()
	require.NoError(t, desktopendpoint.Write(dataDir, desktopendpoint.Endpoint{URL: fake.srv.URL, PID: 1}))
	cs := connectServe(t, dataDir, "layout", "inbox")

	res, err := cs.CallTool(t.Context(), &mcp.CallToolParams{
		Name:      "tab_open",
		Arguments: map[string]any{"view": "drive", "caller": "spoofed"},
	})
	require.NoError(t, err)
	require.False(t, res.IsError)
	assert.Equal(t, map[string]any{"ok": true, "path": "/mcp/drive"}, res.StructuredContent)

	res, err = cs.CallTool(t.Context(), &mcp.CallToolParams{Name: "inbox_list", Arguments: map[string]any{}})
	require.NoError(t, err)
	require.False(t, res.IsError)
	assert.Equal(t, map[string]any{"ok": true, "path": "/mcp"}, res.StructuredContent)

	fake.mu.Lock()
	defer fake.mu.Unlock()
	assert.Equal(t, []recordedCall{{Tool: "tab_open", Args: map[string]any{"view": "drive", "caller": "agent-1"}}}, fake.calls["/mcp/drive"])
	assert.Equal(t, []recordedCall{{Tool: "inbox_list", Args: map[string]any{"caller": "agent-1"}}}, fake.calls["/mcp"])
}

func TestMCPServeDesktopNotRunning(t *testing.T) {
	cs := connectServe(t, t.TempDir(), "layout")

	res, err := cs.CallTool(t.Context(), &mcp.CallToolParams{Name: "layout_get", Arguments: map[string]any{}})
	require.NoError(t, err)
	require.True(t, res.IsError)
	require.Len(t, res.Content, 1)
	assert.Equal(t, "unavailable: Hive Desktop is not running", res.Content[0].(*mcp.TextContent).Text)
}

func TestMCPServeRefusedConnectionIsUnavailable(t *testing.T) {
	closed := httptest.NewServer(http.NotFoundHandler())
	url := closed.URL
	closed.Close()
	dataDir := t.TempDir()
	require.NoError(t, desktopendpoint.Write(dataDir, desktopendpoint.Endpoint{URL: url, PID: 1}))
	cs := connectServe(t, dataDir, "layout")

	res, err := cs.CallTool(t.Context(), &mcp.CallToolParams{Name: "layout_get", Arguments: map[string]any{}})
	require.NoError(t, err)
	require.True(t, res.IsError)
	assert.Equal(t, "unavailable: Hive Desktop is not running", res.Content[0].(*mcp.TextContent).Text)
}

func TestMCPServeUnknownToolset(t *testing.T) {
	_, err := newMCPServer(NewDesktopClient(t.TempDir(), "", "test"), "c", []string{"layout", "nope"}, "test", zerolog.Nop())
	require.Error(t, err)
	assert.Contains(t, err.Error(), `unknown toolset "nope"`)
	for _, name := range []string{"inbox", "sessions", "actions", "diagnostics", "flows", "workspaces", "drive", "layout"} {
		assert.Contains(t, err.Error(), name)
	}
}

func TestSplitToolsets(t *testing.T) {
	assert.Equal(t, []string{"drive", "layout", "inbox"}, splitToolsets([]string{"drive, layout", "inbox", ""}))
}
