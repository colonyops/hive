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
	updates []events.CanvasUpdated
	toggles []events.CanvasToggleRequested
}

func (s *canvasSignals) addUpdate(e events.CanvasUpdated) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.updates = append(s.updates, e)
}

func (s *canvasSignals) addToggle(e events.CanvasToggleRequested) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.toggles = append(s.toggles, e)
}

func (s *canvasSignals) Updates() []events.CanvasUpdated {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.updates)
}

func (s *canvasSignals) Toggles() []events.CanvasToggleRequested {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.toggles)
}

// waitUpdates polls until exactly n updates have been recorded. Publish hands
// off to the bus subscriber's own goroutine, so a test asserting a positive
// count has to wait rather than read Updates() synchronously.
func (s *canvasSignals) waitUpdates(t *testing.T, n int) []events.CanvasUpdated {
	t.Helper()
	require.Eventually(t, func() bool { return len(s.Updates()) == n }, eventWait, 5*time.Millisecond)
	return s.Updates()
}

func (s *canvasSignals) waitToggles(t *testing.T, n int) []events.CanvasToggleRequested {
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
	sentinel := events.CanvasUpdated{Session: "sentinel"}
	s.bus.Publish(t.Context(), sentinel)
	require.Eventually(t, func() bool { return slices.Contains(s.Updates(), sentinel) }, eventWait, 5*time.Millisecond)
	assert.Equal(t, []events.CanvasUpdated{sentinel}, s.Updates(), msg)
}

func updateSessions(updates []events.CanvasUpdated) []string {
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
	cancelUpdated := events.Subscribe(t.Context(), bus, "test.canvas-updated", events.Buffer(64), func(_ context.Context, e events.CanvasUpdated) {
		signals.addUpdate(e)
	})
	t.Cleanup(cancelUpdated)
	cancelToggled := events.Subscribe(t.Context(), bus, "test.canvas-toggled", events.Buffer(64), func(_ context.Context, e events.CanvasToggleRequested) {
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

	metas, err := f.svc.ListCanvasesAt(ctx, "workspaces/ws")
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

	metas, err := f.svc.ListCanvasesAt(ctx, "workspaces/ws")
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

	assert.Equal(t, []events.CanvasToggleRequested{
		{Session: "1", Open: true},
		{Session: "1", Name: "plan", Open: true},
		{Session: "1", Name: "plan", Open: true},
		{Session: "1", Open: false},
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

func TestCanvasDeleteAt(t *testing.T) {
	f := newDriveFixture(t)
	ctx := t.Context()

	_, err := f.svc.PutBlock(ctx, "1", "plan", "The Plan", "", markdownBlock("a", "hello"))
	require.NoError(t, err)
	require.NoError(t, f.svc.DeleteAt(ctx, "workspaces/ws", "plan"))
	err = f.svc.DeleteAt(ctx, "workspaces/ws", "plan")
	assert.Equal(t, KindNotFound, KindOf(err))

	updates := f.signals.waitUpdates(t, 2)
	assert.Equal(t, []string{"1", "1"}, updateSessions(updates), "the delete wakes the chat that wrote the canvas")
}

func TestCanvasListAt(t *testing.T) {
	f := newDriveFixture(t)
	ctx := t.Context()

	metas, err := f.svc.ListCanvasesAt(ctx, "workspaces/ws")
	require.NoError(t, err)
	assert.Empty(t, metas)

	_, err = f.svc.PutBlock(ctx, "1", "plan", "The Plan", "", markdownBlock("a", "x"))
	require.NoError(t, err)
	metas, err = f.svc.ListCanvasesAt(ctx, "workspaces/ws")
	require.NoError(t, err)
	require.Len(t, metas, 1)
	assert.Equal(t, "workspaces/ws", metas[0].Space)
	assert.Equal(t, "canvases/plan.canvas.json", metas[0].Path)
	assert.Equal(t, "The Plan", metas[0].Title)
	assert.Equal(t, "1", metas[0].Session)

	fromSession, err := f.svc.ListCanvases(ctx, "1")
	require.NoError(t, err)
	assert.Equal(t, metas, fromSession, "ListCanvases resolves the session's space and answers the same rows")

	_, err = f.svc.ListCanvasesAt(ctx, "workspaces/../escape")
	assert.Equal(t, KindInvalid, KindOf(err))
	_, err = f.svc.ListCanvasesAt(ctx, "ws")
	assert.Equal(t, KindInvalid, KindOf(err), "a bare workspace name is an owner key, not a space key")
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
	assert.Equal(t, events.CanvasUpdated{HiveSession: "abc123"}, updates[0])

	read, err := f.svc.GetCanvas(ctx, "/work/site-abc123", "plan")
	require.NoError(t, err)
	assert.Equal(t, "The Plan", read.Title)

	shown, err := f.svc.GetCanvasAt(ctx, "acme/site", "plan")
	require.NoError(t, err)
	require.Len(t, shown.Blocks, 1)

	repositories, err := f.svc.Repositories(ctx)
	require.NoError(t, err)
	assert.Equal(t, []string{"acme/site"}, repositories, "the workspace space is not a repository")
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

	metas, err := f.svc.ListCanvasesAt(ctx, drive.SharedKey)
	require.NoError(t, err)
	require.Len(t, metas, 1)
	assert.Equal(t, 2, metas[0].BlockCount)
	updates := f.signals.waitUpdates(t, 2)
	assert.Equal(t, []events.CanvasUpdated{{}, {}}, updates, "shared writes still wake the readers, with no author")

	err = f.svc.SetPaneOpen(ctx, "global", "notes", true)
	assert.Equal(t, KindInvalid, KindOf(err), "a shared canvas has no pane")
}

func TestCanvasPaneToggleNamesTheHiveSession(t *testing.T) {
	f := newDriveFixture(t, siteSession)

	require.NoError(t, f.svc.SetPaneOpen(t.Context(), siteSession.Path, "", true))

	toggles := f.signals.waitToggles(t, 1)
	assert.Equal(t, events.CanvasToggleRequested{HiveSession: "abc123", Open: true}, toggles[0])
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

func TestSpaceKeysTranslateOwnerKeysBothWays(t *testing.T) {
	for owner, key := range map[string]string{
		"@global":   "shared",
		"demo":      "workspaces/demo",
		"acme/site": "acme/site",
	} {
		assert.Equal(t, key, SpaceKeyForOwner(owner), owner)
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
	_, err = svc.ListCanvasesAt(ctx, "workspaces/ws")
	assert.Equal(t, KindUnavailable, KindOf(err))
	_, err = svc.Repositories(ctx)
	assert.Equal(t, KindUnavailable, KindOf(err))
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
	metas, err := f.svc.ListCanvasesAt(t.Context(), drive.SharedKey)
	require.NoError(t, err)
	assert.Len(t, metas, 1, "a second run is a no-op")
}
