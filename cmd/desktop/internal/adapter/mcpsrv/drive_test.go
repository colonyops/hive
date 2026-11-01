package mcpsrv_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/desktop/internal/app"
	"github.com/colonyops/hive/cmd/desktop/internal/app/events"
)

type documentView struct {
	Kind  string         `json:"kind"`
	Path  string         `json:"path"`
	Name  string         `json:"name"`
	Title string         `json:"title"`
	Size  int64          `json:"size"`
	Front map[string]any `json:"front"`
	Body  string         `json:"body"`
}

type documentListView struct {
	Space    string             `json:"space"`
	Kind     string             `json:"kind"`
	Path     string             `json:"path"`
	Detail   string             `json:"detail"`
	Children []documentListView `json:"children"`
}

func childPaths(entries []documentListView) []string {
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		out = append(out, e.Path)
	}
	return out
}

func TestDocumentRoundTrip(t *testing.T) {
	core, session := testCanvasSession(t)
	id := seedAgentSession(t, core, "demo", "chat")

	var wrote documentView
	call(t, session, "write_document", map[string]any{
		"session": id, "path": "plans/auth.md", "body": "---\ntitle: Auth rewrite\ntags: [auth]\n---\n# Plan\n",
	}, &wrote)
	assert.Equal(t, "markdown", wrote.Kind)
	assert.Equal(t, "plans/auth.md", wrote.Path)
	assert.Equal(t, "Auth rewrite", wrote.Title)
	assert.Equal(t, []any{"auth"}, wrote.Front["tags"])

	var read documentView
	call(t, session, "read_document", map[string]any{"session": id, "path": "plans/auth.md"}, &read)
	assert.Equal(t, "---\ntitle: Auth rewrite\ntags: [auth]\n---\n# Plan\n", read.Body)

	call(t, session, "write_document", map[string]any{
		"session": id, "path": "reports/2026-10/perf.html", "body": "<h1>Perf</h1>",
	}, &wrote)
	assert.Equal(t, "html", wrote.Kind)

	var moved documentView
	call(t, session, "move_document", map[string]any{"session": id, "from": "plans/auth.md", "to": "archive/auth.md"}, &moved)
	assert.Equal(t, "archive/auth.md", moved.Path)
	text := callErr(t, session, "read_document", map[string]any{"session": id, "path": "plans/auth.md"})
	assert.Contains(t, text, "not_found")

	var deleted struct {
		Deleted string `json:"deleted"`
	}
	call(t, session, "delete_document", map[string]any{"session": id, "path": "archive/auth.md"}, &deleted)
	assert.Equal(t, "archive/auth.md", deleted.Deleted, "the answer names what was deleted, never a constant")
	call(t, session, "delete_document", map[string]any{"session": id, "path": "archive"}, &deleted)
	assert.Equal(t, "archive", deleted.Deleted, "an empty folder deletes")

	text = callErr(t, session, "delete_document", map[string]any{"session": id, "path": "reports"})
	assert.Contains(t, text, "not empty")
}

func TestWriteDocumentRefusals(t *testing.T) {
	core, session := testCanvasSession(t)
	id := seedAgentSession(t, core, "demo", "chat")

	text := callErr(t, session, "write_document", map[string]any{"session": id, "path": "reports/perf.canvas.json", "body": "{}"})
	assert.Contains(t, text, "invalid")
	assert.Contains(t, text, "put_blocks", "a canvas path points at the canvas tools")

	text = callErr(t, session, "write_document", map[string]any{"session": id, "path": "../escape.md", "body": "x"})
	assert.Contains(t, text, "invalid")

	text = callErr(t, session, "write_document", map[string]any{"session": id, "path": "notes.txt", "body": "x"})
	assert.Contains(t, text, "invalid")
}

func TestListDocuments(t *testing.T) {
	core, session := testCanvasSession(t)
	id := seedAgentSession(t, core, "demo", "chat")

	var root documentListView
	call(t, session, "list_documents", map[string]any{"session": id}, &root)
	assert.Equal(t, "workspaces/demo", root.Space)
	assert.Equal(t, "dir", root.Kind)
	assert.Equal(t, "children", root.Detail, "the answer echoes the level it used")
	assert.Empty(t, root.Children)

	text := callErr(t, session, "list_documents", map[string]any{"session": id, "path": "ghost"})
	assert.Contains(t, text, "not_found")
	text = callErr(t, session, "list_documents", map[string]any{"session": id, "detail": "full"})
	assert.Contains(t, text, "invalid")

	var wrote documentView
	call(t, session, "write_document", map[string]any{"session": id, "path": "reports/2026-10/perf.md", "body": "x"}, &wrote)
	call(t, session, "write_document", map[string]any{"session": id, "path": "readme.md", "body": "x"}, &wrote)

	call(t, session, "list_documents", map[string]any{"session": id}, &root)
	assert.Equal(t, []string{"reports", "readme.md"}, childPaths(root.Children), "folders first")
	assert.Empty(t, root.Children[0].Children, "one level by default")

	call(t, session, "list_documents", map[string]any{"session": id, "detail": "tree"}, &root)
	assert.Equal(t, "tree", root.Detail)
	require.Len(t, root.Children[0].Children, 1)
	assert.Equal(t, "reports/2026-10", root.Children[0].Children[0].Path)
	assert.Equal(t, []string{"reports/2026-10/perf.md"}, childPaths(root.Children[0].Children[0].Children))

	var folder documentListView
	call(t, session, "list_documents", map[string]any{"session": id, "path": "reports/2026-10"}, &folder)
	assert.Equal(t, []string{"reports/2026-10/perf.md"}, childPaths(folder.Children))

	var doc documentListView
	call(t, session, "list_documents", map[string]any{"session": id, "path": "readme.md"}, &doc)
	assert.Equal(t, "markdown", doc.Kind, "a document path lists that document")
}

func TestCanvasNamesFileUnderPaths(t *testing.T) {
	core, session := testCanvasSession(t)
	id := seedAgentSession(t, core, "demo", "chat")

	var wrote canvasWriteView
	call(t, session, "put_blocks", map[string]any{
		"session": id, "canvas": "reports/2026-10/perf", "blocks": []map[string]any{{"id": "a", "kind": "markdown", "body": "x"}},
	}, &wrote)
	assert.Equal(t, "reports/2026-10/perf", wrote.Name)
	call(t, session, "put_block", map[string]any{"session": id, "canvas": "plan", "id": "a", "kind": "markdown", "body": "x"}, &wrote)
	assert.Equal(t, "plan", wrote.Name)

	var folder documentListView
	call(t, session, "list_documents", map[string]any{"session": id, "path": "reports/2026-10"}, &folder)
	assert.Equal(t, []string{"reports/2026-10/perf.canvas.json"}, childPaths(folder.Children))
	call(t, session, "list_documents", map[string]any{"session": id, "path": "canvases"}, &folder)
	assert.Equal(t, []string{"canvases/plan.canvas.json"}, childPaths(folder.Children), "a bare name still files under canvases/")

	var got canvasView
	call(t, session, "read_canvas", map[string]any{"session": id, "canvas": "reports/2026-10/perf"}, &got)
	require.Len(t, got.Blocks, 1)

	var listed canvasListView
	call(t, session, "list_canvases", map[string]any{"session": id}, &listed)
	names := []string{}
	for _, c := range listed.Canvases {
		names = append(names, c.Name)
	}
	assert.ElementsMatch(t, []string{"reports/2026-10/perf", "plan"}, names)
}

func TestTabToolsPublishTheRequest(t *testing.T) {
	core, session := testCanvasSession(t)
	id := seedAgentSession(t, core, "demo", "chat")

	var mu sync.Mutex
	var tabs []events.SessionLayoutRequested
	cancel := events.Subscribe(t.Context(), core.Events, "test.session-tab", events.Buffer(8), func(_ context.Context, e events.SessionLayoutRequested) {
		mu.Lock()
		defer mu.Unlock()
		tabs = append(tabs, e)
	})
	t.Cleanup(cancel)
	waitTabs := func(n int) []events.SessionLayoutRequested {
		require.Eventually(t, func() bool {
			mu.Lock()
			defer mu.Unlock()
			return len(tabs) == n
		}, 2*time.Second, 5*time.Millisecond)
		mu.Lock()
		defer mu.Unlock()
		return append([]events.SessionLayoutRequested(nil), tabs...)
	}

	text := callErr(t, session, "open_tab", map[string]any{"session": id, "view": "drive", "path": "plans/ghost.md"})
	assert.Contains(t, text, "not_found", "the document must exist")

	var wrote documentView
	call(t, session, "write_document", map[string]any{"session": id, "path": "plans/auth.md", "body": "x"}, &wrote)
	var opened struct {
		Requested string  `json:"requested"`
		View      string  `json:"view"`
		Path      string  `json:"path"`
		Window    *string `json:"window"`
	}
	call(t, session, "open_tab", map[string]any{"session": id, "view": "drive", "path": "plans/auth.md", "placement": "beside"}, &opened)
	assert.Equal(t, "open", opened.Requested)
	require.NotNil(t, opened.Window, "the result always carries window")
	assert.Empty(t, *opened.Window, "and it is empty for a drive tab")
	assert.Equal(t, "plans/auth.md", opened.Path)

	var closed struct {
		Requested string `json:"requested"`
		View      string `json:"view"`
	}
	call(t, session, "close_tab", map[string]any{"session": id, "view": "drive"}, &closed)
	assert.Equal(t, "close", closed.Requested)

	assert.Equal(t, []events.SessionLayoutRequested{
		{Session: id, Space: "workspaces/demo", Verb: "open", View: "drive", Path: "plans/auth.md", Placement: "beside"},
		{Session: id, Space: "workspaces/demo", Verb: "close", View: "drive"},
	}, waitTabs(2))

	text = callErr(t, session, "open_tab", map[string]any{"session": id, "view": "diff"})
	assert.Contains(t, text, "invalid", "a chat has only a Drive tab")
	text = callErr(t, session, "open_tab", map[string]any{"session": id, "view": "terminal"})
	assert.Contains(t, text, "a chat has one terminal", "new terminals belong to a Code session")
	text = callErr(t, session, "open_tab", map[string]any{"session": id, "view": "terminal", "window": "@3", "command": "make"})
	assert.Contains(t, text, "window places an existing terminal")
	text = callErr(t, session, "open_tab", map[string]any{"session": id, "view": "drive", "name": "x"})
	assert.Contains(t, text, "apply only to the terminal view")
	text = callErr(t, session, "close_tab", map[string]any{"session": id, "view": "terminal", "window": "@3"})
	assert.Contains(t, text, "a chat's terminal is the chat")
	text = callErr(t, session, "open_tab", map[string]any{"session": "global", "view": "drive"})
	assert.Contains(t, text, "invalid", "the shared space has no tabs")
	text = callErr(t, session, "close_tab", map[string]any{"session": "global", "view": "drive"})
	assert.Contains(t, text, "invalid", "the shared space has no tabs")
}

func TestLayoutToolsPublishAndRefuse(t *testing.T) {
	core, session := testCanvasSession(t)
	id := seedAgentSession(t, core, "demo", "chat")

	var mu sync.Mutex
	var got []events.SessionLayoutRequested
	cancel := events.Subscribe(t.Context(), core.Events, "test.session-layout", events.Buffer(8), func(_ context.Context, e events.SessionLayoutRequested) {
		mu.Lock()
		defer mu.Unlock()
		got = append(got, e)
	})
	t.Cleanup(cancel)

	var moved struct {
		Requested string `json:"requested"`
		Tab       string `json:"tab"`
	}
	call(t, session, "move_tab", map[string]any{"session": id, "tab": "terminal", "placement": "below", "anchor": "drive"}, &moved)
	assert.Equal(t, "move", moved.Requested)
	call(t, session, "focus_tab", map[string]any{"session": id, "tab": "drive"}, &moved)
	call(t, session, "turn_split", map[string]any{"session": id, "tab": "terminal"}, &moved)
	call(t, session, "close_panel", map[string]any{"session": id, "tab": "drive"}, &moved)
	call(t, session, "resize_split", map[string]any{"session": id, "tab": "drive", "share": 0.3}, &moved)

	require.Eventually(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(got) == 5
	}, 2*time.Second, 5*time.Millisecond)
	mu.Lock()
	defer mu.Unlock()
	space := "workspaces/demo"
	assert.Equal(t, []events.SessionLayoutRequested{
		{Session: id, Space: space, Verb: "move", View: "terminal", Placement: "below", AnchorView: "drive"},
		{Session: id, Space: space, Verb: "focus", View: "drive"},
		{Session: id, Space: space, Verb: "turn", View: "terminal"},
		{Session: id, Space: space, Verb: "close_panel", View: "drive"},
		{Session: id, Space: space, Verb: "resize", View: "drive", Share: 0.3},
	}, got)

	text := callErr(t, session, "resize_split", map[string]any{"session": id, "tab": "drive", "share": 0.1})
	assert.Contains(t, text, "invalid")
	text = callErr(t, session, "focus_tab", map[string]any{"session": id, "tab": "@3"})
	assert.Contains(t, text, "a chat has one terminal; say terminal")
	text = callErr(t, session, "move_tab", map[string]any{"session": id, "tab": "diff"})
	assert.Contains(t, text, "invalid")
}

func TestGetLayoutAnswersFromTheMirror(t *testing.T) {
	core, session := testCanvasSession(t)
	id := seedAgentSession(t, core, "demo", "chat")

	type layout struct {
		Known        bool   `json:"known"`
		OnScreen     bool   `json:"onScreen"`
		FocusedPanel string `json:"focusedPanel"`
		Note         string `json:"note"`
		Panels       []struct {
			ID   string `json:"id"`
			Tabs []struct {
				Tab   string `json:"tab"`
				Kind  string `json:"kind"`
				Shown bool   `json:"shown"`
			} `json:"tabs"`
		} `json:"panels"`
		Splits []struct {
			ID     string  `json:"id"`
			Axis   string  `json:"axis"`
			Ratio  float64 `json:"ratio"`
			First  string  `json:"first"`
			Second string  `json:"second"`
		} `json:"splits"`
	}
	var before layout
	call(t, session, "get_layout", map[string]any{"session": id}, &before)
	assert.False(t, before.Known)
	assert.NotEmpty(t, before.Note)
	assert.Empty(t, before.Panels)

	require.NoError(t, core.Layouts.Report(app.LayoutKindChats, id, map[string]app.PanelLayoutReport{
		id: {FocusedPanel: "p1", Root: app.PanelNodeReport{
			Kind: "split", ID: "s1", Axis: "row", Ratio: 0.5,
			Children: []app.PanelNodeReport{
				{Kind: "panel", ID: "p0", ActiveTab: "t0", Tabs: []app.PanelTabReport{{ID: "t0", Kind: "window"}}},
				{Kind: "panel", ID: "p1", ActiveTab: "t1", Tabs: []app.PanelTabReport{{ID: "t1", Kind: "docs"}}},
			},
		}},
	}))
	var after layout
	call(t, session, "get_layout", map[string]any{"session": id}, &after)
	assert.True(t, after.Known)
	assert.True(t, after.OnScreen)
	assert.Equal(t, "p1", after.FocusedPanel)
	require.Len(t, after.Panels, 2)
	assert.Equal(t, "terminal", after.Panels[0].Tabs[0].Tab)
	assert.Equal(t, "drive", after.Panels[1].Tabs[0].Kind)
	assert.True(t, after.Panels[1].Tabs[0].Shown)
	require.Len(t, after.Splits, 1)
	assert.Equal(t, "p0", after.Splits[0].First)
}
