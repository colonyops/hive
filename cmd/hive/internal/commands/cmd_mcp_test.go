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
	"github.com/colonyops/hive/internal/ops/actions"
	"github.com/colonyops/hive/internal/ops/catalog"
	"github.com/colonyops/hive/internal/ops/layout"
	"github.com/colonyops/hive/internal/ops/mcpops"
	"github.com/colonyops/hive/internal/platform/desktopendpoint"
)

type fakeDesktop struct {
	srv *httptest.Server

	mu    sync.Mutex
	calls map[string][]recordedCall
}

// newFakeDesktop mounts applet_list at /mcp and tab_open at /mcp/drive with
// the catalog specs, so a call validates against the schema the desktop
// enforces.
func newFakeDesktop(t *testing.T) *fakeDesktop {
	t.Helper()
	f := &fakeDesktop{calls: map[string][]recordedCall{}}
	mux := http.NewServeMux()

	root := mcp.NewServer(&mcp.Implementation{Name: "fake", Version: "test"}, nil)
	mountRecorded(t, f, root, "/mcp", actions.AppletList)
	mux.Handle("/mcp", mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return root }, nil))

	drive := mcp.NewServer(&mcp.Implementation{Name: "fake", Version: "test"}, nil)
	mountRecorded(t, f, drive, "/mcp/drive", layout.TabOpen)
	mux.Handle("/mcp/drive", mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return drive }, nil))

	f.srv = httptest.NewServer(mux)
	t.Cleanup(f.srv.Close)
	return f
}

func mountRecorded[In any](t *testing.T, f *fakeDesktop, srv *mcp.Server, path string, spec ops.Spec[In]) {
	t.Helper()
	tool := spec.Info().ToolName
	mcpops.Mount(srv, spec, func(_ context.Context, in In) (map[string]any, error) {
		raw, err := json.Marshal(in)
		require.NoError(t, err)
		args := map[string]any{}
		require.NoError(t, json.Unmarshal(raw, &args))
		f.mu.Lock()
		f.calls[path] = append(f.calls[path], recordedCall{Tool: tool, Args: args})
		f.mu.Unlock()
		return map[string]any{"ok": true, "path": path}, nil
	}, func(err error) error { return err })
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

func TestMCPServeForwardsCallerOnlyWhereDeclared(t *testing.T) {
	fake := newFakeDesktop(t)
	dataDir := t.TempDir()
	require.NoError(t, desktopendpoint.Write(dataDir, desktopendpoint.Endpoint{URL: fake.srv.URL, PID: 1}))
	cs := connectServe(t, dataDir, "layout", "actions")

	res, err := cs.CallTool(t.Context(), &mcp.CallToolParams{
		Name:      "tab_open",
		Arguments: map[string]any{"view": "drive", "caller": "spoofed"},
	})
	require.NoError(t, err)
	require.False(t, res.IsError, "%v", res.Content)
	assert.Equal(t, map[string]any{"ok": true, "path": "/mcp/drive"}, res.StructuredContent)

	for _, args := range []map[string]any{{}, {"caller": "habit"}} {
		res, err = cs.CallTool(t.Context(), &mcp.CallToolParams{Name: "applet_list", Arguments: args})
		require.NoError(t, err)
		require.False(t, res.IsError, "%v", res.Content)
		assert.Equal(t, map[string]any{"ok": true, "path": "/mcp"}, res.StructuredContent)
	}

	fake.mu.Lock()
	defer fake.mu.Unlock()
	require.Len(t, fake.calls["/mcp/drive"], 1)
	assert.Equal(t, "agent-1", fake.calls["/mcp/drive"][0].Args["caller"])
	assert.Equal(t, []recordedCall{{Tool: "applet_list", Args: map[string]any{}}, {Tool: "applet_list", Args: map[string]any{}}}, fake.calls["/mcp"])
}

func TestForwardedArgsMatchEachOpsCallerField(t *testing.T) {
	for _, op := range catalog.All() {
		info := op.Info()
		if info.Owner != ops.OwnerDesktop {
			continue
		}
		schema, err := op.InputSchema()
		require.NoError(t, err)
		_, declared := schema.Properties[ops.CallerField]

		args, err := forwardedArgs(json.RawMessage(`{"caller":"spoofed"}`), info, "agent-1")
		require.NoError(t, err)
		got, sent := args[ops.CallerField]
		assert.Equal(t, declared, sent, info.ToolName)
		if declared {
			assert.Equal(t, "agent-1", got, info.ToolName)
		}
	}
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
