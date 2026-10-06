package mcpsrv_test

import (
	"fmt"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/desktop/internal/adapter/mcpsrv"
	"github.com/colonyops/hive/cmd/desktop/internal/app"
	"github.com/colonyops/hive/cmd/desktop/internal/app/data/stores"
	"github.com/colonyops/hive/cmd/desktop/internal/app/mcpcatalog"
	"github.com/colonyops/hive/cmd/desktop/internal/app/settings"
)

// testCanvasSession is testSession's canvas-server twin: same app, same
// in-memory transport, the hive-canvas tool table under test instead.
func testCanvasSession(t *testing.T) (*app.App, *mcp.ClientSession) {
	t.Helper()
	root := t.TempDir()
	t.Setenv(settings.EnvDataDir, filepath.Join(root, "data"))
	t.Setenv("HIVE_CONFIG", filepath.Join(root, "hive.yaml"))
	t.Setenv(settings.EnvConfigDir, filepath.Join(root, "config"))
	t.Setenv(settings.EnvMockMode, "feed")

	core, err := app.New(t.Context(), app.Config{
		Settings: settings.DefaultSettings(),
		MockMode: settings.MockMode(),
		Logger:   zerolog.Nop(),
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = core.Close() })

	srv := mcpsrv.NewCanvas(core, zerolog.Nop(), mcpsrv.Options{Version: "test"}).Server()

	clientTransport, serverTransport := mcp.NewInMemoryTransports()
	_, err = srv.Connect(t.Context(), serverTransport, nil)
	require.NoError(t, err)

	session, err := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "test"}, nil).
		Connect(t.Context(), clientTransport, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = session.Close() })

	return core, session
}

// seedAgentSession answers the record id as a chat's agent passes it: the
// text of HIVE_AGENT_SESSION.
func seedAgentSession(t *testing.T, core *app.App, workspace, name string) string {
	t.Helper()
	rec, err := core.Stores.AgentSessions.Create(t.Context(), stores.AgentSessionCreate{
		Workspace: workspace, Name: name, Agent: "claude",
	})
	require.NoError(t, err)
	return strconv.FormatInt(rec.ID, 10)
}

type canvasView struct {
	Workspace string `json:"workspace"`
	Name      string `json:"name"`
	Title     string `json:"title"`
	Session   int64  `json:"session"`
	Blocks    []struct {
		ID    string `json:"id"`
		Kind  string `json:"kind"`
		Title string `json:"title"`
		Body  string `json:"body"`
		URL   string `json:"url"`
	} `json:"blocks"`
}

type canvasListView struct {
	Canvases []struct {
		Name       string `json:"name"`
		Title      string `json:"title"`
		Session    int64  `json:"session"`
		BlockCount int    `json:"blockCount"`
	} `json:"canvases"`
}

// canvasWriteView is what every mutation answers: metadata, plus the stored
// block for a single-block write — never the whole surface.
type canvasWriteView struct {
	Workspace  string `json:"workspace"`
	Name       string `json:"name"`
	Title      string `json:"title"`
	Session    int64  `json:"session"`
	BlockCount int    `json:"blockCount"`
	Block      *struct {
		ID   string `json:"id"`
		Body string `json:"body"`
	} `json:"block"`
}

func TestCanvasToolsListDeclaresEveryToolWithAnObjectInputSchema(t *testing.T) {
	_, session := testCanvasSession(t)

	res, err := session.ListTools(t.Context(), nil)
	require.NoError(t, err)

	names := make([]string, 0, len(res.Tools))
	for _, tool := range res.Tools {
		names = append(names, tool.Name)
		assert.NotEmpty(t, tool.Description, "tool %s has no description", tool.Name)

		require.NotNil(t, tool.InputSchema, "tool %s has no input schema", tool.Name)
		schema, ok := tool.InputSchema.(map[string]any)
		require.True(t, ok, "tool %s input schema is not an object", tool.Name)
		assert.Equal(t, "object", schema["type"], "tool %s input schema is not type object", tool.Name)
	}

	assert.ElementsMatch(t, []string{"put_block", "put_blocks", "remove_block", "clear_canvas", "delete_canvas", "read_canvas", "list_canvases", "open_canvas", "close_canvas"}, names)
}

func TestCanvasRoundTrip(t *testing.T) {
	core, session := testCanvasSession(t)
	id := seedAgentSession(t, core, "demo", "chat")

	var listed canvasListView
	call(t, session, "list_canvases", map[string]any{"session": id}, &listed)
	assert.Empty(t, listed.Canvases, "a fresh workspace has no canvases")

	var wrote canvasWriteView
	call(t, session, "put_block", map[string]any{
		"session": id, "canvas": "plan", "canvasTitle": "The Plan",
		"id": "status", "kind": "markdown", "title": "Progress", "body": "working…",
	}, &wrote)
	assert.Equal(t, "demo", wrote.Workspace)
	assert.Equal(t, "plan", wrote.Name)
	assert.Equal(t, "The Plan", wrote.Title)
	assert.Equal(t, id, strconv.FormatInt(wrote.Session, 10), "the creating chat is recorded")
	assert.Equal(t, 1, wrote.BlockCount)
	require.NotNil(t, wrote.Block, "a single-block write echoes the stored block")
	assert.Equal(t, "status", wrote.Block.ID)
	call(t, session, "put_block", map[string]any{
		"session": id, "canvas": "plan", "id": "pr", "kind": "link", "title": "The PR", "url": "https://example.com/pr/1",
	}, &wrote)
	assert.Equal(t, 2, wrote.BlockCount)
	assert.Equal(t, "The Plan", wrote.Title, "an omitted canvasTitle keeps the stored one")

	// Same id revises in place: position and count hold, content changes.
	call(t, session, "put_block", map[string]any{
		"session": id, "canvas": "plan", "id": "status", "kind": "markdown", "body": "done",
	}, &wrote)
	assert.Equal(t, 2, wrote.BlockCount)
	assert.Equal(t, "done", wrote.Block.Body)
	var got canvasView
	call(t, session, "read_canvas", map[string]any{"session": id, "canvas": "plan"}, &got)
	require.Len(t, got.Blocks, 2)
	assert.Equal(t, "status", got.Blocks[0].ID)
	assert.Equal(t, "done", got.Blocks[0].Body)

	// A before anchor places a block ahead of an existing one.
	call(t, session, "put_block", map[string]any{
		"session": id, "canvas": "plan", "id": "intro", "kind": "markdown", "body": "i", "before": "status",
	}, &wrote)
	call(t, session, "read_canvas", map[string]any{"session": id, "canvas": "plan"}, &got)
	require.Len(t, got.Blocks, 3)
	assert.Equal(t, "intro", got.Blocks[0].ID)

	// A batch is one atomic write: a second canvas appears whole.
	wrote = canvasWriteView{}
	call(t, session, "put_blocks", map[string]any{
		"session": id, "canvas": "report", "blocks": []map[string]any{
			{"id": "a", "kind": "markdown", "body": "x"},
			{"id": "b", "kind": "markdown", "body": "```mermaid\nflowchart LR\nA --> B\n```"},
			{"id": "c", "kind": "link", "title": "The PR", "url": "https://example.com/pr/1"},
		},
	}, &wrote)
	assert.Equal(t, "report", wrote.Name)
	assert.Equal(t, 3, wrote.BlockCount)
	assert.Nil(t, wrote.Block, "a batch echoes no single block")
	call(t, session, "list_canvases", map[string]any{"session": id}, &listed)
	require.Len(t, listed.Canvases, 2)
	names := []string{listed.Canvases[0].Name, listed.Canvases[1].Name}
	assert.ElementsMatch(t, []string{"plan", "report"}, names)

	call(t, session, "remove_block", map[string]any{"session": id, "canvas": "plan", "id": "pr"}, &wrote)
	assert.Equal(t, 2, wrote.BlockCount)

	call(t, session, "clear_canvas", map[string]any{"session": id, "canvas": "plan"}, &wrote)
	assert.Equal(t, 0, wrote.BlockCount)
	assert.Equal(t, "The Plan", wrote.Title, "clear keeps the canvas and its title")

	call(t, session, "read_canvas", map[string]any{"session": id, "canvas": "plan"}, &got)
	assert.Empty(t, got.Blocks)

	var deleted struct {
		Deleted string `json:"deleted"`
	}
	call(t, session, "delete_canvas", map[string]any{"session": id, "canvas": "report"}, &deleted)
	assert.Equal(t, "report", deleted.Deleted)
	call(t, session, "list_canvases", map[string]any{"session": id}, &listed)
	require.Len(t, listed.Canvases, 1)
	assert.Equal(t, "plan", listed.Canvases[0].Name)
}

func TestCanvasPaneToggle(t *testing.T) {
	core, session := testCanvasSession(t)
	id := seedAgentSession(t, core, "demo", "chat")

	var pane struct {
		Requested string `json:"requested"`
	}
	call(t, session, "open_canvas", map[string]any{"session": id}, &pane)
	assert.Equal(t, "open", pane.Requested, "the result reports the ask, not a pane state nothing acknowledges")

	text := callErr(t, session, "open_canvas", map[string]any{"session": id, "canvas": "ghost"})
	assert.Contains(t, text, "not_found", "pinning the pane to a canvas requires it to exist")

	var wrote canvasWriteView
	call(t, session, "put_block", map[string]any{"session": id, "canvas": "plan", "id": "a", "kind": "markdown", "body": "x"}, &wrote)
	call(t, session, "open_canvas", map[string]any{"session": id, "canvas": "plan"}, &pane)
	assert.Equal(t, "open", pane.Requested)

	call(t, session, "close_canvas", map[string]any{"session": id}, &pane)
	assert.Equal(t, "close", pane.Requested)
}

func TestCanvasToolErrors(t *testing.T) {
	core, session := testCanvasSession(t)
	id := seedAgentSession(t, core, "demo", "chat")

	text := callErr(t, session, "read_canvas", map[string]any{"session": "999", "canvas": "plan"})
	assert.Contains(t, text, "not_found")

	text = callErr(t, session, "read_canvas", map[string]any{"session": id, "canvas": "ghost"})
	assert.Contains(t, text, "not_found", "a name nothing was written under is not_found, not blank")

	text = callErr(t, session, "delete_canvas", map[string]any{"session": id, "canvas": "ghost"})
	assert.Contains(t, text, "not_found")

	var got canvasWriteView
	call(t, session, "put_block", map[string]any{"session": id, "canvas": "plan", "id": "a", "kind": "markdown", "body": "x"}, &got)
	text = callErr(t, session, "remove_block", map[string]any{"session": id, "canvas": "plan", "id": "ghost"})
	assert.Contains(t, text, "not_found")

	text = callErr(t, session, "put_block", map[string]any{"session": id, "canvas": "Bad Name", "id": "a", "kind": "markdown", "body": "x"})
	assert.Contains(t, text, "invalid")

	text = callErr(t, session, "put_block", map[string]any{"session": id, "canvas": "plan", "id": "a", "kind": "diagram", "body": "x"})
	assert.Contains(t, text, "invalid")

	text = callErr(t, session, "put_block", map[string]any{
		"session": id, "canvas": "plan", "id": "a", "kind": "html", "body": `<div class="hv-card" onclick="x()">x</div>`,
	})
	assert.Contains(t, text, "invalid", "an html block is refused rather than silently stripped")

	text = callErr(t, session, "put_block", map[string]any{
		"session": id, "canvas": "plan", "id": "a", "kind": "link", "title": "x", "url": "javascript:alert(1)",
	})
	assert.Contains(t, text, "invalid")
}

// TestCanvasCatalogueAgreement pins the adapter's mount paths to the
// RuntimePaths mcpcatalog declares — the literals live in core so the
// registry stays adapter-free, and this is what keeps them from drifting.
func TestCanvasCatalogueAgreement(t *testing.T) {
	desktop, ok := mcpcatalog.Lookup("hive-desktop")
	require.True(t, ok)
	assert.Equal(t, mcpsrv.PathPrefix, desktop.RuntimePath)

	canvasEntry, ok := mcpcatalog.Lookup("hive-canvas")
	require.True(t, ok)
	assert.Equal(t, mcpsrv.CanvasPathPrefix, canvasEntry.RuntimePath)
}

// The canvas mount is a second exact-match mux entry beside /mcp; each server
// answers only its own tools, so the two mounts must not shadow each other.
func TestCanvasMountedBesideTheDesktopServer(t *testing.T) {
	root := t.TempDir()
	t.Setenv(settings.EnvDataDir, filepath.Join(root, "data"))
	t.Setenv("HIVE_CONFIG", filepath.Join(root, "hive.yaml"))
	t.Setenv(settings.EnvConfigDir, filepath.Join(root, "config"))
	t.Setenv(settings.EnvMockMode, "feed")
	t.Setenv(settings.EnvHTTPEnabled, "true")
	t.Setenv(settings.EnvHTTPPort, "0")

	cfg, err := settings.NewStore(filepath.Join(root, "config", "settings.yaml")).Effective()
	require.NoError(t, err)
	core, err := app.New(t.Context(), app.Config{Settings: cfg, MockMode: cfg.MockMode(), Logger: zerolog.Nop()})
	require.NoError(t, err)
	t.Cleanup(func() { _ = core.Close() })

	require.True(t, core.MountAPI(mcpsrv.PathPrefix,
		mcpsrv.New(core, zerolog.Nop(), mcpsrv.Options{Version: "test"}).Handler()))
	require.True(t, core.MountAPI(mcpsrv.CanvasPathPrefix,
		mcpsrv.NewCanvas(core, zerolog.Nop(), mcpsrv.Options{Version: "test"}).Handler()))
	require.NoError(t, core.Start(t.Context()))

	id := seedAgentSession(t, core, "demo", "chat")

	running, port := core.Webhooks.Endpoint(t.Context())
	require.True(t, running)

	canvasSession, err := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "test"}, nil).
		Connect(t.Context(), &mcp.StreamableClientTransport{
			Endpoint:             fmt.Sprintf("http://127.0.0.1:%d%s", port, mcpsrv.CanvasPathPrefix),
			DisableStandaloneSSE: true,
		}, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = canvasSession.Close() })

	var got canvasWriteView
	call(t, canvasSession, "put_block", map[string]any{
		"session": id, "canvas": "plan", "id": "a", "kind": "markdown", "body": "x",
	}, &got)
	assert.Equal(t, "demo", got.Workspace)
	assert.Equal(t, "plan", got.Name)

	// The desktop server still answers at its own path with its own table.
	desktopSession, err := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "test"}, nil).
		Connect(t.Context(), &mcp.StreamableClientTransport{
			Endpoint:             fmt.Sprintf("http://127.0.0.1:%d%s", port, mcpsrv.PathPrefix),
			DisableStandaloneSSE: true,
		}, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = desktopSession.Close() })

	tools, err := desktopSession.ListTools(t.Context(), nil)
	require.NoError(t, err)
	names := make([]string, 0, len(tools.Tools))
	for _, tool := range tools.Tools {
		names = append(names, tool.Name)
	}
	assert.Contains(t, names, "get_status")
	assert.NotContains(t, names, "put_block", "the canvas table must not leak onto the app-control server")
}
