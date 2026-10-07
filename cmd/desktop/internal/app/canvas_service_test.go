package app

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/colonyops/hive/cmd/desktop/internal/app/canvas"
	"github.com/colonyops/hive/cmd/desktop/internal/app/data/stores"
	"github.com/colonyops/hive/cmd/desktop/internal/app/events"
	"github.com/colonyops/hive/internal/config"
	"github.com/colonyops/hive/internal/domain/session"
	"github.com/colonyops/hive/internal/hive/repocontext"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeCanvasSessions map[string]stores.AgentSession

func (f fakeCanvasSessions) Get(_ context.Context, id string) (stores.AgentSession, error) {
	rec, ok := f[id]
	if !ok {
		return stores.AgentSession{}, stores.NotFoundError{Entity: "agent_workspace_session", Key: id}
	}
	return rec, nil
}

type canvasToggle struct {
	session string
	name    string
	open    bool
}

// canvasSignals mirrors the two events CanvasService publishes. Delivery
// runs on the bus subscriber's own goroutine, so every field is guarded by a
// mutex and read through the wait helpers below rather than directly.
type canvasSignals struct {
	bus     *events.Bus
	mu      sync.Mutex
	updates []string
	toggles []canvasToggle
}

func (s *canvasSignals) addUpdate(session string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.updates = append(s.updates, session)
}

func (s *canvasSignals) addToggle(tg canvasToggle) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.toggles = append(s.toggles, tg)
}

func (s *canvasSignals) Updates() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.updates...)
}

func (s *canvasSignals) Toggles() []canvasToggle {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]canvasToggle(nil), s.toggles...)
}

// waitUpdates polls until exactly n updates have been recorded. Publish hands
// off to the bus subscriber's own goroutine, so a test asserting a positive
// count has to wait rather than read signals.Updates() synchronously.
func (s *canvasSignals) waitUpdates(t *testing.T, n int) []string {
	t.Helper()
	require.Eventually(t, func() bool { return len(s.Updates()) == n }, 2*time.Second, 5*time.Millisecond)
	return s.Updates()
}

func (s *canvasSignals) waitToggles(t *testing.T, n int) []canvasToggle {
	t.Helper()
	require.Eventually(t, func() bool { return len(s.Toggles()) == n }, 2*time.Second, 5*time.Millisecond)
	return s.Toggles()
}

// requireNoUpdates proves the service published nothing so far. Buffer
// delivers in order, so a sentinel published now lands behind anything the
// service already published; once it arrives the recorded list is complete,
// which a synchronous read of Updates() could never guarantee.
func (s *canvasSignals) requireNoUpdates(t *testing.T, msg string) {
	t.Helper()
	const sentinel = "sentinel"
	s.bus.Publish(t.Context(), events.CanvasUpdated{Session: sentinel})
	require.Eventually(t, func() bool { return slices.Contains(s.Updates(), sentinel) }, 2*time.Second, 5*time.Millisecond)
	assert.Equal(t, []string{sentinel}, s.Updates(), msg)
}

func testCanvasService(t *testing.T) (*CanvasService, *canvasSignals) {
	t.Helper()
	bus := newTestBus(t)
	signals := &canvasSignals{bus: bus}
	sessions := fakeCanvasSessions{
		"1": {ID: "1", Workspace: "ws", Name: "chat", Agent: "claude"},
	}
	cancelUpdated := events.Subscribe(t.Context(), bus, "test.canvas-updated", events.Buffer(64), func(_ context.Context, e events.CanvasUpdated) {
		signals.addUpdate(e.Session)
	})
	t.Cleanup(cancelUpdated)
	cancelToggled := events.Subscribe(t.Context(), bus, "test.canvas-toggled", events.Buffer(64), func(_ context.Context, e events.CanvasToggleRequested) {
		signals.addToggle(canvasToggle{e.Session, e.Name, e.Open})
	})
	t.Cleanup(cancelToggled)
	svc := newCanvasService(CanvasDeps{
		Store:    canvas.NewStore(canvas.Roots{Workspaces: t.TempDir()}),
		Sessions: sessions,
		Events:   bus,
	})
	return svc, signals
}

func TestCanvasUnknownSessionIsNotFound(t *testing.T) {
	svc, _ := testCanvasService(t)
	ctx := t.Context()

	_, err := svc.Get(ctx, "99", "plan")
	assert.Equal(t, KindNotFound, KindOf(err))
	_, err = svc.PutBlock(ctx, "99", "plan", "", "", canvas.Block{ID: "a", Kind: canvas.KindMarkdown, Body: "x"})
	assert.Equal(t, KindNotFound, KindOf(err))
	_, err = svc.RemoveBlock(ctx, "99", "plan", "a")
	assert.Equal(t, KindNotFound, KindOf(err))
	_, err = svc.Clear(ctx, "99", "plan")
	assert.Equal(t, KindNotFound, KindOf(err))
	err = svc.Delete(ctx, "99", "plan")
	assert.Equal(t, KindNotFound, KindOf(err))
	_, err = svc.List(ctx, "99")
	assert.Equal(t, KindNotFound, KindOf(err))
}

func TestCanvasGetUnknownNameIsNotFound(t *testing.T) {
	svc, signals := testCanvasService(t)

	_, err := svc.Get(t.Context(), "1", "plan")
	assert.Equal(t, KindNotFound, KindOf(err), "an agent asking by name should learn the name is wrong")
	signals.requireNoUpdates(t, "a read never notifies")
}

func TestCanvasGetForWorkspaceUnwrittenAnswersEmpty(t *testing.T) {
	svc, signals := testCanvasService(t)

	c, err := svc.GetForOwner(t.Context(), "ws", "plan")
	require.NoError(t, err)
	assert.Equal(t, "ws", c.Workspace)
	assert.Equal(t, "plan", c.Name)
	assert.NotNil(t, c.Blocks)
	assert.Empty(t, c.Blocks)
	signals.requireNoUpdates(t, "a read never notifies")
}

func TestCanvasPutBlockValidation(t *testing.T) {
	svc, signals := testCanvasService(t)
	ctx := t.Context()

	cases := map[string]canvas.Block{
		"empty id":              {ID: "  ", Kind: canvas.KindMarkdown, Body: "x"},
		"id too long":           {ID: strings.Repeat("a", 201), Kind: canvas.KindMarkdown, Body: "x"},
		"unknown kind":          {ID: "a", Kind: "diagram", Body: "x"},
		"markdown without body": {ID: "a", Kind: canvas.KindMarkdown},
		"markdown with url":     {ID: "a", Kind: canvas.KindMarkdown, Body: "x", URL: "https://example.com"},
		"oversized body":        {ID: "a", Kind: canvas.KindMarkdown, Body: strings.Repeat("x", maxCanvasBodyBytes+1)},
		"html without body":     {ID: "a", Kind: canvas.KindHTML},
		"html with url":         {ID: "a", Kind: canvas.KindHTML, Body: "<p>x</p>", URL: "https://example.com"},
		"html with a script":    {ID: "a", Kind: canvas.KindHTML, Body: "<p>x</p><script>alert(1)</script>"},
		"html with a handler":   {ID: "a", Kind: canvas.KindHTML, Body: `<div onclick="x()">x</div>`},
		"html with an id":       {ID: "a", Kind: canvas.KindHTML, Body: `<div id="wails">x</div>`},
		"html with a bad link":  {ID: "a", Kind: canvas.KindHTML, Body: `<a href="javascript:alert(1)">x</a>`},
		"html with a bad image": {ID: "a", Kind: canvas.KindHTML, Body: `<img src="javascript:alert(1)">`},
		"link without title":    {ID: "a", Kind: canvas.KindLink, URL: "https://example.com"},
		"link without url":      {ID: "a", Kind: canvas.KindLink, Title: "t"},
		"link with body":        {ID: "a", Kind: canvas.KindLink, Title: "t", URL: "https://example.com", Body: "x"},
		"javascript url":        {ID: "a", Kind: canvas.KindLink, Title: "t", URL: "javascript:alert(1)"},
	}
	for name, block := range cases {
		_, err := svc.PutBlock(ctx, "1", "plan", "", "", block)
		assert.Equal(t, KindInvalid, KindOf(err), name)
	}

	_, err := svc.PutBlock(ctx, "1", "Bad Name", "", "", canvas.Block{ID: "a", Kind: canvas.KindMarkdown, Body: "x"})
	assert.Equal(t, KindInvalid, KindOf(err), "an invalid canvas name is the caller's mistake")
	_, err = svc.PutBlock(ctx, "1", "plan", strings.Repeat("t", maxCanvasTitleLength+1), "", canvas.Block{ID: "a", Kind: canvas.KindMarkdown, Body: "x"})
	assert.Equal(t, KindInvalid, KindOf(err), "an oversized canvas title is refused")

	signals.requireNoUpdates(t, "a refused write never notifies")
}

func TestCanvasTitlesDecodeHTMLEntities(t *testing.T) {
	svc, _ := testCanvasService(t)

	c, err := svc.PutBlock(t.Context(), "1", "plan", "Rice &amp; Peppers", "", canvas.Block{
		ID: "a", Kind: canvas.KindLink, Title: "Q&amp;A &lt;draft&gt;", URL: "https://example.com",
	})
	require.NoError(t, err)
	assert.Equal(t, "Rice & Peppers", c.Title)
	assert.Equal(t, "Q&A <draft>", c.Blocks[0].Title)
}

// A silently stripped tag is the one failure an agent cannot see, so the
// error names it. How a block looks is the agent's own business, so styling
// it never reaches this path.
func TestCanvasHTMLBlockRejectionNamesTheOffender(t *testing.T) {
	svc, _ := testCanvasService(t)
	ctx := t.Context()

	_, err := svc.PutBlock(ctx, "1", "plan", "", "", canvas.Block{
		ID: "a", Kind: canvas.KindHTML, Body: `<div><iframe src="https://x.example"></iframe></div>`,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "<iframe>")

	_, err = svc.PutBlock(ctx, "1", "plan", "", "", canvas.Block{
		ID: "a", Kind: canvas.KindHTML,
		Body: `<div class="hv-grid hv-cols-2"><span class="hv-badge hv-warn">2 flaky</span></div>`,
	})
	require.NoError(t, err, "the vocabulary itself is accepted")

	_, err = svc.PutBlock(ctx, "1", "plan", "", "", canvas.Block{
		ID: "a", Kind: canvas.KindHTML,
		Body: `<div class="mystery" style="color:#f00"><img src="https://example.com/a.png"></div>`,
	})
	require.NoError(t, err, "a class, a style and an image are the agent's own business")
}

// The pane's read is the seam between stored agent source and the app's own
// webview; the agent's read-back is not, so the two disagree by design.
func TestCanvasHTMLIsSanitizedOnTheWayOutNotIn(t *testing.T) {
	svc, _ := testCanvasService(t)
	ctx := t.Context()

	// Written straight to the store: validateBlock would have refused this,
	// which is exactly why the read path cannot rely on it.
	_, err := svc.store.Upsert("ws", "plan", canvas.Author{Session: "1"}, "", "", canvas.Block{
		ID: "a", Kind: canvas.KindHTML, Body: `<p class="hv-muted">ok</p><script>alert(1)</script>`,
	})
	require.NoError(t, err)

	shown, err := svc.GetForOwner(ctx, "ws", "plan")
	require.NoError(t, err)
	assert.Equal(t, `<p class="hv-muted">ok</p>`, shown.Blocks[0].Body)

	stored, err := svc.Get(ctx, "1", "plan")
	require.NoError(t, err)
	assert.Contains(t, stored.Blocks[0].Body, "<script>", "read_canvas shows the agent what it wrote")
}

func TestCanvasReadsReportAMigratedChatByItsUUID(t *testing.T) {
	const chatID = "0199bd92-cc5f-7c61-9185-7423813c9c12"
	svc, _ := testCanvasService(t)
	migrated := stores.AgentSession{ID: chatID, LegacyID: "7", Workspace: "ws", Name: "migrated", Agent: "claude"}
	svc.sessions = fakeCanvasSessions{"7": migrated, chatID: migrated}
	ctx := t.Context()

	for _, author := range []string{"7", "8"} {
		_, err := svc.store.Upsert("ws", "by-"+author, canvas.Author{Session: author}, "", "", canvas.Block{
			ID: "a", Kind: canvas.KindMarkdown, Body: "x",
		})
		require.NoError(t, err)
	}

	shown, err := svc.GetForOwner(ctx, "ws", "by-7")
	require.NoError(t, err)
	assert.Equal(t, chatID, shown.Session)

	read, err := svc.Get(ctx, chatID, "by-7")
	require.NoError(t, err)
	assert.Equal(t, chatID, read.Session)

	metas, err := svc.ListForOwner(ctx, "ws")
	require.NoError(t, err)
	authors := map[string]string{}
	for _, meta := range metas {
		authors[meta.Name] = meta.Session
	}
	assert.Equal(t, map[string]string{"by-7": chatID, "by-8": "8"}, authors, "a deleted chat keeps its old number")
}

func TestCanvasPutBlocks(t *testing.T) {
	svc, signals := testCanvasService(t)
	ctx := t.Context()

	_, err := svc.PutBlocks(ctx, "1", "plan", "The Plan", nil)
	assert.Equal(t, KindInvalid, KindOf(err), "an empty batch is refused")

	_, err = svc.PutBlocks(ctx, "1", "plan", "", []canvas.Block{
		{ID: "a", Kind: canvas.KindMarkdown, Body: "x"},
		{ID: "b", Kind: canvas.KindLink, Title: "t"},
	})
	assert.Equal(t, KindInvalid, KindOf(err))
	assert.Contains(t, err.Error(), "blocks[1]", "the error names which block was rejected")

	_, err = svc.PutBlocks(ctx, "1", "plan", "", []canvas.Block{
		{ID: "a", Kind: canvas.KindMarkdown, Body: "x"},
		{ID: "a", Kind: canvas.KindMarkdown, Body: "y"},
	})
	assert.Equal(t, KindInvalid, KindOf(err), "duplicate ids in one batch are refused")

	metas, err := svc.ListForOwner(ctx, "ws")
	require.NoError(t, err)
	assert.Empty(t, metas, "a rejected batch writes nothing")
	assert.Empty(t, signals.Updates())

	c, err := svc.PutBlocks(ctx, "1", "plan", "The Plan", []canvas.Block{
		{ID: "a", Kind: canvas.KindMarkdown, Body: "x"},
		{ID: "b", Kind: canvas.KindMarkdown, Body: "y"},
	})
	require.NoError(t, err)
	assert.Len(t, c.Blocks, 2)
	signals.waitUpdates(t, 1)

	oversized := make([]canvas.Block, maxCanvasBatchBlocks+1)
	for i := range oversized {
		oversized[i] = canvas.Block{ID: strings.Repeat("a", i+1), Kind: canvas.KindMarkdown, Body: "x"}
	}
	_, err = svc.PutBlocks(ctx, "1", "plan", "", oversized)
	assert.Equal(t, KindInvalid, KindOf(err))
}

func TestCanvasPutBlockBeforeAnchor(t *testing.T) {
	svc, _ := testCanvasService(t)
	ctx := t.Context()

	_, err := svc.PutBlock(ctx, "1", "plan", "", "", canvas.Block{ID: "a", Kind: canvas.KindMarkdown, Body: "x"})
	require.NoError(t, err)

	c, err := svc.PutBlock(ctx, "1", "plan", "", "a", canvas.Block{ID: "intro", Kind: canvas.KindMarkdown, Body: "i"})
	require.NoError(t, err)
	assert.Equal(t, "intro", c.Blocks[0].ID)

	_, err = svc.PutBlock(ctx, "1", "plan", "", "ghost", canvas.Block{ID: "x", Kind: canvas.KindMarkdown, Body: "x"})
	assert.Equal(t, KindNotFound, KindOf(err), "a missing anchor is the caller's mistake")
}

func TestCanvasMutationsNotify(t *testing.T) {
	svc, signals := testCanvasService(t)
	ctx := t.Context()

	_, err := svc.PutBlock(ctx, "1", "plan", "The Plan", "", canvas.Block{ID: "a", Kind: canvas.KindMarkdown, Body: "x"})
	require.NoError(t, err)
	_, err = svc.PutBlock(ctx, "1", "plan", "", "", canvas.Block{ID: "pr", Kind: canvas.KindLink, Title: "PR", URL: "https://example.com/pr/1"})
	require.NoError(t, err)
	_, err = svc.RemoveBlock(ctx, "1", "plan", "pr")
	require.NoError(t, err)
	c, err := svc.Clear(ctx, "1", "plan")
	require.NoError(t, err)
	assert.Empty(t, c.Blocks)
	assert.Equal(t, "The Plan", c.Title)
	err = svc.Delete(ctx, "1", "plan")
	require.NoError(t, err)

	updates := signals.waitUpdates(t, 5)
	for _, update := range updates {
		assert.Equal(t, "1", update)
	}
}

func TestCanvasMutationsOnUnknownCanvasAreNotFound(t *testing.T) {
	svc, signals := testCanvasService(t)
	ctx := t.Context()

	_, err := svc.RemoveBlock(ctx, "1", "ghost", "a")
	assert.Equal(t, KindNotFound, KindOf(err))
	_, err = svc.Clear(ctx, "1", "ghost")
	assert.Equal(t, KindNotFound, KindOf(err))
	err = svc.Delete(ctx, "1", "ghost")
	assert.Equal(t, KindNotFound, KindOf(err))
	signals.requireNoUpdates(t, "a mutation on an unknown canvas never notifies")
}

func TestCanvasSetPaneOpen(t *testing.T) {
	svc, signals := testCanvasService(t)
	ctx := t.Context()

	require.NoError(t, svc.SetPaneOpen(ctx, "1", "", true), "opening without a name leaves the pane's own pick")
	err := svc.SetPaneOpen(ctx, "1", "ghost", true)
	assert.Equal(t, KindNotFound, KindOf(err), "opening pinned to a canvas requires it to exist")

	_, err = svc.PutBlock(ctx, "1", "plan", "", "", canvas.Block{ID: "a", Kind: canvas.KindMarkdown, Body: "x"})
	require.NoError(t, err)
	require.NoError(t, svc.SetPaneOpen(ctx, "1", "plan", true))
	require.NoError(t, svc.SetPaneOpen(ctx, "1", "", false))
	err = svc.SetPaneOpen(ctx, "99", "", true)
	assert.Equal(t, KindNotFound, KindOf(err))

	assert.Equal(t, []canvasToggle{
		{"1", "", true},
		{"1", "plan", true},
		{"1", "", false},
	}, signals.waitToggles(t, 3))
	signals.waitUpdates(t, 1)
}

func TestCanvasRemoveAbsentBlockIsNotFound(t *testing.T) {
	svc, _ := testCanvasService(t)
	ctx := t.Context()
	_, err := svc.PutBlock(ctx, "1", "plan", "", "", canvas.Block{ID: "a", Kind: canvas.KindMarkdown, Body: "x"})
	require.NoError(t, err)
	_, err = svc.RemoveBlock(ctx, "1", "plan", "ghost")
	assert.Equal(t, KindNotFound, KindOf(err))
}

func TestCanvasExport(t *testing.T) {
	svc, signals := testCanvasService(t)
	ctx := t.Context()

	_, err := svc.MarkdownForOwner(ctx, "ws", "ghost")
	assert.Equal(t, KindNotFound, KindOf(err))

	_, err = svc.PutBlock(ctx, "1", "plan", "The Plan", "", canvas.Block{ID: "a", Kind: canvas.KindMarkdown, Body: "hello"})
	require.NoError(t, err)

	markdown, err := svc.MarkdownForOwner(ctx, "ws", "plan")
	require.NoError(t, err)
	assert.Equal(t, "# The Plan\n\nhello\n", markdown)

	err = svc.ExportForOwner(ctx, "ws", "plan", "relative.md")
	assert.Equal(t, KindInvalid, KindOf(err), "the save dialog hands back absolute paths; anything else is a caller bug")

	dest := filepath.Join(t.TempDir(), "plan.md")
	require.NoError(t, svc.ExportForOwner(ctx, "ws", "plan", dest))
	written, err := os.ReadFile(dest)
	require.NoError(t, err)
	assert.Equal(t, markdown, string(written))

	signals.waitUpdates(t, 1)
}

func TestCanvasListForOwner(t *testing.T) {
	svc, _ := testCanvasService(t)
	ctx := t.Context()

	metas, err := svc.ListForOwner(ctx, "ws")
	require.NoError(t, err)
	assert.Empty(t, metas)

	_, err = svc.PutBlock(ctx, "1", "plan", "The Plan", "", canvas.Block{ID: "a", Kind: canvas.KindMarkdown, Body: "x"})
	require.NoError(t, err)
	metas, err = svc.ListForOwner(ctx, "ws")
	require.NoError(t, err)
	require.Len(t, metas, 1)
	assert.Equal(t, "plan", metas[0].Name)
	assert.Equal(t, "The Plan", metas[0].Title)
	assert.Equal(t, "1", metas[0].Session)

	fromSession, err := svc.List(ctx, "1")
	require.NoError(t, err)
	assert.Equal(t, metas, fromSession, "List resolves the session's workspace and answers the same rows")

	_, err = svc.ListForOwner(ctx, "../escape")
	assert.Equal(t, KindInvalid, KindOf(err))
}

type fakeHiveSessions []session.Session

func (f fakeHiveSessions) SessionAtPath(_ context.Context, path string) (session.Session, bool, error) {
	for _, sess := range f {
		if path == sess.Path || strings.HasPrefix(path, sess.Path+string(filepath.Separator)) {
			return sess, true, nil
		}
	}
	return session.Session{}, false, nil
}

func testHiveCanvasService(t *testing.T, sessions ...session.Session) (*CanvasService, string, func() []events.CanvasUpdated) {
	t.Helper()
	bus := newTestBus(t)
	var mu sync.Mutex
	var updates []events.CanvasUpdated
	cancel := events.Subscribe(t.Context(), bus, "test.hive-canvas-updated", events.Buffer(64), func(_ context.Context, e events.CanvasUpdated) {
		mu.Lock()
		defer mu.Unlock()
		updates = append(updates, e)
	})
	t.Cleanup(cancel)
	contextRoot := t.TempDir()
	svc := newCanvasService(CanvasDeps{
		Store: canvas.NewStore(canvas.Roots{
			Workspaces:   t.TempDir(),
			Repositories: func() string { return contextRoot },
			Global:       t.TempDir(),
		}),
		Sessions:     fakeCanvasSessions{"1": {ID: "1", Workspace: "ws", Name: "chat", Agent: "claude"}},
		HiveSessions: fakeHiveSessions(sessions),
		Events:       bus,
	})
	return svc, contextRoot, func() []events.CanvasUpdated {
		mu.Lock()
		defer mu.Unlock()
		return append([]events.CanvasUpdated(nil), updates...)
	}
}

var siteSession = session.Session{
	ID: "abc123", Name: "fix-login", Path: "/work/site-abc123", Remote: "git@github.com:acme/site.git", State: session.StateActive,
}

// An agent in a hive session has no id to pass, only where it runs. The
// checkout that holds that directory names the repository, and the canvas is
// filed there, outside the checkout.
func TestCanvasFromAHiveSessionIsFiledUnderItsRepository(t *testing.T) {
	svc, contextRoot, updates := testHiveCanvasService(t, siteSession)
	ctx := t.Context()

	c, err := svc.PutBlock(ctx, "/work/site-abc123/cmd/api", "plan", "The Plan", "", canvas.Block{ID: "a", Kind: canvas.KindMarkdown, Body: "x"})
	require.NoError(t, err)
	assert.Equal(t, "acme/site", c.Workspace)
	assert.Equal(t, "abc123", c.HiveSession)
	assert.Empty(t, c.Session)
	require.FileExists(t, filepath.Join(contextRoot, "acme", "site", canvas.DirName, "plan.json"))

	require.Eventually(t, func() bool { return len(updates()) == 1 }, 2*time.Second, 5*time.Millisecond)
	assert.Equal(t, events.CanvasUpdated{HiveSession: "abc123"}, updates()[0])

	read, err := svc.Get(ctx, "/work/site-abc123", "plan")
	require.NoError(t, err)
	assert.Equal(t, "The Plan", read.Title)

	shown, err := svc.GetForOwner(ctx, "acme/site", "plan")
	require.NoError(t, err)
	require.Len(t, shown.Blocks, 1)

	repositories, err := svc.Repositories(ctx)
	require.NoError(t, err)
	assert.Equal(t, []string{"acme/site"}, repositories)
}

// Two sessions of one repository share its canvases, the way two chats share
// a workspace's.
func TestHiveSessionsOfOneRepositoryShareItsCanvases(t *testing.T) {
	other := session.Session{ID: "def456", Name: "add-search", Path: "/work/site-def456", Remote: "https://github.com/acme/site", State: session.StateActive}
	svc, _, _ := testHiveCanvasService(t, siteSession, other)
	ctx := t.Context()

	_, err := svc.PutBlock(ctx, siteSession.Path, "plan", "", "", canvas.Block{ID: "a", Kind: canvas.KindMarkdown, Body: "x"})
	require.NoError(t, err)

	metas, err := svc.List(ctx, other.Path)
	require.NoError(t, err)
	require.Len(t, metas, 1)
	assert.Equal(t, "abc123", metas[0].HiveSession, "the first author stays the provenance")
}

func TestCanvasSessionArgumentThatResolvesToNothing(t *testing.T) {
	noRemote := session.Session{ID: "zzz999", Name: "scratch", Path: "/work/scratch", Remote: "", State: session.StateActive}
	svc, _, _ := testHiveCanvasService(t, siteSession, noRemote)
	ctx := t.Context()
	block := canvas.Block{ID: "a", Kind: canvas.KindMarkdown, Body: "x"}

	_, err := svc.PutBlock(ctx, "/somewhere/else", "plan", "", "", block)
	assert.Equal(t, KindNotFound, KindOf(err), "a directory in no checkout is not filed as global")
	require.ErrorContains(t, err, "global", "the error names the way out")

	_, err = svc.PutBlock(ctx, "site-abc123", "plan", "", "", block)
	assert.Equal(t, KindInvalid, KindOf(err), "neither a chat id nor an absolute path")

	_, err = svc.PutBlock(ctx, "/work/scratch", "plan", "", "", block)
	assert.Equal(t, KindInvalid, KindOf(err), "a session whose remote names no repository")

	_, err = svc.PutBlock(ctx, " 1 ", "plan", "", "", block)
	require.NoError(t, err, "a chat id still resolves, as HIVE_AGENT_SESSION carries it")
}

func TestAgentsOutsideEverySessionShareTheGlobalCanvases(t *testing.T) {
	svc, _, updates := testHiveCanvasService(t, siteSession)
	ctx := t.Context()

	c, err := svc.PutBlock(ctx, "global", "notes", "Notes", "", canvas.Block{ID: "a", Kind: canvas.KindMarkdown, Body: "x"})
	require.NoError(t, err)
	assert.Equal(t, canvas.GlobalOwner, c.Workspace)
	assert.Empty(t, c.Session)
	assert.Empty(t, c.HiveSession)

	_, err = svc.PutBlock(ctx, " global ", "notes", "", "", canvas.Block{ID: "b", Kind: canvas.KindMarkdown, Body: "y"})
	require.NoError(t, err, "a second global agent writes to the same canvas")

	metas, err := svc.ListForOwner(ctx, canvas.GlobalOwner)
	require.NoError(t, err)
	require.Len(t, metas, 1)
	assert.Equal(t, 2, metas[0].BlockCount)
	require.Eventually(t, func() bool { return len(updates()) == 2 }, 2*time.Second, 5*time.Millisecond, "global writes still wake the readers")

	err = svc.SetPaneOpen(ctx, "global", "notes", true)
	assert.Equal(t, KindInvalid, KindOf(err), "a global canvas has no pane")
}

func TestCanvasPaneToggleNamesTheHiveSession(t *testing.T) {
	bus := newTestBus(t)
	var mu sync.Mutex
	var toggles []events.CanvasToggleRequested
	cancel := events.Subscribe(t.Context(), bus, "test.hive-canvas-toggle", events.Buffer(8), func(_ context.Context, e events.CanvasToggleRequested) {
		mu.Lock()
		defer mu.Unlock()
		toggles = append(toggles, e)
	})
	t.Cleanup(cancel)
	contextRoot := t.TempDir()
	svc := newCanvasService(CanvasDeps{
		Store:        canvas.NewStore(canvas.Roots{Workspaces: t.TempDir(), Repositories: func() string { return contextRoot }}),
		Sessions:     fakeCanvasSessions{},
		HiveSessions: fakeHiveSessions{siteSession},
		Events:       bus,
	})

	require.NoError(t, svc.SetPaneOpen(t.Context(), siteSession.Path, "", true))

	require.Eventually(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(toggles) == 1
	}, 2*time.Second, 5*time.Millisecond)
	assert.Equal(t, events.CanvasToggleRequested{HiveSession: "abc123", Open: true}, toggles[0])
}

// The store restates hive's context layout because it imports nothing, and
// hive's prune skips the canvases directory by name. Both have to keep
// agreeing with the engine.
func TestRepositoryCanvasesAgreeWithHiveContextLayout(t *testing.T) {
	cfg := &config.Config{DataDir: t.TempDir()}
	store := canvas.NewStore(canvas.Roots{Workspaces: t.TempDir(), Repositories: cfg.ContextDir})

	_, err := store.Upsert(canvas.RepositoryOwner("acme", "site"), "plan", canvas.Author{HiveSession: "abc123"}, "", "",
		canvas.Block{ID: "a", Kind: canvas.KindMarkdown, Body: "x"})
	require.NoError(t, err)

	require.FileExists(t, filepath.Join(cfg.RepoContextDir("acme", "site"), repocontext.CanvasesDirName, "plan.json"))
	assert.Equal(t, "acme/site", CanvasOwnerForRemote("git@github.com:acme/site.git"))
	assert.Empty(t, CanvasOwnerForRemote(""))
}
