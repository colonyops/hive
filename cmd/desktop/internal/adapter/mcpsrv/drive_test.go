package mcpsrv_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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

func TestOpenDocumentPublishesTheToggle(t *testing.T) {
	core, session := testCanvasSession(t)
	id := seedAgentSession(t, core, "demo", "chat")

	var mu sync.Mutex
	var toggles []events.DriveToggleRequested
	cancel := events.Subscribe(t.Context(), core.Events, "test.drive-toggle", events.Buffer(8), func(_ context.Context, e events.DriveToggleRequested) {
		mu.Lock()
		defer mu.Unlock()
		toggles = append(toggles, e)
	})
	t.Cleanup(cancel)

	text := callErr(t, session, "open_document", map[string]any{"session": id, "path": "plans/ghost.md"})
	assert.Contains(t, text, "not_found", "the document must exist")

	var wrote documentView
	call(t, session, "write_document", map[string]any{"session": id, "path": "plans/auth.md", "body": "x"}, &wrote)
	var pane struct {
		Requested string `json:"requested"`
	}
	call(t, session, "open_document", map[string]any{"session": id, "path": "plans/auth.md"}, &pane)
	assert.Equal(t, "open", pane.Requested)

	require.Eventually(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(toggles) == 1
	}, 2*time.Second, 5*time.Millisecond)
	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, events.DriveToggleRequested{Session: id, Space: "workspaces/demo", Path: "plans/auth.md", Open: true}, toggles[0])

	text = callErr(t, session, "open_document", map[string]any{"session": "global", "path": "x.md"})
	assert.Contains(t, text, "invalid", "the shared space has no Drive tab")
}
