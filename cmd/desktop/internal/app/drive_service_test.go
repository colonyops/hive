package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/desktop/internal/app/data/stores"
	"github.com/colonyops/hive/cmd/desktop/internal/app/events"
	"github.com/colonyops/hive/cmd/desktop/internal/app/settings"
	"github.com/colonyops/hive/internal/config"
	"github.com/colonyops/hive/internal/domain/drive"
	"github.com/colonyops/hive/internal/domain/session"
	"github.com/colonyops/hive/internal/hive"
)

type fakeCanvasSessions map[string]stores.AgentSession

func (f fakeCanvasSessions) Get(_ context.Context, id string) (stores.AgentSession, error) {
	rec, ok := f[id]
	if !ok {
		return stores.AgentSession{}, stores.NotFoundError{Entity: "agent_workspace_session", Key: id}
	}
	return rec, nil
}

// canvasSignals mirrors the two events DriveService publishes. Delivery runs
// on the bus subscriber's own goroutine, so every field is guarded by a
// mutex and read through the wait helpers rather than directly.
type canvasSignals struct {
	bus     *events.Bus
	mu      sync.Mutex
	updates []events.DriveChanged
	toggles []events.DriveToggleRequested
}

func (s *canvasSignals) addUpdate(e events.DriveChanged) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.updates = append(s.updates, e)
}

func (s *canvasSignals) addToggle(e events.DriveToggleRequested) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.toggles = append(s.toggles, e)
}

func (s *canvasSignals) Updates() []events.DriveChanged {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.updates)
}

func (s *canvasSignals) Toggles() []events.DriveToggleRequested {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.toggles)
}

// waitUpdates polls until exactly n updates have been recorded. Publish hands
// off to the bus subscriber's own goroutine, so a test asserting a positive
// count has to wait rather than read Updates() synchronously.
func (s *canvasSignals) waitUpdates(t *testing.T, n int) []events.DriveChanged {
	t.Helper()
	require.Eventually(t, func() bool { return len(s.Updates()) == n }, eventWait, 5*time.Millisecond)
	return s.Updates()
}

func (s *canvasSignals) waitToggles(t *testing.T, n int) []events.DriveToggleRequested {
	t.Helper()
	require.Eventually(t, func() bool { return len(s.Toggles()) == n }, eventWait, 5*time.Millisecond)
	return s.Toggles()
}

// requireNoUpdates proves the service published nothing so far. Buffer
// delivers in order, so a sentinel published now lands behind anything the
// service already published; once it arrives the recorded list is complete,
// which a synchronous read of Updates() could never guarantee.
func (s *canvasSignals) requireNoUpdates(t *testing.T, msg string) {
	t.Helper()
	sentinel := events.DriveChanged{Session: "sentinel"}
	s.bus.Publish(t.Context(), sentinel)
	require.Eventually(t, func() bool {
		return slices.ContainsFunc(s.Updates(), func(e events.DriveChanged) bool { return e.Session == sentinel.Session })
	}, eventWait, 5*time.Millisecond)
	assert.Equal(t, []events.DriveChanged{sentinel}, s.Updates(), msg)
}

func updateSessions(updates []events.DriveChanged) []string {
	out := make([]string, 0, len(updates))
	for _, e := range updates {
		out = append(out, e.Session)
	}
	return out
}

type driveFixture struct {
	svc     *DriveService
	signals *canvasSignals
	harness *hiveHarness
}

// newDriveFixture is a DriveService over a real engine whose context root is
// a temp dir, with chat "1" in workspace "ws" and the given hive sessions
// saved in hive.db.
func newDriveFixture(t *testing.T, hiveSessions ...session.Session) driveFixture {
	t.Helper()
	bus := newTestBus(t)
	signals := &canvasSignals{bus: bus}
	cancelUpdated := events.Subscribe(t.Context(), bus, "test.canvas-updated", events.Buffer(64), func(_ context.Context, e events.DriveChanged) {
		signals.addUpdate(e)
	})
	t.Cleanup(cancelUpdated)
	cancelToggled := events.Subscribe(t.Context(), bus, "test.canvas-toggled", events.Buffer(64), func(_ context.Context, e events.DriveToggleRequested) {
		signals.addToggle(e)
	})
	t.Cleanup(cancelToggled)

	contextRoot := t.TempDir()
	h := newHiveHarness(t, engineOptions{cfg: func(cfg *config.Config) { cfg.Context.BaseDir = contextRoot }})
	h.save(t, hiveSessions...)
	svc := newDriveService(DriveDeps{
		Engine:   func() *hive.Engine { return h.engine },
		Sessions: fakeCanvasSessions{"1": {ID: "1", Workspace: "ws", Name: "chat", Agent: "claude"}},
		Events:   bus,
	})
	return driveFixture{svc: svc, signals: signals, harness: h}
}

func (f driveFixture) contextDir() string { return f.harness.cfg.ContextDir() }

func (f driveFixture) canvasesAt(ctx context.Context, space string) ([]drive.CanvasMeta, error) {
	parsed, err := drive.ParseSpaceKey(space)
	if err != nil {
		return nil, err
	}
	return f.svc.listCanvases(ctx, f.harness.engine.Drive(), parsed)
}

func mustSpace(space drive.Space, err error) drive.Space {
	if err != nil {
		panic(err)
	}
	return space
}

var wsSpace = mustSpace(drive.WorkspaceSpace("ws"))

func markdownBlock(id, body string) drive.Block {
	return drive.Block{ID: id, Kind: drive.BlockKindMarkdown, Body: body}
}

func TestCanvasUnknownSessionIsNotFound(t *testing.T) {
	f := newDriveFixture(t)
	ctx := t.Context()

	_, err := f.svc.GetCanvas(ctx, "99", "plan")
	assert.Equal(t, KindNotFound, KindOf(err))
	_, err = f.svc.PutBlock(ctx, "99", "plan", "", "", markdownBlock("a", "x"))
	assert.Equal(t, KindNotFound, KindOf(err))
	_, err = f.svc.RemoveBlock(ctx, "99", "plan", "a")
	assert.Equal(t, KindNotFound, KindOf(err))
	_, err = f.svc.SetFrontmatter(ctx, "99", "plan", drive.Frontmatter{"tags": []any{"x"}})
	assert.Equal(t, KindNotFound, KindOf(err))
	_, err = f.svc.ClearCanvas(ctx, "99", "plan")
	assert.Equal(t, KindNotFound, KindOf(err))
	err = f.svc.DeleteCanvas(ctx, "99", "plan")
	assert.Equal(t, KindNotFound, KindOf(err))
	_, err = f.svc.ListCanvases(ctx, "99")
	assert.Equal(t, KindNotFound, KindOf(err))
}

func TestCanvasGetUnknownNameIsNotFound(t *testing.T) {
	f := newDriveFixture(t)

	_, err := f.svc.GetCanvas(t.Context(), "1", "plan")
	assert.Equal(t, KindNotFound, KindOf(err), "an agent asking by name should learn the name is wrong")
	f.signals.requireNoUpdates(t, "a read never notifies")
}

func TestCanvasGetAtUnwrittenAnswersEmpty(t *testing.T) {
	f := newDriveFixture(t)

	c, err := f.svc.GetCanvasAt(t.Context(), "workspaces/ws", "plan")
	require.NoError(t, err)
	assert.Equal(t, "workspaces/ws", c.Space)
	assert.Equal(t, "canvases/plan.canvas.json", c.Path)
	assert.NotNil(t, c.Frontmatter)
	assert.NotNil(t, c.Blocks)
	assert.Empty(t, c.Blocks)
	f.signals.requireNoUpdates(t, "a read never notifies")

	_, err = f.svc.GetCanvasAt(t.Context(), "nowhere", "plan")
	assert.Equal(t, KindInvalid, KindOf(err), "a key that is no space is the caller's mistake")
}

func TestCanvasPutBlockValidation(t *testing.T) {
	f := newDriveFixture(t)
	ctx := t.Context()

	cases := map[string]drive.Block{
		"empty id":              {ID: "  ", Kind: drive.BlockKindMarkdown, Body: "x"},
		"id too long":           {ID: strings.Repeat("a", 201), Kind: drive.BlockKindMarkdown, Body: "x"},
		"unknown kind":          {ID: "a", Kind: "diagram", Body: "x"},
		"markdown without body": {ID: "a", Kind: drive.BlockKindMarkdown},
		"markdown with url":     {ID: "a", Kind: drive.BlockKindMarkdown, Body: "x", URL: "https://example.com"},
		"oversized body":        {ID: "a", Kind: drive.BlockKindMarkdown, Body: strings.Repeat("x", maxCanvasBodyBytes+1)},
		"html without body":     {ID: "a", Kind: drive.BlockKindHTML},
		"html with url":         {ID: "a", Kind: drive.BlockKindHTML, Body: "<p>x</p>", URL: "https://example.com"},
		"html with a script":    {ID: "a", Kind: drive.BlockKindHTML, Body: "<p>x</p><script>alert(1)</script>"},
		"html with a handler":   {ID: "a", Kind: drive.BlockKindHTML, Body: `<div onclick="x()">x</div>`},
		"html with an id":       {ID: "a", Kind: drive.BlockKindHTML, Body: `<div id="wails">x</div>`},
		"html with a bad link":  {ID: "a", Kind: drive.BlockKindHTML, Body: `<a href="javascript:alert(1)">x</a>`},
		"html with a bad image": {ID: "a", Kind: drive.BlockKindHTML, Body: `<img src="javascript:alert(1)">`},
		"link without title":    {ID: "a", Kind: drive.BlockKindLink, URL: "https://example.com"},
		"link without url":      {ID: "a", Kind: drive.BlockKindLink, Title: "t"},
		"link with body":        {ID: "a", Kind: drive.BlockKindLink, Title: "t", URL: "https://example.com", Body: "x"},
		"javascript url":        {ID: "a", Kind: drive.BlockKindLink, Title: "t", URL: "javascript:alert(1)"},
	}
	for name, block := range cases {
		_, err := f.svc.PutBlock(ctx, "1", "plan", "", "", block)
		assert.Equal(t, KindInvalid, KindOf(err), name)
	}

	_, err := f.svc.PutBlock(ctx, "1", "Bad Name", "", "", markdownBlock("a", "x"))
	assert.Equal(t, KindInvalid, KindOf(err), "an invalid canvas name is the caller's mistake")
	_, err = f.svc.PutBlock(ctx, "1", "../escape", "", "", markdownBlock("a", "x"))
	assert.Equal(t, KindInvalid, KindOf(err), "a path out of the space is refused")
	_, err = f.svc.PutBlock(ctx, "1", "plan", strings.Repeat("t", maxCanvasTitleLength+1), "", markdownBlock("a", "x"))
	assert.Equal(t, KindInvalid, KindOf(err), "an oversized canvas title is refused")

	f.signals.requireNoUpdates(t, "a refused write never notifies")
}

func TestCanvasTitlesDecodeHTMLEntities(t *testing.T) {
	f := newDriveFixture(t)

	c, err := f.svc.PutBlock(t.Context(), "1", "plan", "Rice &amp; Peppers", "", drive.Block{
		ID: "a", Kind: drive.BlockKindLink, Title: "Q&amp;A &lt;draft&gt;", URL: "https://example.com",
	})
	require.NoError(t, err)
	assert.Equal(t, "Rice & Peppers", c.Title)
	assert.Equal(t, "Q&A <draft>", c.Blocks[0].Title)
}

// A silently stripped tag is the one failure an agent cannot see, so the
// error names it. How a block looks is the agent's own business, so styling
// it never reaches this path.
func TestCanvasHTMLBlockRejectionNamesTheOffender(t *testing.T) {
	f := newDriveFixture(t)
	ctx := t.Context()

	_, err := f.svc.PutBlock(ctx, "1", "plan", "", "", drive.Block{
		ID: "a", Kind: drive.BlockKindHTML, Body: `<div><iframe src="https://x.example"></iframe></div>`,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "<iframe>")

	_, err = f.svc.PutBlock(ctx, "1", "plan", "", "", drive.Block{
		ID: "a", Kind: drive.BlockKindHTML,
		Body: `<div class="hv-grid hv-cols-2"><span class="hv-badge hv-warn">2 flaky</span></div>`,
	})
	require.NoError(t, err, "the vocabulary itself is accepted")

	_, err = f.svc.PutBlock(ctx, "1", "plan", "", "", drive.Block{
		ID: "a", Kind: drive.BlockKindHTML,
		Body: `<div class="mystery" style="color:#f00"><img src="https://example.com/a.png"></div>`,
	})
	require.NoError(t, err, "a class, a style and an image are the agent's own business")
}

// The pane's read is the seam between stored agent source and the app's own
// webview; the agent's read-back is not, so the two disagree by design.
func TestCanvasHTMLIsSanitizedOnTheWayOutNotIn(t *testing.T) {
	f := newDriveFixture(t)
	ctx := t.Context()

	// Written straight to the engine: validateBlock would have refused this,
	// which is exactly why the read path cannot rely on it.
	_, err := f.harness.engine.Drive().UpsertBlocks(ctx, wsSpace, "canvases/plan.canvas.json", drive.Author{Session: "1"}, "", "", drive.Block{
		ID: "a", Kind: drive.BlockKindHTML, Body: `<p class="hv-muted">ok</p><script>alert(1)</script>`,
	})
	require.NoError(t, err)

	shown, err := f.svc.GetCanvasAt(ctx, "workspaces/ws", "plan")
	require.NoError(t, err)
	assert.Equal(t, `<p class="hv-muted">ok</p>`, shown.Blocks[0].Body)

	stored, err := f.svc.GetCanvas(ctx, "1", "plan")
	require.NoError(t, err)
	assert.Contains(t, stored.Blocks[0].Body, "<script>", "read_canvas shows the agent what it wrote")
}

func TestCanvasReadsReportAMigratedChatByItsUUID(t *testing.T) {
	const chatID = "0199bd92-cc5f-7c61-9185-7423813c9c12"
	f := newDriveFixture(t)
	migrated := stores.AgentSession{ID: chatID, LegacyID: "7", Workspace: "ws", Name: "migrated", Agent: "claude"}
	f.svc.sessions = fakeCanvasSessions{"7": migrated, chatID: migrated}
	ctx := t.Context()

	dir := filepath.Join(f.contextDir(), "workspaces", "ws", "canvases")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	for _, author := range []int{7, 8} {
		// The pre-drive file format: owner and name fields, a numeric session.
		data := fmt.Sprintf(`{
			"workspace":"ws", "name":"by-%d", "title":"Legacy canvas", "session":%d,
			"createdAt":1, "updatedAt":2,
			"blocks":[{"id":"a", "kind":"markdown", "body":"x", "createdAt":1, "updatedAt":2}]
		}`, author, author)
		require.NoError(t, os.WriteFile(filepath.Join(dir, fmt.Sprintf("by-%d.canvas.json", author)), []byte(data), 0o644))
	}

	shown, err := f.svc.GetCanvasAt(ctx, "workspaces/ws", "by-7")
	require.NoError(t, err)
	assert.Equal(t, drive.Canvas{
		Space: "workspaces/ws", Path: "canvases/by-7.canvas.json", Title: "Legacy canvas", Session: chatID,
		CreatedAt: 1, UpdatedAt: 2, Frontmatter: drive.Frontmatter{},
		Blocks: []drive.Block{{ID: "a", Kind: drive.BlockKindMarkdown, Body: "x", CreatedAt: 1, UpdatedAt: 2}},
	}, shown)

	for _, id := range []string{chatID, "7"} {
		read, err := f.svc.GetCanvas(ctx, id, "by-7")
		require.NoError(t, err)
		assert.Equal(t, shown, read)
	}

	metas, err := f.canvasesAt(ctx, "workspaces/ws")
	require.NoError(t, err)
	authors := map[string]string{}
	for _, meta := range metas {
		authors[drive.CanvasName(meta.Path)] = meta.Session
	}
	assert.Equal(t, map[string]string{"by-7": chatID, "by-8": "8"}, authors, "a deleted chat keeps its old number")
}

func TestCanvasPutBlocks(t *testing.T) {
	f := newDriveFixture(t)
	ctx := t.Context()

	_, err := f.svc.PutBlocks(ctx, "1", "plan", "The Plan", nil)
	assert.Equal(t, KindInvalid, KindOf(err), "an empty batch is refused")

	_, err = f.svc.PutBlocks(ctx, "1", "plan", "", []drive.Block{
		markdownBlock("a", "x"),
		{ID: "b", Kind: drive.BlockKindLink, Title: "t"},
	})
	assert.Equal(t, KindInvalid, KindOf(err))
	assert.Contains(t, err.Error(), "blocks[1]", "the error names which block was rejected")

	_, err = f.svc.PutBlocks(ctx, "1", "plan", "", []drive.Block{markdownBlock("a", "x"), markdownBlock("a", "y")})
	assert.Equal(t, KindInvalid, KindOf(err), "duplicate ids in one batch are refused")

	metas, err := f.canvasesAt(ctx, "workspaces/ws")
	require.NoError(t, err)
	assert.Empty(t, metas, "a rejected batch writes nothing")
	assert.Empty(t, f.signals.Updates())

	c, err := f.svc.PutBlocks(ctx, "1", "plan", "The Plan", []drive.Block{markdownBlock("a", "x"), markdownBlock("b", "y")})
	require.NoError(t, err)
	assert.Len(t, c.Blocks, 2)
	f.signals.waitUpdates(t, 1)

	oversized := make([]drive.Block, maxCanvasBatchBlocks+1)
	for i := range oversized {
		oversized[i] = markdownBlock(strings.Repeat("a", i+1), "x")
	}
	_, err = f.svc.PutBlocks(ctx, "1", "plan", "", oversized)
	assert.Equal(t, KindInvalid, KindOf(err))
}

func TestCanvasPutBlockBeforeAnchor(t *testing.T) {
	f := newDriveFixture(t)
	ctx := t.Context()

	_, err := f.svc.PutBlock(ctx, "1", "plan", "", "", markdownBlock("a", "x"))
	require.NoError(t, err)

	c, err := f.svc.PutBlock(ctx, "1", "plan", "", "a", markdownBlock("intro", "i"))
	require.NoError(t, err)
	assert.Equal(t, "intro", c.Blocks[0].ID)

	_, err = f.svc.PutBlock(ctx, "1", "plan", "", "ghost", markdownBlock("x", "x"))
	assert.Equal(t, KindNotFound, KindOf(err), "a missing anchor is the caller's mistake")
}

func TestCanvasMutationsNotify(t *testing.T) {
	f := newDriveFixture(t)
	ctx := t.Context()

	_, err := f.svc.PutBlock(ctx, "1", "plan", "The Plan", "", markdownBlock("a", "x"))
	require.NoError(t, err)
	_, err = f.svc.PutBlock(ctx, "1", "plan", "", "", drive.Block{ID: "pr", Kind: drive.BlockKindLink, Title: "PR", URL: "https://example.com/pr/1"})
	require.NoError(t, err)
	c, err := f.svc.SetFrontmatter(ctx, "1", "plan", drive.Frontmatter{"tags": []any{"release"}})
	require.NoError(t, err)
	assert.Equal(t, drive.Frontmatter{"tags": []any{"release"}}, c.Frontmatter)
	_, err = f.svc.RemoveBlock(ctx, "1", "plan", "pr")
	require.NoError(t, err)
	c, err = f.svc.ClearCanvas(ctx, "1", "plan")
	require.NoError(t, err)
	assert.Empty(t, c.Blocks)
	assert.Equal(t, "The Plan", c.Title)
	err = f.svc.DeleteCanvas(ctx, "1", "plan")
	require.NoError(t, err)

	updates := f.signals.waitUpdates(t, 6)
	assert.Equal(t, []string{"1", "1", "1", "1", "1", "1"}, updateSessions(updates))
}

func TestCanvasMutationsOnUnknownCanvasAreNotFound(t *testing.T) {
	f := newDriveFixture(t)
	ctx := t.Context()

	_, err := f.svc.RemoveBlock(ctx, "1", "ghost", "a")
	assert.Equal(t, KindNotFound, KindOf(err))
	_, err = f.svc.SetFrontmatter(ctx, "1", "ghost", drive.Frontmatter{"tags": []any{"x"}})
	assert.Equal(t, KindNotFound, KindOf(err))
	_, err = f.svc.ClearCanvas(ctx, "1", "ghost")
	assert.Equal(t, KindNotFound, KindOf(err))
	err = f.svc.DeleteCanvas(ctx, "1", "ghost")
	assert.Equal(t, KindNotFound, KindOf(err))
	f.signals.requireNoUpdates(t, "a mutation on an unknown canvas never notifies")
}

func TestCanvasSetPaneOpen(t *testing.T) {
	f := newDriveFixture(t)
	ctx := t.Context()

	require.NoError(t, f.svc.SetPaneOpen(ctx, "1", "", true), "opening without a name leaves the pane's own pick")
	err := f.svc.SetPaneOpen(ctx, "1", "ghost", true)
	assert.Equal(t, KindNotFound, KindOf(err), "opening pinned to a canvas requires it to exist")

	_, err = f.svc.PutBlock(ctx, "1", "plan", "", "", markdownBlock("a", "x"))
	require.NoError(t, err)
	require.NoError(t, f.svc.SetPaneOpen(ctx, "1", "plan", true))
	require.NoError(t, f.svc.SetPaneOpen(ctx, "1", "canvases/plan.canvas.json", true), "a document path pins the same canvas")
	require.NoError(t, f.svc.SetPaneOpen(ctx, "1", "", false))
	err = f.svc.SetPaneOpen(ctx, "99", "", true)
	assert.Equal(t, KindNotFound, KindOf(err))

	assert.Equal(t, []events.DriveToggleRequested{
		{Session: "1", Space: "workspaces/ws", Open: true},
		{Session: "1", Space: "workspaces/ws", Path: "canvases/plan.canvas.json", Open: true},
		{Session: "1", Space: "workspaces/ws", Path: "canvases/plan.canvas.json", Open: true},
		{Session: "1", Space: "workspaces/ws", Open: false},
	}, f.signals.waitToggles(t, 4))
	f.signals.waitUpdates(t, 1)
}

func TestCanvasRemoveAbsentBlockIsNotFound(t *testing.T) {
	f := newDriveFixture(t)
	ctx := t.Context()
	_, err := f.svc.PutBlock(ctx, "1", "plan", "", "", markdownBlock("a", "x"))
	require.NoError(t, err)
	_, err = f.svc.RemoveBlock(ctx, "1", "plan", "ghost")
	assert.Equal(t, KindNotFound, KindOf(err))
}

func TestCanvasExport(t *testing.T) {
	f := newDriveFixture(t)
	ctx := t.Context()

	_, err := f.svc.Markdown(ctx, "workspaces/ws", "ghost")
	assert.Equal(t, KindNotFound, KindOf(err))

	_, err = f.svc.PutBlock(ctx, "1", "plan", "The Plan", "", markdownBlock("a", "hello"))
	require.NoError(t, err)
	_, err = f.svc.SetFrontmatter(ctx, "1", "plan", drive.Frontmatter{"tags": []any{"release"}})
	require.NoError(t, err)

	markdown, err := f.svc.Markdown(ctx, "workspaces/ws", "plan")
	require.NoError(t, err)
	assert.Contains(t, markdown, "tags:\n    - release\n---\n\n# The Plan\n\nhello\n")

	err = f.svc.Export(ctx, "workspaces/ws", "plan", "relative.md")
	assert.Equal(t, KindInvalid, KindOf(err), "the save dialog hands back absolute paths; anything else is a caller bug")

	dest := filepath.Join(t.TempDir(), "plan.md")
	require.NoError(t, f.svc.Export(ctx, "workspaces/ws", "plan", dest))
	written, err := os.ReadFile(dest)
	require.NoError(t, err)
	assert.Equal(t, markdown, string(written))

	f.signals.waitUpdates(t, 2)
}

func TestDriveDeleteOfACanvasWakesItsAuthor(t *testing.T) {
	f := newDriveFixture(t)
	ctx := t.Context()

	_, err := f.svc.PutBlock(ctx, "1", "plan", "The Plan", "", markdownBlock("a", "hello"))
	require.NoError(t, err)
	require.NoError(t, f.svc.Delete(ctx, "workspaces/ws", "canvases/plan.canvas.json"))
	err = f.svc.Delete(ctx, "workspaces/ws", "canvases/plan.canvas.json")
	assert.Equal(t, KindNotFound, KindOf(err))

	updates := f.signals.waitUpdates(t, 2)
	assert.Equal(t, []string{"1", "1"}, updateSessions(updates), "the delete wakes the chat that wrote the canvas")
}

func TestCanvasListBySpaceAndBySession(t *testing.T) {
	f := newDriveFixture(t)
	ctx := t.Context()

	metas, err := f.canvasesAt(ctx, "workspaces/ws")
	require.NoError(t, err)
	assert.Empty(t, metas)

	_, err = f.svc.PutBlock(ctx, "1", "plan", "The Plan", "", markdownBlock("a", "x"))
	require.NoError(t, err)
	metas, err = f.canvasesAt(ctx, "workspaces/ws")
	require.NoError(t, err)
	require.Len(t, metas, 1)
	assert.Equal(t, "workspaces/ws", metas[0].Space)
	assert.Equal(t, "canvases/plan.canvas.json", metas[0].Path)
	assert.Equal(t, "The Plan", metas[0].Title)
	assert.Equal(t, "1", metas[0].Session)

	fromSession, err := f.svc.ListCanvases(ctx, "1")
	require.NoError(t, err)
	assert.Equal(t, metas, fromSession, "ListCanvases resolves the session's space and answers the same rows")
}

var siteSession = session.Session{
	ID: "abc123", Name: "fix-login", Slug: "fix-login", Path: "/work/site-abc123",
	Remote: "git@github.com:acme/site.git", State: session.StateActive, CloneStrategy: session.CloneStrategyFull,
}

// An agent in a hive session has no id to pass, only where it runs. The
// checkout that holds that directory names the repository, and the canvas is
// filed in that repository's space, outside the checkout.
func TestCanvasFromAHiveSessionIsFiledUnderItsRepository(t *testing.T) {
	f := newDriveFixture(t, siteSession)
	ctx := t.Context()

	c, err := f.svc.PutBlock(ctx, "/work/site-abc123/cmd/api", "plan", "The Plan", "", markdownBlock("a", "x"))
	require.NoError(t, err)
	assert.Equal(t, "acme/site", c.Space)
	assert.Equal(t, "abc123", c.HiveSession)
	assert.Empty(t, c.Session)
	require.FileExists(t, filepath.Join(f.contextDir(), "acme", "site", "canvases", "plan.canvas.json"))

	updates := f.signals.waitUpdates(t, 1)
	assert.Equal(t, events.DriveChanged{Space: "acme/site", Paths: []string{"canvases/plan.canvas.json"}, HiveSession: "abc123"}, updates[0])

	read, err := f.svc.GetCanvas(ctx, "/work/site-abc123", "plan")
	require.NoError(t, err)
	assert.Equal(t, "The Plan", read.Title)

	shown, err := f.svc.GetCanvasAt(ctx, "acme/site", "plan")
	require.NoError(t, err)
	require.Len(t, shown.Blocks, 1)
}

// Two sessions of one repository share its canvases, the way two chats share
// a workspace's.
func TestHiveSessionsOfOneRepositoryShareItsCanvases(t *testing.T) {
	other := session.Session{
		ID: "def456", Name: "add-search", Slug: "add-search", Path: "/work/site-def456",
		Remote: "https://github.com/acme/site", State: session.StateActive, CloneStrategy: session.CloneStrategyFull,
	}
	f := newDriveFixture(t, siteSession, other)
	ctx := t.Context()

	_, err := f.svc.PutBlock(ctx, siteSession.Path, "plan", "", "", markdownBlock("a", "x"))
	require.NoError(t, err)

	metas, err := f.svc.ListCanvases(ctx, other.Path)
	require.NoError(t, err)
	require.Len(t, metas, 1)
	assert.Equal(t, "abc123", metas[0].HiveSession, "the first author stays the provenance")
}

func TestCanvasSessionArgumentThatResolvesToNothing(t *testing.T) {
	noRemote := session.Session{
		ID: "zzz999", Name: "scratch", Slug: "scratch", Path: "/work/scratch",
		State: session.StateActive, CloneStrategy: session.CloneStrategyFull,
	}
	f := newDriveFixture(t, siteSession, noRemote)
	ctx := t.Context()
	block := markdownBlock("a", "x")

	_, err := f.svc.PutBlock(ctx, "/somewhere/else", "plan", "", "", block)
	assert.Equal(t, KindNotFound, KindOf(err), "a directory in no checkout is not filed as shared")
	require.ErrorContains(t, err, "global", "the error names the way out")

	_, err = f.svc.PutBlock(ctx, "site-abc123", "plan", "", "", block)
	assert.Equal(t, KindInvalid, KindOf(err), "neither a chat id nor an absolute path")

	_, err = f.svc.PutBlock(ctx, "/work/scratch", "plan", "", "", block)
	assert.Equal(t, KindInvalid, KindOf(err), "a session whose remote names no repository")

	_, err = f.svc.PutBlock(ctx, " 1 ", "plan", "", "", block)
	require.NoError(t, err, "a chat id still resolves, as HIVE_AGENT_SESSION carries it")
}

func TestAgentsOutsideEverySessionShareTheSharedSpace(t *testing.T) {
	f := newDriveFixture(t, siteSession)
	ctx := t.Context()

	c, err := f.svc.PutBlock(ctx, "global", "notes", "Notes", "", markdownBlock("a", "x"))
	require.NoError(t, err)
	assert.Equal(t, drive.SharedKey, c.Space)
	assert.Empty(t, c.Session)
	assert.Empty(t, c.HiveSession)

	_, err = f.svc.PutBlock(ctx, " global ", "notes", "", "", markdownBlock("b", "y"))
	require.NoError(t, err, "a second global agent writes to the same canvas")

	metas, err := f.canvasesAt(ctx, drive.SharedKey)
	require.NoError(t, err)
	require.Len(t, metas, 1)
	assert.Equal(t, 2, metas[0].BlockCount)
	updates := f.signals.waitUpdates(t, 2)
	notes := events.DriveChanged{Space: drive.SharedKey, Paths: []string{"canvases/notes.canvas.json"}}
	assert.Equal(t, []events.DriveChanged{notes, notes}, updates, "shared writes still wake the readers, with no author")

	err = f.svc.SetPaneOpen(ctx, "global", "notes", true)
	assert.Equal(t, KindInvalid, KindOf(err), "a shared canvas has no pane")
}

func TestCanvasPaneToggleNamesTheHiveSession(t *testing.T) {
	f := newDriveFixture(t, siteSession)

	require.NoError(t, f.svc.SetPaneOpen(t.Context(), siteSession.Path, "", true))

	toggles := f.signals.waitToggles(t, 1)
	assert.Equal(t, events.DriveToggleRequested{HiveSession: "abc123", Space: "acme/site", Open: true}, toggles[0])
}

// A repository's canvases sit where hive's own context layout puts the
// repository, so the checkout's .hive link and the pane read one directory.
func TestRepositoryCanvasesSitInTheHiveContextLayout(t *testing.T) {
	f := newDriveFixture(t, siteSession)

	_, err := f.svc.PutBlock(t.Context(), siteSession.Path, "plan", "", "", markdownBlock("a", "x"))
	require.NoError(t, err)

	require.FileExists(t, filepath.Join(f.harness.cfg.RepoContextDir("acme", "site"), drive.CanvasesDir, "plan.canvas.json"))
	assert.Equal(t, "acme/site", SpaceKeyForRemote("git@github.com:acme/site.git"))
	assert.Empty(t, SpaceKeyForRemote(""))
	assert.Empty(t, SpaceKeyForRemote("git@github.com:shared/site.git"), "a reserved owner names no space")
}

func TestOwnerForSpaceKey(t *testing.T) {
	for key, owner := range map[string]string{
		"shared":          "@global",
		"workspaces/demo": "demo",
		"acme/site":       "acme/site",
	} {
		assert.Equal(t, owner, OwnerForSpaceKey(key), key)
	}
}

func TestDriveIsUnavailableBeforeTheEngineStarts(t *testing.T) {
	svc := newDriveService(DriveDeps{
		Engine:   func() *hive.Engine { return nil },
		Sessions: fakeCanvasSessions{"1": {ID: "1", Workspace: "ws", Name: "chat", Agent: "claude"}},
		Events:   newTestBus(t),
	})
	ctx := t.Context()

	_, err := svc.GetCanvas(ctx, "1", "plan")
	assert.Equal(t, KindUnavailable, KindOf(err))
	_, err = svc.PutBlock(ctx, "/work/site-abc123", "plan", "", "", markdownBlock("a", "x"))
	assert.Equal(t, KindUnavailable, KindOf(err))
	_, err = svc.GetCanvasAt(ctx, "workspaces/ws", "plan")
	assert.Equal(t, KindUnavailable, KindOf(err))
	_, err = svc.Spaces(ctx)
	assert.Equal(t, KindUnavailable, KindOf(err))
	_, err = svc.Tree(ctx, "workspaces/ws")
	assert.Equal(t, KindUnavailable, KindOf(err))
	_, err = svc.Search(ctx, "plan", 0)
	assert.Equal(t, KindUnavailable, KindOf(err))
	_, err = svc.Document(ctx, "workspaces/ws", "a.md")
	assert.Equal(t, KindUnavailable, KindOf(err))
	_, err = svc.CreateFolder(ctx, "workspaces/ws", "x")
	assert.Equal(t, KindUnavailable, KindOf(err))
	_, err = svc.Rename(ctx, "workspaces/ws", "a.md", "b")
	assert.Equal(t, KindUnavailable, KindOf(err))
	_, err = svc.Move(ctx, "workspaces/ws", "a.md", "")
	assert.Equal(t, KindUnavailable, KindOf(err))
	assert.Equal(t, KindUnavailable, KindOf(svc.Delete(ctx, "workspaces/ws", "a.md")))
}

func TestLegacyCanvasImportsNameTheRootsOnlyTheDesktopKnows(t *testing.T) {
	data := t.TempDir()
	workspaces := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(workspaces, "demo"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(workspaces, ".git"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(workspaces, "notes.md"), nil, 0o644))

	imports, err := legacyCanvasImports(data, workspaces)
	require.NoError(t, err)
	assert.Equal(t, []legacyCanvasImport{
		{from: filepath.Join(data, "canvases"), space: drive.Shared},
		{from: filepath.Join(workspaces, "demo", "canvases"), space: mustSpace(drive.WorkspaceSpace("demo"))},
	}, imports)

	imports, err = legacyCanvasImports(data, filepath.Join(workspaces, "missing"))
	require.NoError(t, err)
	assert.Equal(t, []legacyCanvasImport{{from: filepath.Join(data, "canvases"), space: drive.Shared}}, imports,
		"a workspace root that does not exist yet still imports the shared canvases")
}

// The desktop filed global and workspace canvases outside the context root
// before the drive; the first start after the change moves them into their
// spaces, and a second start finds nothing to do.
func TestImportLegacyCanvasesMovesEveryPreDriveCanvasIntoItsSpace(t *testing.T) {
	f := newDriveFixture(t)
	paths := settings.Paths{DataDir: t.TempDir(), AgentWorkspacesDir: t.TempDir()}
	legacy := `{"workspace":"x","name":"x","session":"1","createdAt":1,"updatedAt":2,"blocks":[{"id":"a","kind":"markdown","body":"x","createdAt":1,"updatedAt":2}]}`
	for _, dir := range []string{filepath.Join(paths.DataDir, "canvases"), filepath.Join(paths.AgentWorkspacesDir, "demo", "canvases")} {
		require.NoError(t, os.MkdirAll(dir, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "plan.json"), []byte(legacy), 0o644))
	}

	importLegacyCanvases(t.Context(), zerolog.Nop(), f.harness.engine, paths)

	require.FileExists(t, filepath.Join(f.contextDir(), "shared", "canvases", "plan.canvas.json"))
	require.FileExists(t, filepath.Join(f.contextDir(), "workspaces", "demo", "canvases", "plan.canvas.json"))
	assert.NoDirExists(t, filepath.Join(paths.DataDir, "canvases"), "an emptied legacy directory is removed")
	assert.NoDirExists(t, filepath.Join(paths.AgentWorkspacesDir, "demo", "canvases"))

	shown, err := f.svc.GetCanvasAt(t.Context(), "workspaces/demo", "plan")
	require.NoError(t, err)
	assert.Equal(t, "workspaces/demo", shown.Space)
	require.Len(t, shown.Blocks, 1)

	importLegacyCanvases(t.Context(), zerolog.Nop(), f.harness.engine, paths)
	metas, err := f.canvasesAt(t.Context(), drive.SharedKey)
	require.NoError(t, err)
	assert.Len(t, metas, 1, "a second run is a no-op")
}

// writeDoc puts a file in a space on disk. Call it before the space's first
// read: no watcher runs in these tests, so the snapshot never sees a file
// written after the scan.
func (f driveFixture) writeDoc(t *testing.T, space, path, body string) {
	t.Helper()
	full := filepath.Join(f.contextDir(), filepath.FromSlash(space), filepath.FromSlash(path))
	require.NoError(t, os.MkdirAll(filepath.Dir(full), 0o755))
	require.NoError(t, os.WriteFile(full, []byte(body), 0o644))
}

func TestDriveDocumentByKind(t *testing.T) {
	f := newDriveFixture(t)
	ctx := t.Context()
	f.writeDoc(t, "workspaces/ws", "plans/p.md", "---\ntitle: P\n---\n# Hi <b>raw</b>\n")
	f.writeDoc(t, "workspaces/ws", "page.html", "<p>x</p>")
	f.writeDoc(t, "workspaces/ws", "data.bin", "\x00\x01")
	_, err := f.harness.engine.Drive().UpsertBlocks(ctx, wsSpace, "canvases/plan.canvas.json", drive.Author{Session: "1"}, "", "", drive.Block{
		ID: "a", Kind: drive.BlockKindHTML, Body: `<p>ok</p><script>alert(1)</script>`,
	})
	require.NoError(t, err)

	md, err := f.svc.Document(ctx, "workspaces/ws", "plans/p.md")
	require.NoError(t, err)
	assert.Equal(t, wsSpace, md.Space)
	assert.Equal(t, drive.KindMarkdown, md.Entry.Kind)
	assert.Equal(t, "P", md.Entry.Title)
	assert.Equal(t, "---\ntitle: P\n---\n# Hi <b>raw</b>\n", md.Markdown, "markdown leaves raw; the renderer sanitizes")
	assert.Empty(t, md.HTML)
	assert.Nil(t, md.Canvas)

	page, err := f.svc.Document(ctx, "workspaces/ws", "page.html")
	require.NoError(t, err)
	assert.Equal(t, drive.KindHtml, page.Entry.Kind)
	assert.True(t, strings.HasPrefix(page.HTML, `<html><head><meta http-equiv="Content-Security-Policy"`), page.HTML)
	assert.Contains(t, page.HTML, "<p>x</p>")
	assert.Empty(t, page.Markdown)

	cv, err := f.svc.Document(ctx, "workspaces/ws", "canvases/plan.canvas.json")
	require.NoError(t, err)
	assert.Equal(t, drive.KindCanvas, cv.Entry.Kind)
	require.NotNil(t, cv.Canvas)
	assert.Equal(t, "<p>ok</p>", cv.Canvas.Blocks[0].Body, "canvas html blocks leave sanitized")

	file, err := f.svc.Document(ctx, "workspaces/ws", "data.bin")
	require.NoError(t, err)
	assert.Equal(t, drive.KindFile, file.Entry.Kind)
	assert.Empty(t, file.Markdown)
	assert.Empty(t, file.HTML)
	assert.Nil(t, file.Canvas)

	for _, tc := range []struct {
		path string
		want Kind
	}{
		{"plans", KindInvalid},
		{"", KindInvalid},
		{"../escape.md", KindInvalid},
		{"/etc/hosts", KindInvalid},
		{"nope.md", KindNotFound},
		{"nope", KindNotFound},
	} {
		_, err := f.svc.Document(ctx, "workspaces/ws", tc.path)
		assert.Equal(t, tc.want, KindOf(err), "Document(%q)", tc.path)
	}
	_, err = f.svc.Document(ctx, "nowhere", "a.md")
	assert.Equal(t, KindInvalid, KindOf(err))
}

func TestDriveMarkdownOfAMarkdownDocumentIsItsBody(t *testing.T) {
	f := newDriveFixture(t)
	f.writeDoc(t, "workspaces/ws", "notes.md", "# Notes\n")

	got, err := f.svc.Markdown(t.Context(), "workspaces/ws", "notes.md")
	require.NoError(t, err)
	assert.Equal(t, "# Notes\n", got)
}

func TestDriveSpacesTreeAndSearch(t *testing.T) {
	f := newDriveFixture(t)
	ctx := t.Context()
	f.writeDoc(t, "acme/site", "plans/release-plan.md", "---\ntitle: Release\n---\n")
	f.writeDoc(t, "workspaces/ws", "notes/plan-b.md", "b")

	spaces, err := f.svc.Spaces(ctx)
	require.NoError(t, err)
	keys := make([]string, 0, len(spaces))
	for _, sp := range spaces {
		keys = append(keys, sp.Key)
	}
	assert.Equal(t, []string{"acme/site", "workspaces/ws", "shared"}, keys)

	root, err := f.svc.Tree(ctx, "acme/site")
	require.NoError(t, err)
	entry, ok := drive.Find(root, "plans/release-plan.md")
	require.True(t, ok)
	assert.Equal(t, "Release", entry.Title)

	empty, err := f.svc.Tree(ctx, "shared")
	require.NoError(t, err)
	assert.Empty(t, empty.Children, "a space with no directory is an empty root")

	_, err = f.svc.Tree(ctx, "../acme")
	assert.Equal(t, KindInvalid, KindOf(err))

	hits, err := f.svc.Search(ctx, "plan", 0)
	require.NoError(t, err)
	var found []string
	for _, h := range hits {
		found = append(found, h.Space.Key+":"+h.Entry.Path)
	}
	assert.Equal(t, []string{"acme/site:plans", "acme/site:plans/release-plan.md", "workspaces/ws:notes/plan-b.md"}, found)

	limited, err := f.svc.Search(ctx, "plan", 1)
	require.NoError(t, err)
	assert.Len(t, limited, 1)
	_, err = f.svc.Search(ctx, "plan", -1)
	assert.Equal(t, KindInvalid, KindOf(err))
}

func TestDriveReorganizes(t *testing.T) {
	f := newDriveFixture(t)
	ctx := t.Context()
	f.writeDoc(t, "workspaces/ws", "plans/a.md", "a")

	folder, err := f.svc.CreateFolder(ctx, "workspaces/ws", "reports/2026")
	require.NoError(t, err)
	assert.Equal(t, drive.KindDir, folder.Kind)
	assert.DirExists(t, filepath.Join(f.contextDir(), "workspaces", "ws", "reports", "2026"))

	renamed, err := f.svc.Rename(ctx, "workspaces/ws", "plans/a.md", "b")
	require.NoError(t, err)
	assert.Equal(t, "plans/b.md", renamed.Path)

	moved, err := f.svc.Move(ctx, "workspaces/ws", "plans/b.md", "reports/2026")
	require.NoError(t, err)
	assert.Equal(t, "reports/2026/b.md", moved.Path)

	atRoot, err := f.svc.Move(ctx, "workspaces/ws", "reports/2026/b.md", "")
	require.NoError(t, err)
	assert.Equal(t, "b.md", atRoot.Path, "a move to the root keeps the base name")

	folderRenamed, err := f.svc.Rename(ctx, "workspaces/ws", "reports", "archive")
	require.NoError(t, err)
	assert.Equal(t, "archive", folderRenamed.Path)
	assert.Equal(t, []string{"archive/2026"}, entryPaths(folderRenamed.Children))

	same, err := f.svc.Move(ctx, "workspaces/ws", "b.md", "")
	require.NoError(t, err, "a move into the current folder changes nothing")
	assert.Equal(t, "b.md", same.Path)

	require.NoError(t, f.svc.Delete(ctx, "workspaces/ws", "b.md"))
	require.NoError(t, f.svc.Delete(ctx, "workspaces/ws", "archive/2026"))

	root, err := f.svc.Tree(ctx, "workspaces/ws")
	require.NoError(t, err)
	assert.Equal(t, []string{"archive", "plans"}, entryPaths(root.Children))

	updates := f.signals.waitUpdates(t, 7)
	for _, u := range updates {
		assert.Equal(t, "workspaces/ws", u.Space)
		assert.Empty(t, u.Session, "a user's change has no agent author")
		assert.Empty(t, u.HiveSession)
	}
	assert.Equal(t, []string{"plans/a.md", "plans/b.md"}, updates[1].Paths)
	assert.Equal(t, []string{"plans/b.md", "reports/2026/b.md"}, updates[2].Paths)
}

func entryPaths(entries []drive.Entry) []string {
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		out = append(out, e.Path)
	}
	return out
}

func TestDriveRenameKeepsTheKind(t *testing.T) {
	f := newDriveFixture(t)
	ctx := t.Context()
	f.writeDoc(t, "workspaces/ws", "a.md", "a")
	f.writeDoc(t, "workspaces/ws", "c.md", "c")
	f.writeDoc(t, "workspaces/ws", "page.html", "<p>x</p>")
	f.writeDoc(t, "workspaces/ws", "data.json", "{}")
	f.writeDoc(t, "workspaces/ws", "canvases/plan.canvas.json", `{"blocks":[]}`)

	for _, tc := range []struct {
		path, name, want string
	}{
		{"a.md", "b", "b.md"},
		{"b.md", "d.md", "d.md"},
		{"page.html", "index", "index.html"},
		{"data.json", "other", "other.json"},
		{"other.json", "other.csv", "other.csv"},
		{"canvases/plan.canvas.json", "roadmap", "canvases/roadmap.canvas.json"},
	} {
		got, err := f.svc.Rename(ctx, "workspaces/ws", tc.path, tc.name)
		require.NoError(t, err, "Rename(%q, %q)", tc.path, tc.name)
		assert.Equal(t, tc.want, got.Path, "Rename(%q, %q)", tc.path, tc.name)
	}

	for _, tc := range []struct {
		path, name string
		want       Kind
	}{
		{"d.md", "d.html", KindInvalid},
		{"index.html", "index.md", KindInvalid},
		{"other.csv", "x.md", KindInvalid},
		{"d.md", "x.canvas.json", KindInvalid},
		{"canvases/roadmap.canvas.json", "Road Map", KindInvalid},
		{"d.md", "sub/x", KindInvalid},
		{"d.md", ".hidden", KindInvalid},
		{"d.md", "..", KindInvalid},
		{"d.md", "", KindInvalid},
		{"d.md", "c", KindConflict},
		{"ghost.md", "x", KindNotFound},
		{"../d.md", "x", KindInvalid},
		{"/d.md", "x", KindInvalid},
		{"", "x", KindInvalid},
	} {
		_, err := f.svc.Rename(ctx, "workspaces/ws", tc.path, tc.name)
		assert.Equal(t, tc.want, KindOf(err), "Rename(%q, %q): %v", tc.path, tc.name, err)
	}
	assert.FileExists(t, filepath.Join(f.contextDir(), "workspaces", "ws", "d.md"), "a refused rename leaves the file")
}

func TestDriveMoveAndCreateFolderRefusals(t *testing.T) {
	f := newDriveFixture(t)
	ctx := t.Context()
	f.writeDoc(t, "workspaces/ws", "a.md", "a")
	f.writeDoc(t, "workspaces/ws", "plans/a.md", "taken")
	f.writeDoc(t, "workspaces/ws", "plans/sub/x.md", "x")

	for _, tc := range []struct {
		path, dir string
		want      Kind
	}{
		{"a.md", "plans", KindConflict},
		{"a.md", "ghost", KindNotFound},
		{"a.md", "plans/a.md", KindInvalid},
		{"plans", "plans/sub", KindInvalid},
		{"../a.md", "plans", KindInvalid},
		{"a.md", "../elsewhere", KindInvalid},
		{"a.md", "/tmp", KindInvalid},
		{"ghost.md", "plans", KindNotFound},
		{"", "plans", KindInvalid},
	} {
		_, err := f.svc.Move(ctx, "workspaces/ws", tc.path, tc.dir)
		assert.Equal(t, tc.want, KindOf(err), "Move(%q, %q): %v", tc.path, tc.dir, err)
	}

	_, err := f.svc.CreateFolder(ctx, "workspaces/ws", "plans")
	assert.Equal(t, KindConflict, KindOf(err))
	_, err = f.svc.CreateFolder(ctx, "workspaces/ws", "../out")
	assert.Equal(t, KindInvalid, KindOf(err))
	_, err = f.svc.CreateFolder(ctx, "workspaces/ws", "")
	assert.Equal(t, KindInvalid, KindOf(err))

	err = f.svc.Delete(ctx, "workspaces/ws", "plans")
	assert.Equal(t, KindInvalid, KindOf(err))
	assert.Contains(t, err.Error(), "not empty")
	assert.Equal(t, KindInvalid, KindOf(f.svc.Delete(ctx, "workspaces/ws", "..")))
	assert.Equal(t, KindInvalid, KindOf(f.svc.Delete(ctx, "workspaces/ws", "")))
	assert.Equal(t, KindNotFound, KindOf(f.svc.Delete(ctx, "workspaces/ws", "ghost.md")))

	f.signals.requireNoUpdates(t, "a refused change publishes nothing")
}

func TestDriveLocateAndReveal(t *testing.T) {
	f := newDriveFixture(t)
	var revealed []string
	f.svc.reveal = func(path string) error {
		revealed = append(revealed, path)
		return nil
	}
	ctx := t.Context()
	f.writeDoc(t, "workspaces/ws", "plans/a.md", "a")
	spaceDir := filepath.Join(f.contextDir(), "workspaces", "ws")

	got, err := f.svc.Locate(ctx, "workspaces/ws", "plans/a.md")
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(spaceDir, "plans", "a.md"), got)

	root, err := f.svc.Locate(ctx, "workspaces/ws", "")
	require.NoError(t, err)
	assert.Equal(t, spaceDir, root)

	for _, tc := range []struct {
		path string
		want Kind
	}{
		{"ghost.md", KindNotFound},
		{"../ws2/a.md", KindInvalid},
		{"/etc/passwd", KindInvalid},
	} {
		_, err := f.svc.Locate(ctx, "workspaces/ws", tc.path)
		assert.Equal(t, tc.want, KindOf(err), "Locate(%q): %v", tc.path, err)
	}
	_, err = f.svc.Locate(ctx, "../outside", "a.md")
	assert.Equal(t, KindInvalid, KindOf(err))

	require.NoError(t, f.svc.Reveal(ctx, "workspaces/ws", "plans"))
	assert.Equal(t, []string{filepath.Join(spaceDir, "plans")}, revealed)
	assert.Equal(t, KindNotFound, KindOf(f.svc.Reveal(ctx, "workspaces/ws", "ghost.md")))
	assert.Len(t, revealed, 1, "a path that is not there is never handed to the file manager")
}
