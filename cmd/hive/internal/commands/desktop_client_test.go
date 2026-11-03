package commands

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v3"

	"github.com/colonyops/hive/cmd/hive/internal/app"
	"github.com/colonyops/hive/cmd/hive/internal/config"
	"github.com/colonyops/hive/internal/ops"
	"github.com/colonyops/hive/internal/ops/catalog"
	"github.com/colonyops/hive/internal/platform/desktopendpoint"
)

type recordedCall struct {
	Tool string
	Args map[string]any
}

// fakeDrive serves every desktop tool of the catalog at its MCP path and
// records each call.
type fakeDrive struct {
	srv *httptest.Server

	mu    sync.Mutex
	calls []recordedCall
}

func newFakeDrive(t *testing.T, refuse string) *fakeDrive {
	t.Helper()
	f := &fakeDrive{}
	servers := map[string]*mcp.Server{}
	for _, op := range catalog.All() {
		info := op.Info()
		server, ok := servers[info.Toolset.MCPPath]
		if !ok {
			server = mcp.NewServer(&mcp.Implementation{Name: "fake", Version: "test"}, nil)
			servers[info.Toolset.MCPPath] = server
		}
		server.AddTool(&mcp.Tool{
			Name:        info.ToolName,
			InputSchema: map[string]any{"type": "object"},
		}, func(_ context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			args := map[string]any{}
			if len(req.Params.Arguments) > 0 {
				if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
					return nil, err
				}
			}
			f.mu.Lock()
			f.calls = append(f.calls, recordedCall{Tool: req.Params.Name, Args: args})
			f.mu.Unlock()

			if refuse != "" {
				return &mcp.CallToolResult{
					IsError: true,
					Content: []mcp.Content{&mcp.TextContent{Text: refuse}},
				}, nil
			}
			return &mcp.CallToolResult{
				Content:           []mcp.Content{&mcp.TextContent{Text: `{"ok":true}`}},
				StructuredContent: map[string]any{"ok": true, "tool": req.Params.Name},
			}, nil
		})
	}

	mux := http.NewServeMux()
	for path, server := range servers {
		mux.Handle(path, mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, nil))
	}
	f.srv = httptest.NewServer(mux)
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeDrive) only(t *testing.T) recordedCall {
	t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	require.Len(t, f.calls, 1)
	return f.calls[0]
}

type desktopRun struct {
	stdout string
	err    error
}

func runDesktop(t *testing.T, dataDir string, args ...string) desktopRun {
	t.Helper()
	var out, errOut bytes.Buffer
	cmd := NewDesktopCmd(&Flags{}, &app.App{Config: &config.Config{}})
	cmd.app.Config.DataDir = dataDir

	root := &cli.Command{Name: "hive", Writer: &out, ErrWriter: &errOut}
	cmd.Register(root)

	err := root.Run(context.Background(), append([]string{"hive", "dt"}, args...))
	return desktopRun{stdout: out.String(), err: err}
}

func TestDesktopToolArguments(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		wantTool string
		want     map[string]any
	}{
		{
			name:     "layout",
			args:     []string{"layout", "get"},
			wantTool: "layout_get",
			want:     map[string]any{"caller": "s1"},
		},
		{
			name:     "open sends only the flags that are set",
			args:     []string{"tab", "open", "drive", "--path", "plans/x.md", "--placement", "beside", "--anchor", "diff"},
			wantTool: "tab_open",
			want: map[string]any{
				"caller": "s1", "view": "drive", "path": "plans/x.md", "placement": "beside", "anchor": "diff",
			},
		},
		{
			name:     "open a new terminal window",
			args:     []string{"tab", "open", "terminal", "--name", "tests"},
			wantTool: "tab_open",
			want:     map[string]any{"caller": "s1", "view": "terminal", "name": "tests"},
		},
		{
			name:     "close a view",
			args:     []string{"tab", "close", "diff"},
			wantTool: "tab_close",
			want:     map[string]any{"caller": "s1", "view": "diff"},
		},
		{
			name:     "close an applet's tab",
			args:     []string{"tab", "close", "applet:radio"},
			wantTool: "tab_close",
			want:     map[string]any{"caller": "s1", "view": "applet:radio"},
		},
		{
			name:     "a field named url keeps its own flag",
			args:     []string{"canvas-block", "put", "plan", "pr", "--kind", "link", "--title", "PR", "--url", "https://example.com/1"},
			wantTool: "canvas_block_put",
			want: map[string]any{
				"caller": "s1", "canvas": "plan", "id": "pr", "kind": "link", "title": "PR", "url": "https://example.com/1",
			},
		},
		{
			name:     "close a terminal window",
			args:     []string{"tab", "close", "terminal", "--window", "@3"},
			wantTool: "tab_close",
			want:     map[string]any{"caller": "s1", "view": "terminal", "window": "@3"},
		},
		{
			name:     "input file fields, overridden by a flag",
			args:     []string{"tab", "open", "--input", `{"view":"drive","path":"a.md","caller":"from-input"}`, "--path", "b.md"},
			wantTool: "tab_open",
			want:     map[string]any{"caller": "from-input", "view": "drive", "path": "b.md"},
		},
		{
			name:     "move",
			args:     []string{"tab", "move", "tasks", "--placement", "below", "--anchor", "drive"},
			wantTool: "tab_move",
			want:     map[string]any{"caller": "s1", "tab": "tasks", "placement": "below", "anchor": "drive"},
		},
		{
			name:     "move without an anchor",
			args:     []string{"tab", "move", "tasks", "--placement", "behind"},
			wantTool: "tab_move",
			want:     map[string]any{"caller": "s1", "tab": "tasks", "placement": "behind"},
		},
		{
			name:     "focus",
			args:     []string{"tab", "focus", "drive"},
			wantTool: "tab_focus",
			want:     map[string]any{"caller": "s1", "tab": "drive"},
		},
		{
			name:     "panel close",
			args:     []string{"panel", "close", "diff"},
			wantTool: "panel_close",
			want:     map[string]any{"caller": "s1", "tab": "diff"},
		},
		{
			name:     "split resize",
			args:     []string{"split", "resize", "drive", "--share", "0.6"},
			wantTool: "split_resize",
			want:     map[string]any{"caller": "s1", "tab": "drive", "share": 0.6},
		},
		{
			name:     "split turn",
			args:     []string{"split", "turn", "drive"},
			wantTool: "split_turn",
			want:     map[string]any{"caller": "s1", "tab": "drive"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := newFakeDrive(t, "")
			args := append(append([]string{}, tt.args...), "--desktop-url", fake.srv.URL, "--caller", "s1")

			res := runDesktop(t, t.TempDir(), args...)
			require.NoError(t, res.err)

			call := fake.only(t)
			assert.Equal(t, tt.wantTool, call.Tool)
			assert.Equal(t, tt.want, call.Args)

			var got map[string]any
			require.NoError(t, json.Unmarshal([]byte(res.stdout), &got), "stdout: %q", res.stdout)
			assert.Equal(t, true, got["ok"])
			assert.Equal(t, tt.wantTool, got["tool"])
			assert.Equal(t, 1, bytes.Count([]byte(res.stdout), []byte("\n")), "one JSON line")
		})
	}
}

func TestDesktopCallerDefaults(t *testing.T) {
	t.Run("agent session variable", func(t *testing.T) {
		t.Setenv("HIVE_AGENT_SESSION", "chat-42")
		fake := newFakeDrive(t, "")

		res := runDesktop(t, t.TempDir(), "layout", "get", "--desktop-url", fake.srv.URL)
		require.NoError(t, res.err)
		assert.Equal(t, "chat-42", fake.only(t).Args["caller"])
	})

	t.Run("current directory", func(t *testing.T) {
		t.Setenv("HIVE_AGENT_SESSION", "")
		dir := t.TempDir()
		t.Chdir(dir)
		wd, err := os.Getwd()
		require.NoError(t, err)
		fake := newFakeDrive(t, "")

		res := runDesktop(t, t.TempDir(), "layout", "get", "--desktop-url", fake.srv.URL)
		require.NoError(t, res.err)
		assert.Equal(t, wd, fake.only(t).Args["caller"])
	})

	t.Run("flag beats variable", func(t *testing.T) {
		t.Setenv("HIVE_AGENT_SESSION", "chat-42")
		fake := newFakeDrive(t, "")

		res := runDesktop(t, t.TempDir(), "layout", "get", "--desktop-url", fake.srv.URL, "--caller", "other")
		require.NoError(t, res.err)
		assert.Equal(t, "other", fake.only(t).Args["caller"])
	})
}

func TestDesktopURLResolution(t *testing.T) {
	t.Run("environment variable", func(t *testing.T) {
		fake := newFakeDrive(t, "")
		t.Setenv("HIVE_DESKTOP_URL", fake.srv.URL)

		res := runDesktop(t, t.TempDir(), "layout", "get", "--caller", "s1")
		require.NoError(t, res.err)
		assert.Equal(t, "layout_get", fake.only(t).Tool)
	})

	t.Run("endpoint file", func(t *testing.T) {
		t.Setenv("HIVE_DESKTOP_URL", "")
		fake := newFakeDrive(t, "")
		dataDir := t.TempDir()
		require.NoError(t, desktopendpoint.Write(dataDir, desktopendpoint.Endpoint{URL: fake.srv.URL, PID: 1}))

		res := runDesktop(t, dataDir, "layout", "get", "--caller", "s1")
		require.NoError(t, res.err)
		assert.Equal(t, "layout_get", fake.only(t).Tool)
	})

	t.Run("the flag and the environment outrank a stale endpoint file", func(t *testing.T) {
		closed := httptest.NewServer(http.NotFoundHandler())
		stale := closed.URL
		closed.Close()
		dataDir := t.TempDir()
		require.NoError(t, desktopendpoint.Write(dataDir, desktopendpoint.Endpoint{URL: stale, PID: 1}))

		fromEnv := newFakeDrive(t, "")
		t.Setenv("HIVE_DESKTOP_URL", fromEnv.srv.URL)
		res := runDesktop(t, dataDir, "layout", "get", "--caller", "s1")
		require.NoError(t, res.err)
		assert.Equal(t, "layout_get", fromEnv.only(t).Tool)

		fromFlag := newFakeDrive(t, "")
		res = runDesktop(t, dataDir, "layout", "get", "--caller", "s1", "--desktop-url", fromFlag.srv.URL)
		require.NoError(t, res.err)
		assert.Equal(t, "layout_get", fromFlag.only(t).Tool)
	})
}

func TestDesktopToolErrorIsTheMessage(t *testing.T) {
	fake := newFakeDrive(t, `not_found: tab "@9" is not in this session`)

	res := runDesktop(t, t.TempDir(), "tab", "focus", "@9", "--desktop-url", fake.srv.URL, "--caller", "s1")

	require.EqualError(t, res.err, `not_found: tab "@9" is not in this session`)
	opErr, ok := errors.AsType[*ops.Error](res.err)
	require.True(t, ok)
	assert.Equal(t, 3, ops.ExitCode(opErr.Kind))
	assert.Empty(t, res.stdout)
}

func TestDesktopNotRunning(t *testing.T) {
	t.Setenv("HIVE_DESKTOP_URL", "")

	t.Run("no endpoint file", func(t *testing.T) {
		res := runDesktop(t, t.TempDir(), "layout", "get", "--caller", "s1")
		require.EqualError(t, res.err, "Hive Desktop is not running; start it, or pass --desktop-url")
	})

	t.Run("connection refused", func(t *testing.T) {
		closed := httptest.NewServer(http.NotFoundHandler())
		url := closed.URL
		closed.Close()

		res := runDesktop(t, t.TempDir(), "layout", "get", "--caller", "s1", "--desktop-url", url)
		require.EqualError(t, res.err, "Hive Desktop is not running; start it, or pass --desktop-url")
	})

	t.Run("stale endpoint file", func(t *testing.T) {
		closed := httptest.NewServer(http.NotFoundHandler())
		url := closed.URL
		closed.Close()
		dataDir := t.TempDir()
		require.NoError(t, desktopendpoint.Write(dataDir, desktopendpoint.Endpoint{URL: url, PID: 1}))

		res := runDesktop(t, dataDir, "layout", "get", "--caller", "s1")
		require.EqualError(t, res.err, "Hive Desktop is not running; start it, or pass --desktop-url")
	})
}

func TestDesktopLocalValidation(t *testing.T) {
	fake := newFakeDrive(t, "")
	base := []string{"--desktop-url", fake.srv.URL, "--caller", "s1"}

	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{
			name:    "placement",
			args:    []string{"tab", "open", "drive", "--placement", "above"},
			wantErr: `invalid: placement "above" is not one of behind, beside, below`,
		},
		{
			name:    "share too high",
			args:    []string{"split", "resize", "drive", "--share", "0.95"},
			wantErr: "invalid: share 0.95 is outside 0.2 to 0.8",
		},
		{
			name:    "share too low",
			args:    []string{"split", "resize", "drive", "--share", "0.1"},
			wantErr: "invalid: share 0.1 is outside 0.2 to 0.8",
		},
		{
			name:    "missing argument",
			args:    []string{"tab", "focus"},
			wantErr: "invalid: <tab> is required",
		},
		{
			name:    "too many arguments",
			args:    []string{"tab", "focus", "a", "b"},
			wantErr: "invalid: tab.focus takes at most 1 argument(s): <tab>",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := runDesktop(t, t.TempDir(), append(tt.args, base...)...)
			require.EqualError(t, res.err, tt.wantErr)
			opErr, ok := errors.AsType[*ops.Error](res.err)
			require.True(t, ok)
			assert.Equal(t, 2, ops.ExitCode(opErr.Kind))
		})
	}

	fake.mu.Lock()
	defer fake.mu.Unlock()
	assert.Empty(t, fake.calls, "a locally refused call never reaches the desktop")
}
