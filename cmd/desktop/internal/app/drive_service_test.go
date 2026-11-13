package app

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/desktop/internal/app/data/stores"
	"github.com/colonyops/hive/cmd/desktop/internal/app/events"
	"github.com/colonyops/hive/internal/config"
	"github.com/colonyops/hive/internal/domain/drive"
	"github.com/colonyops/hive/internal/domain/session"
	"github.com/colonyops/hive/internal/hive"
	hiveevents "github.com/colonyops/hive/internal/hive/events"
)

type fakeChatSessions map[string]stores.AgentSession

func (f fakeChatSessions) Get(_ context.Context, id string) (stores.AgentSession, error) {
	rec, ok := f[id]
	if !ok {
		return stores.AgentSession{}, stores.NotFoundError{Entity: "agent_workspace_session", Key: id}
	}
	return rec, nil
}

// driveSignals records the drive events that reach the app bus: changes the
// engine publishes, forwarded as the app does, and DriveService's tab requests.
// Delivery runs on the bus subscriber's own goroutine, so every field is
// guarded by a mutex and read through the wait helpers rather than directly.
type driveSignals struct {
	bus     *events.Bus
	mu      sync.Mutex
	updates []events.DriveChanged
	tabs    []events.SessionLayoutRequested
}

func (s *driveSignals) addUpdate(e events.DriveChanged) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.updates = append(s.updates, e)
}

func (s *driveSignals) addTab(e events.SessionLayoutRequested) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tabs = append(s.tabs, e)
}

func (s *driveSignals) Updates() []events.DriveChanged {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.updates)
}

func (s *driveSignals) Tabs() []events.SessionLayoutRequested {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.tabs)
}

// waitUpdates polls until exactly n updates have been recorded. Publish hands
// off to the bus subscriber's own goroutine, so a test asserting a positive
// count has to wait rather than read Updates() synchronously.
func (s *driveSignals) waitUpdates(t *testing.T, n int) []events.DriveChanged {
	t.Helper()
	require.Eventually(t, func() bool { return len(s.Updates()) == n }, eventWait, 5*time.Millisecond)
	return s.Updates()
}

func (s *driveSignals) waitTabs(t *testing.T, n int) []events.SessionLayoutRequested {
	t.Helper()
	require.Eventually(t, func() bool { return len(s.Tabs()) == n }, eventWait, 5*time.Millisecond)
	return s.Tabs()
}

// requireNoUpdates proves the service published nothing so far. Buffer
// delivers in order, so a sentinel published now lands behind anything the
// service already published; once it arrives the recorded list is complete,
// which a synchronous read of Updates() could never guarantee.
func (s *driveSignals) requireNoUpdates(t *testing.T, msg string) {
	t.Helper()
	sentinel := events.DriveChanged{Session: "sentinel"}
	s.bus.Publish(t.Context(), sentinel)
	require.Eventually(t, func() bool {
		return slices.ContainsFunc(s.Updates(), func(e events.DriveChanged) bool { return e.Session == sentinel.Session })
	}, eventWait, 5*time.Millisecond)
	assert.Equal(t, []events.DriveChanged{sentinel}, s.Updates(), msg)
}

type driveFixture struct {
	svc       *DriveService
	signals   *driveSignals
	harness   *hiveHarness
	terminals *fakeTerminals
	layouts   *LayoutsService
}

// openTab opens a view with no window inputs and answers the resolved path.
func (f driveFixture) openTab(ctx context.Context, session, view, path, placement string) (string, error) {
	res, err := f.layouts.OpenTab(ctx, OpenTabRequest{Session: session, View: view, Path: path, Placement: placement})
	return res.Path, err
}

type createdWindow struct{ Slug, Dir, Name string }

// fakeTerminals stands in for tmux: the sessions in running exist, and the
// windows map answers an id or a name with the window's id.
type fakeTerminals struct {
	running  map[string]bool
	windows  map[string]string
	names    map[string]string
	created  []createdWindow
	closed   []string
	closeErr error
}

func (f *fakeTerminals) HasSession(_ context.Context, slug string) (bool, error) {
	return f.running[slug], nil
}

func (f *fakeTerminals) NewWindow(_ context.Context, slug, dir, name string) (string, error) {
	f.created = append(f.created, createdWindow{slug, dir, name})
	return fmt.Sprintf("@%d", 10+len(f.created)), nil
}

func (f *fakeTerminals) FindWindow(_ context.Context, _, idOrName string) (string, error) {
	id, ok := f.windows[idOrName]
	if !ok {
		return "", Errorf(KindNotFound, "no window %q", idOrName)
	}
	return id, nil
}

func (f *fakeTerminals) WindowNames(context.Context, string) (map[string]string, error) {
	return f.names, nil
}

func (f *fakeTerminals) CloseWindow(_ context.Context, slug, windowID string) error {
	if f.closeErr != nil {
		return f.closeErr
	}
	f.closed = append(f.closed, slug+" "+windowID)
	return nil
}

// newDriveFixture is a DriveService over a real engine whose context root is
// a temp dir, with chat "1" in workspace "ws" and the given hive sessions
// saved in hive.db.
func newDriveFixture(t *testing.T, hiveSessions ...session.Session) driveFixture {
	t.Helper()
	bus := newTestBus(t)
	signals := &driveSignals{bus: bus}
	cancelUpdated := events.Subscribe(t.Context(), bus, "test.drive-updated", events.Buffer(64), func(_ context.Context, e events.DriveChanged) {
		signals.addUpdate(e)
	})
	t.Cleanup(cancelUpdated)
	cancelTabbed := events.Subscribe(t.Context(), bus, "test.drive-tabbed", events.Buffer(64), func(_ context.Context, e events.SessionLayoutRequested) {
		signals.addTab(e)
	})
	t.Cleanup(cancelTabbed)

	contextRoot := t.TempDir()
	h := newHiveHarness(t, engineOptions{cfg: func(cfg *config.Config) { cfg.Context.BaseDir = contextRoot }})
	h.save(t, hiveSessions...)
	forwardDriveChanges(t.Context(), h.engine.Bus(), bus)
	go h.engine.Bus().Start(t.Context())
	terminals := &fakeTerminals{
		running: map[string]bool{siteSession.Slug: true},
		windows: map[string]string{"@3": "@3", "logs": "@4"},
		names:   map[string]string{"@3": "claude", "@4": "logs"},
	}
	svc := newDriveService(DriveDeps{
		Engine:   func() *hive.Engine { return h.engine },
		Sessions: fakeChatSessions{"1": {ID: "1", Workspace: "ws", Name: "chat", Agent: "claude"}},
		Events:   bus,
	})
	layouts := newLayoutsService(LayoutsDeps{
		Callers: svc, Terminals: terminals, Events: bus,
		CheckAppletTab: func(id string) error {
			if id != "radio" {
				return Errorf(KindInvalid, "%s has no tab", id)
			}
			return nil
		},
	})
	return driveFixture{svc: svc, signals: signals, harness: h, terminals: terminals, layouts: layouts}
}

func (f driveFixture) contextDir() string { return f.harness.cfg.ContextDir() }

func mustSpace(space drive.Space, err error) drive.Space {
	if err != nil {
		panic(err)
	}
	return space
}

var wsSpace = mustSpace(drive.WorkspaceSpace("ws"))

func TestDocumentsOfAnUnknownSessionAreNotFound(t *testing.T) {
	f := newDriveFixture(t)
	ctx := t.Context()

	_, _, err := f.svc.ListDocuments(ctx, "99", "", false)
	assert.Equal(t, KindNotFound, KindOf(err))
	_, _, err = f.svc.ReadDocument(ctx, "99", "plan.md")
	assert.Equal(t, KindNotFound, KindOf(err))
	_, err = f.svc.WriteDocument(ctx, "99", "plan.md", []byte("x"))
	assert.Equal(t, KindNotFound, KindOf(err))
	_, err = f.svc.MoveDocument(ctx, "99", "plan.md", "b.md")
	assert.Equal(t, KindNotFound, KindOf(err))
	assert.Equal(t, KindNotFound, KindOf(f.svc.DeleteDocument(ctx, "99", "plan.md")))
}

func TestOpenTabResolvesAndPublishes(t *testing.T) {
	f := newDriveFixture(t)
	ctx := t.Context()

	path, err := f.openTab(ctx, "1", "drive", "", "")
	require.NoError(t, err, "opening without a path leaves the tab's own pick")
	assert.Empty(t, path)
	_, err = f.openTab(ctx, "1", "drive", "ghost", "")
	assert.Equal(t, KindNotFound, KindOf(err), "an unknown path is not found")

	_, err = f.svc.WriteDocument(ctx, "1", "plans/auth.md", []byte("x"))
	require.NoError(t, err)
	path, err = f.openTab(ctx, "1", "drive", "plans/auth.md", "beside")
	require.NoError(t, err)
	assert.Equal(t, "plans/auth.md", path)
	_, err = f.openTab(ctx, "1", "drive", "plans/auth", "")
	assert.Equal(t, KindNotFound, KindOf(err), "a path without its extension is not a document")
	path, err = f.openTab(ctx, "1", "drive", "plans/auth.md", "")
	require.NoError(t, err)
	assert.Equal(t, "plans/auth.md", path)
	_, err = f.openTab(ctx, "1", "drive", "plans", "")
	assert.Equal(t, KindInvalid, KindOf(err), "a folder is not a document")

	require.NoError(t, f.layouts.CloseTab(ctx, "1", "drive", ""))
	_, err = f.openTab(ctx, "99", "drive", "", "")
	assert.Equal(t, KindNotFound, KindOf(err))

	space := "workspaces/ws"
	assert.Equal(t, []events.SessionLayoutRequested{
		{Session: "1", Space: space, Verb: "open", View: "drive", Placement: "behind"},
		{Session: "1", Space: space, Verb: "open", View: "drive", Path: "plans/auth.md", Placement: "beside"},
		{Session: "1", Space: space, Verb: "open", View: "drive", Path: "plans/auth.md", Placement: "behind"},
		{Session: "1", Space: space, Verb: "close", View: "drive"},
	}, f.signals.waitTabs(t, 4))
	f.signals.waitUpdates(t, 1)
}

func TestAgentsOpenAndCloseAnAppletTab(t *testing.T) {
	f := newDriveFixture(t, siteSession)
	ctx := t.Context()

	_, err := f.openTab(ctx, "1", "applet:radio", "", "beside")
	require.NoError(t, err)
	require.NoError(t, f.layouts.CloseTab(ctx, "1", "applet:radio", ""))
	_, err = f.openTab(ctx, siteSession.Path, "applet:radio", "", "")
	require.NoError(t, err, "a chat and a Code session can both hold one")

	tabs := f.signals.waitTabs(t, 3)
	assert.Equal(t, "open", tabs[0].Verb)
	assert.Equal(t, "applet:radio", tabs[0].View)
	assert.Equal(t, "beside", tabs[0].Placement)
	assert.Equal(t, "close", tabs[1].Verb)
	assert.Equal(t, "applet:radio", tabs[1].View)
	assert.Equal(t, "open", tabs[2].Verb)

	_, err = f.openTab(ctx, "1", "applet:clock", "", "")
	assert.Equal(t, KindInvalid, KindOf(err), "the applet's file decides whether it has a tab")
	_, err = f.openTab(ctx, "1", "applet:radio", "plans/a.md", "")
	assert.Equal(t, KindInvalid, KindOf(err), "a path is only for drive")
	_, err = f.openTab(ctx, "global", "applet:radio", "", "")
	assert.Equal(t, KindInvalid, KindOf(err), "the shared space has no tabs")
}

func TestOpenTabRejectsWhatAnAgentCannotAct(t *testing.T) {
	f := newDriveFixture(t, siteSession)
	ctx := t.Context()
	code := siteSession.Path

	for name, tc := range map[string]struct{ session, view, path, placement string }{
		"chat asking for diff":    {"1", "diff", "", ""},
		"unknown view":            {"1", "board", "", ""},
		"unknown placement":       {"1", "drive", "", "above"},
		"path with the diff view": {code, "diff", "plans/a.md", ""},
		"shared space":            {"global", "drive", "", ""},
		"path escaping the space": {"1", "drive", "../x.md", ""},
	} {
		_, err := f.openTab(ctx, tc.session, tc.view, tc.path, tc.placement)
		assert.Equal(t, KindInvalid, KindOf(err), name)
	}
	f.signals.requireNoUpdates(t, "a refused request publishes nothing")
	assert.Empty(t, f.signals.Tabs())
}

func TestOpenTabRejectsTerminalRequestsThatCannotWork(t *testing.T) {
	f := newDriveFixture(t, siteSession)
	ctx := t.Context()
	code := siteSession.Path

	for name, req := range map[string]OpenTabRequest{
		"terminal from a chat":         {Session: "1", View: "terminal"},
		"window with name":             {Session: code, View: "terminal", Window: "@3", Name: "x"},
		"name with the drive view":     {Session: "1", View: "drive", Name: "x"},
		"path with the terminal view":  {Session: code, View: "terminal", Path: "plans/a.md"},
		"terminal in the shared space": {Session: "global", View: "terminal"},
	} {
		_, err := f.layouts.OpenTab(ctx, req)
		assert.Equal(t, KindInvalid, KindOf(err), name)
	}

	f.terminals.running = nil
	_, err := f.layouts.OpenTab(ctx, OpenTabRequest{Session: code, View: "terminal"})
	require.Error(t, err)
	assert.Equal(t, KindInvalid, KindOf(err), "a session that is not running")
	assert.Contains(t, err.Error(), "start it in Code first")

	f.terminals.running = map[string]bool{siteSession.Slug: true}
	_, err = f.layouts.OpenTab(ctx, OpenTabRequest{Session: code, View: "terminal", Window: "ghost"})
	assert.Equal(t, KindNotFound, KindOf(err), "a window the session does not have")

	assert.Empty(t, f.terminals.created, "a refused request starts no window")
	assert.Empty(t, f.signals.Tabs())
}

func TestOpenTabStartsATerminalInTheCheckout(t *testing.T) {
	f := newDriveFixture(t, siteSession)
	ctx := t.Context()

	res, err := f.layouts.OpenTab(ctx, OpenTabRequest{
		Session: siteSession.Path + "/cmd/api", View: "terminal", Name: "tests", Placement: "beside",
	})
	require.NoError(t, err)
	assert.Equal(t, OpenTabResult{Window: "@11"}, res)
	assert.Equal(t, []createdWindow{{Slug: "fix-login", Dir: "/work/site-abc123", Name: "tests"}}, f.terminals.created,
		"the window starts in the checkout root, not the agent's subdirectory")

	res, err = f.layouts.OpenTab(ctx, OpenTabRequest{Session: siteSession.Path, View: "terminal", Window: "logs"})
	require.NoError(t, err)
	assert.Equal(t, OpenTabResult{Window: "@4"}, res, "a name resolves to the window id")
	assert.Len(t, f.terminals.created, 1, "placing a window starts none")

	assert.Equal(t, []events.SessionLayoutRequested{
		{HiveSession: "abc123", Space: "acme/site", Verb: "open", View: "terminal", Window: "@11", Placement: "beside"},
		{HiveSession: "abc123", Space: "acme/site", Verb: "open", View: "terminal", Window: "@4", Placement: "behind"},
	}, f.signals.waitTabs(t, 2))
}

func TestOpenTabUsesTheSessionsTmuxName(t *testing.T) {
	named := siteSession
	named.SetMeta(session.MetaTmuxSession, "renamed")
	f := newDriveFixture(t, named)
	f.terminals.running = map[string]bool{"renamed": true}

	_, err := f.layouts.OpenTab(t.Context(), OpenTabRequest{Session: named.Path, View: "terminal"})
	require.NoError(t, err)
	require.Len(t, f.terminals.created, 1)
	assert.Equal(t, "renamed", f.terminals.created[0].Slug)
}

func TestOpenTabTerminalIsUnavailableWithoutTmux(t *testing.T) {
	f := newDriveFixture(t, siteSession)
	f.layouts.terminals = nil
	_, err := f.layouts.OpenTab(t.Context(), OpenTabRequest{Session: siteSession.Path, View: "terminal"})
	assert.Equal(t, KindUnavailable, KindOf(err))
}

func TestCloseTabAppliesTheSameCallerRules(t *testing.T) {
	f := newDriveFixture(t, siteSession)
	ctx := t.Context()

	assert.Equal(t, KindInvalid, KindOf(f.layouts.CloseTab(ctx, "1", "diff", "")), "a chat has no Diff tab")
	assert.Equal(t, KindInvalid, KindOf(f.layouts.CloseTab(ctx, "1", "board", "")), "an unknown view")
	assert.Equal(t, KindInvalid, KindOf(f.layouts.CloseTab(ctx, "global", "drive", "")), "the shared space has no tabs")
	assert.Equal(t, KindInvalid, KindOf(f.layouts.CloseTab(ctx, siteSession.Path, "terminal", "")), "a terminal closes from its tab or shell")

	require.NoError(t, f.layouts.CloseTab(ctx, siteSession.Path, "tasks", ""))
	assert.Equal(t, []events.SessionLayoutRequested{
		{HiveSession: "abc123", Space: "acme/site", Verb: "close", View: "tasks"},
	}, f.signals.waitTabs(t, 1))
}

// A kill tmux refuses reaches the agent with tmux's own words, not as an
// internal error that hides them.
func TestCloseTabReportsAKillThatFailed(t *testing.T) {
	f := newDriveFixture(t, siteSession)
	f.terminals.closeErr = errors.New("can't find window: @4")

	err := f.layouts.CloseTab(t.Context(), siteSession.Path, "terminal", "logs")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "can't find window: @4")
	assert.Contains(t, err.Error(), `closing window "@4"`)
}

// The engine is the only publisher of a drive change. A sentinel sent through
// the engine bus after the write arrives behind everything the write caused,
// so the recorded list is complete when it lands.
func TestOneWritePublishesOneDriveChangedWithItsAuthor(t *testing.T) {
	f := newDriveFixture(t)
	ctx := t.Context()

	_, err := f.svc.WriteDocument(ctx, "1", "plan.md", []byte("x"))
	require.NoError(t, err)
	f.harness.engine.Bus().PublishDriveChanged(hiveevents.DriveChangedPayload{Space: "sentinel"})

	updates := f.signals.waitUpdates(t, 2)
	assert.Equal(t, []events.DriveChanged{
		{Space: "workspaces/ws", Paths: []string{"plan.md"}, Session: "1"},
		{Space: "sentinel"},
	}, updates)
}

var siteSession = session.Session{
	ID: "abc123", Name: "fix-login", Slug: "fix-login", Path: "/work/site-abc123",
	Remote: "git@github.com:acme/site.git", State: session.StateActive, CloneStrategy: session.CloneStrategyFull,
}

// An agent in a hive session has no id to pass, only where it runs. The
// checkout that holds that directory names the repository, and the document
// is filed in that repository's space, outside the checkout.
func TestDocumentFromAHiveSessionIsFiledUnderItsRepository(t *testing.T) {
	f := newDriveFixture(t, siteSession)
	ctx := t.Context()

	_, err := f.svc.WriteDocument(ctx, "/work/site-abc123/cmd/api", "plan.md", []byte("x"))
	require.NoError(t, err)
	require.FileExists(t, filepath.Join(f.contextDir(), "acme", "site", "plan.md"))

	updates := f.signals.waitUpdates(t, 1)
	assert.Equal(t, events.DriveChanged{Space: "acme/site", Paths: []string{"plan.md"}, HiveSession: "abc123"}, updates[0])

	_, body, err := f.svc.ReadDocument(ctx, "/work/site-abc123", "plan.md")
	require.NoError(t, err)
	assert.Equal(t, "x", string(body))
}

// Two sessions of one repository share its documents, the way two chats share
// a workspace's.
func TestHiveSessionsOfOneRepositoryShareItsDocuments(t *testing.T) {
	other := session.Session{
		ID: "def456", Name: "add-search", Slug: "add-search", Path: "/work/site-def456",
		Remote: "https://github.com/acme/site", State: session.StateActive, CloneStrategy: session.CloneStrategyFull,
	}
	f := newDriveFixture(t, siteSession, other)
	ctx := t.Context()

	_, err := f.svc.WriteDocument(ctx, siteSession.Path, "plan.md", []byte("x"))
	require.NoError(t, err)

	_, body, err := f.svc.ReadDocument(ctx, other.Path, "plan.md")
	require.NoError(t, err)
	assert.Equal(t, "x", string(body))
}

func TestCallerResolvesAChatWithoutTheHiveEngine(t *testing.T) {
	svc := newDriveService(DriveDeps{
		Engine:   func() *hive.Engine { return nil },
		Sessions: fakeChatSessions{"1": {ID: "1", Workspace: "ws", Name: "chat", Agent: "claude"}},
		Events:   newTestBus(t),
	})
	ctx := t.Context()

	chat, err := svc.resolve(ctx, "1")
	require.NoError(t, err)
	assert.Equal(t, "1", chat.author.Session)

	global, err := svc.resolve(ctx, "global")
	require.NoError(t, err)
	assert.Equal(t, drive.Shared, global.space)

	_, err = svc.resolve(ctx, "abc123")
	assert.Equal(t, KindUnavailable, KindOf(err), "a hive session needs the engine")
}

func TestSessionArgumentThatResolvesToNothing(t *testing.T) {
	noRemote := session.Session{
		ID: "zzz999", Name: "scratch", Slug: "scratch", Path: "/work/scratch",
		State: session.StateActive, CloneStrategy: session.CloneStrategyFull,
	}
	f := newDriveFixture(t, siteSession, noRemote)
	ctx := t.Context()

	_, err := f.svc.WriteDocument(ctx, "/somewhere/else", "plan.md", []byte("x"))
	assert.Equal(t, KindNotFound, KindOf(err), "a directory in no checkout is not filed as shared")
	require.ErrorContains(t, err, "global", "the error names the way out")

	_, err = f.svc.WriteDocument(ctx, "site-abc123", "plan.md", []byte("x"))
	assert.Equal(t, KindNotFound, KindOf(err), "not a chat id or an absolute path, so a session id that names no session")

	_, err = f.svc.WriteDocument(ctx, "abc123", "plan.md", []byte("x"))
	require.NoError(t, err, "a Code session's HIVE_AGENT_SESSION is its session id")

	_, err = f.svc.WriteDocument(ctx, "/work/scratch", "plan.md", []byte("x"))
	assert.Equal(t, KindInvalid, KindOf(err), "a session whose remote names no repository")

	_, err = f.svc.WriteDocument(ctx, " 1 ", "plan.md", []byte("x"))
	require.NoError(t, err, "a chat id still resolves, as HIVE_AGENT_SESSION carries it")
}

func TestAgentsOutsideEverySessionShareTheSharedSpace(t *testing.T) {
	f := newDriveFixture(t, siteSession)
	ctx := t.Context()

	_, err := f.svc.WriteDocument(ctx, "global", "notes.md", []byte("x"))
	require.NoError(t, err)
	_, err = f.svc.WriteDocument(ctx, " global ", "notes.md", []byte("y"))
	require.NoError(t, err, "a second global agent writes to the same document")

	root, err := f.svc.Tree(ctx, drive.SharedKey)
	require.NoError(t, err)
	require.Len(t, root.Children, 1)
	assert.Equal(t, "notes.md", root.Children[0].Path)
	updates := f.signals.waitUpdates(t, 2)
	notes := events.DriveChanged{Space: drive.SharedKey, Paths: []string{"notes.md"}}
	assert.Equal(t, []events.DriveChanged{notes, notes}, updates, "shared writes still wake the readers, with no author")

	_, err = f.openTab(ctx, "global", "drive", "notes.md", "")
	assert.Equal(t, KindInvalid, KindOf(err), "a shared document has no Drive tab")
}

func TestOpenTabNamesTheHiveSession(t *testing.T) {
	f := newDriveFixture(t, siteSession)

	path, err := f.openTab(t.Context(), siteSession.Path, "diff", "", "beside")
	require.NoError(t, err)
	assert.Empty(t, path)

	assert.Equal(t, []events.SessionLayoutRequested{
		{HiveSession: "abc123", Space: "acme/site", Verb: "open", View: "diff", Placement: "beside"},
	}, f.signals.waitTabs(t, 1))
}

func TestDriveIsUnavailableBeforeTheEngineStarts(t *testing.T) {
	t.Parallel()
	svc := newDriveService(DriveDeps{
		Engine:   func() *hive.Engine { return nil },
		Sessions: fakeChatSessions{"1": {ID: "1", Workspace: "ws", Name: "chat", Agent: "claude"}},
		Events:   newTestBus(t),
	})
	ctx := t.Context()

	_, _, err := svc.ReadDocument(ctx, "1", "plan.md")
	assert.Equal(t, KindUnavailable, KindOf(err))
	_, err = svc.WriteDocument(ctx, "/work/site-abc123", "plan.md", []byte("x"))
	assert.Equal(t, KindUnavailable, KindOf(err))
	_, err = svc.Spaces(ctx)
	assert.Equal(t, KindUnavailable, KindOf(err))
	_, err = svc.Tree(ctx, "workspaces/ws")
	assert.Equal(t, KindUnavailable, KindOf(err))
	_, err = svc.Search(ctx, "plan", 0)
	assert.Equal(t, KindUnavailable, KindOf(err))
	_, err = svc.Recent(ctx, 0)
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
	f.writeDoc(t, "workspaces/ws", "shots/Login.PNG", "png-bytes")
	f.writeDoc(t, "workspaces/ws", "huge.png", strings.Repeat("x", maxImageBytes+1))
	f.writeDoc(t, "workspaces/ws", "canvases/plan.canvas.json", `{"blocks":[]}`)
	f.writeDoc(t, "workspaces/ws", "src/main.go", "package main\n")
	f.writeDoc(t, "workspaces/ws", "notes", "plain notes\n")
	f.writeDoc(t, "workspaces/ws", "latin1.txt", "caf\xe9")
	f.writeDoc(t, "workspaces/ws", "big.log", strings.Repeat("x", maxTextBytes+1))

	md, err := f.svc.Document(ctx, "workspaces/ws", "plans/p.md")
	require.NoError(t, err)
	assert.Equal(t, wsSpace, md.Space)
	assert.Equal(t, drive.KindMarkdown, md.Entry.Kind)
	assert.Equal(t, "P", md.Entry.Title)
	assert.Equal(t, "---\ntitle: P\n---\n# Hi <b>raw</b>\n", md.Markdown, "markdown leaves raw; the renderer sanitizes")
	assert.Empty(t, md.HTML)

	page, err := f.svc.Document(ctx, "workspaces/ws", "page.html")
	require.NoError(t, err)
	assert.Equal(t, drive.KindHtml, page.Entry.Kind)
	assert.True(t, strings.HasPrefix(page.HTML, `<html><head><base href="about:srcdoc"><meta http-equiv="Content-Security-Policy"`), page.HTML)
	assert.Contains(t, page.HTML, "<p>x</p>")
	assert.Empty(t, page.Markdown)

	leftover, err := f.svc.Document(ctx, "workspaces/ws", "canvases/plan.canvas.json")
	require.NoError(t, err)
	assert.Equal(t, drive.KindFile, leftover.Entry.Kind, "a leftover canvas file is a plain file")
	assert.Empty(t, leftover.Markdown)
	assert.JSONEq(t, `{"blocks":[]}`, leftover.Text)

	code, err := f.svc.Document(ctx, "workspaces/ws", "src/main.go")
	require.NoError(t, err)
	assert.Equal(t, drive.KindFile, code.Entry.Kind)
	assert.Equal(t, "package main\n", code.Text)

	notes, err := f.svc.Document(ctx, "workspaces/ws", "notes")
	require.NoError(t, err)
	assert.Equal(t, "plain notes\n", notes.Text, "a file with no extension is text when its bytes are")

	latin1, err := f.svc.Document(ctx, "workspaces/ws", "latin1.txt")
	require.NoError(t, err)
	assert.Empty(t, latin1.Text, "a file that is not UTF-8 is not shown as text")

	big, err := f.svc.Document(ctx, "workspaces/ws", "big.log")
	require.NoError(t, err)
	assert.Empty(t, big.Text, "a text file over the cap stays a plain file")

	file, err := f.svc.Document(ctx, "workspaces/ws", "data.bin")
	require.NoError(t, err)
	assert.Equal(t, drive.KindFile, file.Entry.Kind)
	assert.Empty(t, file.Markdown)
	assert.Empty(t, file.HTML)
	assert.Empty(t, file.Image)
	assert.Empty(t, file.Text, "a file with a NUL byte is binary")

	img, err := f.svc.Document(ctx, "workspaces/ws", "shots/Login.PNG")
	require.NoError(t, err)
	assert.Equal(t, drive.KindFile, img.Entry.Kind)
	assert.Equal(t, "data:image/png;base64,"+base64.StdEncoding.EncodeToString([]byte("png-bytes")), img.Image)
	assert.Empty(t, img.Text, "an image is never also text")

	huge, err := f.svc.Document(ctx, "workspaces/ws", "huge.png")
	require.NoError(t, err)
	assert.Empty(t, huge.Image, "an image over the cap stays a plain file")

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

func TestDriveMarkdownRefusesHTMLAndPlainFiles(t *testing.T) {
	f := newDriveFixture(t)
	ctx := t.Context()
	f.writeDoc(t, "workspaces/ws", "page.html", "<p>x</p>")
	f.writeDoc(t, "workspaces/ws", "notes.txt", "x")

	for _, path := range []string{"page.html", "notes.txt"} {
		_, err := f.svc.Markdown(ctx, "workspaces/ws", path)
		assert.Equal(t, KindInvalid, KindOf(err), path)
		assert.Contains(t, errorMessage(t, err), "export as markdown", path)
		err = f.svc.Export(ctx, "workspaces/ws", path, filepath.Join(t.TempDir(), "out.md"))
		assert.Equal(t, KindInvalid, KindOf(err), path)
	}
}

// Spaces, Tree, Search and Recent pass through to the engine, whose tests own their
// behaviour; the facade adds the space-key check and the limit check.
func TestDriveTreeSearchAndRecentGuardTheirArguments(t *testing.T) {
	f := newDriveFixture(t)
	ctx := t.Context()
	f.writeDoc(t, "acme/site", "plans/release-plan.md", "---\ntitle: Release\n---\n")

	_, err := f.svc.Tree(ctx, "../acme")
	assert.Equal(t, KindInvalid, KindOf(err))

	hits, err := f.svc.Search(ctx, "release", 1)
	require.NoError(t, err)
	require.Len(t, hits, 1)
	assert.Equal(t, "plans/release-plan.md", hits[0].Entry.Path)
	_, err = f.svc.Search(ctx, "plan", -1)
	assert.Equal(t, KindInvalid, KindOf(err))

	recent, err := f.svc.Recent(ctx, 0)
	require.NoError(t, err)
	require.Len(t, recent, 1)
	assert.Equal(t, "plans/release-plan.md", recent[0].Entry.Path)
	_, err = f.svc.Recent(ctx, -1)
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
		{"canvases/plan.canvas.json", "roadmap", "canvases/roadmap.json"},
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

	_, err := f.svc.Rename(ctx, "workspaces/ws", "d.md", "d.html")
	assert.Contains(t, errorMessage(t, err), "keep the extension")
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
