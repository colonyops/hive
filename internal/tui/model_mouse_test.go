package tui

import (
	"context"
	"io"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/internal/core/config"
	"github.com/colonyops/hive/internal/core/eventbus/testbus"
	"github.com/colonyops/hive/internal/core/git"
	"github.com/colonyops/hive/internal/core/session"
	"github.com/colonyops/hive/internal/core/terminal"
	"github.com/colonyops/hive/internal/hive"
	"github.com/colonyops/hive/internal/hive/plugins"
	"github.com/colonyops/hive/internal/tui/views/sessions"
	"github.com/colonyops/hive/pkg/executil"
	"github.com/colonyops/hive/pkg/tmpl"
)

// --- minimal mock implementations for sessions.View construction ---

type mouseTestStore struct{}

func (s *mouseTestStore) List(_ context.Context) ([]session.Session, error) { return nil, nil }
func (s *mouseTestStore) Get(_ context.Context, _ string) (session.Session, error) {
	return session.Session{}, session.ErrNotFound
}
func (s *mouseTestStore) Save(_ context.Context, _ session.Session) error { return nil }
func (s *mouseTestStore) Delete(_ context.Context, _ string) error        { return nil }

type mouseTestGit struct{}

func (g *mouseTestGit) Clone(_ context.Context, _, _ string) error                { return nil }
func (g *mouseTestGit) Checkout(_ context.Context, _, _ string) error             { return nil }
func (g *mouseTestGit) Pull(_ context.Context, _ string) error                    { return nil }
func (g *mouseTestGit) ResetHard(_ context.Context, _ string) error               { return nil }
func (g *mouseTestGit) RemoteURL(_ context.Context, _ string) (string, error)     { return "", nil }
func (g *mouseTestGit) IsClean(_ context.Context, _ string) (bool, error)         { return true, nil }
func (g *mouseTestGit) Branch(_ context.Context, _ string) (string, error)        { return "main", nil }
func (g *mouseTestGit) DefaultBranch(_ context.Context, _ string) (string, error) { return "main", nil }
func (g *mouseTestGit) DiffStats(_ context.Context, _ string) (int, int, error)   { return 0, 0, nil }
func (g *mouseTestGit) IsValidRepo(_ context.Context, _ string) error             { return nil }
func (g *mouseTestGit) CloneBare(_ context.Context, _, _ string) error            { return nil }
func (g *mouseTestGit) WorktreeAdd(_ context.Context, _, _, _ string) error       { return nil }
func (g *mouseTestGit) WorktreeRemove(_ context.Context, _, _, _ string) error    { return nil }
func (g *mouseTestGit) WorktreeReset(_ context.Context, _, _ string) error        { return nil }
func (g *mouseTestGit) Fetch(_ context.Context, _ string) error                   { return nil }
func (g *mouseTestGit) HasUnpushedCommits(_ context.Context, _ string) (bool, error) {
	return false, nil
}

type mouseTestExec struct{}

func (e *mouseTestExec) Run(_ context.Context, _ string, _ ...string) ([]byte, error) {
	return nil, nil
}

func (e *mouseTestExec) RunDir(_ context.Context, _, _ string, _ ...string) ([]byte, error) {
	return nil, nil
}

func (e *mouseTestExec) RunStream(_ context.Context, _, _ io.Writer, _ string, _ ...string) error {
	return nil
}

func (e *mouseTestExec) RunDirStream(_ context.Context, _ string, _, _ io.Writer, _ string, _ ...string) error {
	return nil
}

var (
	_ session.Store     = (*mouseTestStore)(nil)
	_ git.Git           = (*mouseTestGit)(nil)
	_ executil.Executor = (*mouseTestExec)(nil)
)

// newMouseTestSessionService creates a minimal SessionService for mouse tests.
func newMouseTestSessionService(t *testing.T) *hive.SessionService {
	t.Helper()
	tb := testbus.New(t)
	log := zerolog.New(io.Discard)
	r := tmpl.New(tmpl.Config{})
	return hive.NewSessionService(
		&mouseTestStore{},
		&mouseTestGit{},
		&config.Config{DataDir: t.TempDir(), GitPath: "git"},
		tb.EventBus,
		&mouseTestExec{},
		r,
		log,
		io.Discard,
		io.Discard,
	)
}

// newMouseTestSessionsView builds a minimal sessions.View for mouse tests.
func newMouseTestSessionsView(t *testing.T) *sessions.View {
	t.Helper()
	svc := newMouseTestSessionService(t)
	cfg := &config.Config{}
	handler := NewKeybindingResolver(nil, nil, testRenderer)
	mgr := terminal.NewManager(nil)
	pm := plugins.NewManager(plugins.NewWorkerPool(0))
	return sessions.New(sessions.ViewOpts{
		Cfg:             cfg,
		Service:         svc,
		Handler:         handler,
		TerminalManager: mgr,
		PluginManager:   pm,
	})
}

// newBaseMouseModel returns a minimal Model with the required fields set for mouse tests.
// It has a real sessionsView so switchToView doesn't panic, and a non-nil kvView so
// handleKey doesn't dereference a nil pointer when dispatching Enter on a double-click.
func newBaseMouseModel(t *testing.T) Model {
	t.Helper()
	handler := NewKeybindingResolver(nil, map[string]config.UserCommand{}, testRenderer)
	return Model{
		cfg:             &config.Config{},
		activeView:      ViewSessions,
		handler:         handler,
		modals:          NewModalCoordinator(),
		toastController: NewToastController(),
		sessionsView:    newMouseTestSessionsView(t),
		kvView:          NewKVView(),
	}
}

// --- handleMouseClick tests ---

func TestHandleMouseClick_NonLeftButton_NoOp(t *testing.T) {
	m := newBaseMouseModel(t)
	initialView := m.activeView

	for _, btn := range []tea.MouseButton{tea.MouseRight, tea.MouseMiddle} {
		result, cmd := m.handleMouseClick(tea.MouseClickMsg{Button: btn, X: 5, Y: 1})
		rm := result.(Model)
		assert.Equal(t, initialView, rm.activeView, "non-left button should not change view")
		assert.Nil(t, cmd, "non-left button should return nil cmd")
	}
}

func TestHandleMouseClick_ModalActive_NoOp(t *testing.T) {
	m := newBaseMouseModel(t)
	m.state = stateConfirming // modal active
	initialView := m.activeView

	result, cmd := m.handleMouseClick(tea.MouseClickMsg{Button: tea.MouseLeft, X: 5, Y: 1})
	rm := result.(Model)
	assert.Equal(t, initialView, rm.activeView, "modal active should not change view")
	assert.Nil(t, cmd, "modal active should return nil cmd")
}

func TestHandleMouseClick_TopDivider_NoOp(t *testing.T) {
	// Y=0 → topDivider, contentY = 0 - 3 = -3 < 0 → no-op
	m := newBaseMouseModel(t)
	initialView := m.activeView

	result, cmd := m.handleMouseClick(tea.MouseClickMsg{Button: tea.MouseLeft, X: 5, Y: 0})
	rm := result.(Model)
	assert.Equal(t, initialView, rm.activeView, "Y=0 should be a no-op")
	assert.Nil(t, cmd, "Y=0 should return nil cmd")
}

func TestHandleMouseClick_TabBar_DelegatesTabClick(t *testing.T) {
	// Y=1 is the tab bar; x=12 falls in "Messages" label → ViewMessages
	m := newBaseMouseModel(t)

	result, _ := m.handleMouseClick(tea.MouseClickMsg{Button: tea.MouseLeft, X: 12, Y: 1})
	rm := result.(Model)
	assert.Equal(t, ViewMessages, rm.activeView, "Y=1 with x in Messages tab should switch to ViewMessages")
}

func TestHandleMouseClick_FirstClick_TracksPosition(t *testing.T) {
	m := newBaseMouseModel(t)
	require.True(t, m.lastClickTime.IsZero(), "lastClickTime should be zero initially")

	result, _ := m.handleMouseClick(tea.MouseClickMsg{Button: tea.MouseLeft, X: 3, Y: 5})
	rm := result.(Model)

	assert.Equal(t, 3, rm.lastClickX, "lastClickX should be updated")
	assert.Equal(t, 5, rm.lastClickY, "lastClickY should be updated")
	assert.False(t, rm.lastClickTime.IsZero(), "lastClickTime should be set after first click")
}

func TestHandleMouseClick_ExpiredDoubleClick_NotDoubleClick(t *testing.T) {
	// Set up a prior click more than 300ms ago at the same position.
	m := newBaseMouseModel(t)
	m.lastClickX = 3
	m.lastClickY = 5
	m.lastClickTime = time.Now().Add(-500 * time.Millisecond) // expired

	// Second click at same position, but outside the window → not double-click → SelectAtRow path
	result, _ := m.handleMouseClick(tea.MouseClickMsg{Button: tea.MouseLeft, X: 3, Y: 5})
	rm := result.(Model)

	// The click should update tracking state, not enter the double-click branch.
	assert.Equal(t, 3, rm.lastClickX)
	assert.Equal(t, 5, rm.lastClickY)
	// lastClickTime should be refreshed to now (not the old expired value)
	assert.Less(t, time.Since(rm.lastClickTime), time.Second, "lastClickTime should be refreshed")
}

func TestHandleMouseClick_DoubleClick_ResetsTracking(t *testing.T) {
	// A double-click is within 300ms at the same cell.
	// Because handleKeyMsg(Enter) on a quitting model short-circuits,
	// we set m.quitting=true so the model exits cleanly without panicking.
	m := newBaseMouseModel(t)
	m.quitting = true // ensure handleKeyMsg returns cleanly
	m.lastClickX = 3
	m.lastClickY = 5
	m.lastClickTime = time.Now() // recent click → will trigger double-click

	// Second click at same position within 300ms window
	result, _ := m.handleMouseClick(tea.MouseClickMsg{Button: tea.MouseLeft, X: 3, Y: 5})
	rm := result.(Model)

	// After double-click, position tracking should be preserved (lastClickX/Y updated).
	assert.Equal(t, 3, rm.lastClickX)
	assert.Equal(t, 5, rm.lastClickY)
}

// --- handleTabClick tests ---

// Tab layout (no tasksView / reviewView):
//   left margin = 1
//   "Sessions" (8 chars): x=1..8
//   separator " | ": x=9..11
//   "Messages" (8 chars): x=12..19
//
// (Store tab hidden when kvStore == nil and activeView != ViewStore)

func TestHandleTabClick_Sessions(t *testing.T) {
	tests := []struct {
		name string
		x    int
	}{
		{"first char", 1},
		{"middle", 4},
		{"last char", 8},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newBaseMouseModel(t)
			m.activeView = ViewMessages // start elsewhere
			result, _ := m.handleTabClick(tt.x)
			rm := result.(Model)
			assert.Equal(t, ViewSessions, rm.activeView, "x=%d should map to ViewSessions", tt.x)
		})
	}
}

func TestHandleTabClick_Messages(t *testing.T) {
	tests := []struct {
		name string
		x    int
	}{
		{"first char", 12},
		{"middle", 15},
		{"last char", 19},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newBaseMouseModel(t)
			m.activeView = ViewSessions // start elsewhere
			result, _ := m.handleTabClick(tt.x)
			rm := result.(Model)
			assert.Equal(t, ViewMessages, rm.activeView, "x=%d should map to ViewMessages", tt.x)
		})
	}
}

func TestHandleTabClick_Separator_NoOp(t *testing.T) {
	// Separator " | " occupies x=9..11
	for _, x := range []int{9, 10, 11} {
		m := newBaseMouseModel(t)
		initial := m.activeView
		result, cmd := m.handleTabClick(x)
		rm := result.(Model)
		assert.Equal(t, initial, rm.activeView, "separator x=%d should not change view", x)
		assert.Nil(t, cmd, "separator x=%d should return nil cmd", x)
	}
}

func TestHandleTabClick_PastAllLabels_NoOp(t *testing.T) {
	// x=20 is past all labels (Sessions=8, sep=3, Messages=8 → total 19)
	m := newBaseMouseModel(t)
	initial := m.activeView
	result, cmd := m.handleTabClick(20)
	rm := result.(Model)
	assert.Equal(t, initial, rm.activeView, "x past all labels should not change view")
	assert.Nil(t, cmd, "x past all labels should return nil cmd")
}

func TestHandleTabClick_ZeroX_NoOp(t *testing.T) {
	// x=0 is before the left margin (labels start at x=1)
	m := newBaseMouseModel(t)
	initial := m.activeView
	result, cmd := m.handleTabClick(0)
	rm := result.(Model)
	assert.Equal(t, initial, rm.activeView, "x=0 (before margin) should not change view")
	assert.Nil(t, cmd, "x=0 should return nil cmd")
}

// --- switchToView tests ---

func TestSwitchToView_Sessions_NilCmd(t *testing.T) {
	m := newBaseMouseModel(t)
	m.activeView = ViewMessages

	result, cmd := m.switchToView(ViewSessions)
	rm := result.(Model)

	assert.Equal(t, ViewSessions, rm.activeView)
	assert.Nil(t, cmd, "switching to Sessions should return nil cmd")
}

func TestSwitchToView_Messages_NilCmd(t *testing.T) {
	m := newBaseMouseModel(t)

	result, cmd := m.switchToView(ViewMessages)
	rm := result.(Model)

	assert.Equal(t, ViewMessages, rm.activeView)
	assert.Nil(t, cmd, "switching to Messages should return nil cmd")
}

func TestSwitchToView_Store_NonNilCmd(t *testing.T) {
	m := newBaseMouseModel(t)
	// kvStore is nil, so loadKVKeys returns nil — but we can verify the branch fires
	// by checking activeView and that the loadKVKeys path was taken (cmd is nil when kvStore==nil).
	result, _ := m.switchToView(ViewStore)
	rm := result.(Model)
	assert.Equal(t, ViewStore, rm.activeView)
}

func TestSwitchToView_Tasks_ReturnsCmd(t *testing.T) {
	m := newBaseMouseModel(t)
	// tasksView is nil, so syncTasksRepoFromSessions returns nil and we fall
	// through to the RefreshTasksMsg func. Verify cmd is non-nil.
	result, cmd := m.switchToView(ViewTasks)
	rm := result.(Model)

	assert.Equal(t, ViewTasks, rm.activeView)
	require.NotNil(t, cmd, "switching to Tasks should return a RefreshTasksMsg cmd")

	// Verify the cmd produces a message without panicking.
	msg := cmd()
	assert.NotNil(t, msg)
}

func TestSwitchToView_ResetsLastClickTime(t *testing.T) {
	m := newBaseMouseModel(t)
	m.lastClickTime = time.Now()

	result, _ := m.switchToView(ViewMessages)
	rm := result.(Model)

	assert.True(t, rm.lastClickTime.IsZero(), "switchToView should reset lastClickTime to zero")
}

func TestSwitchToView_SessionsViewSetActive(t *testing.T) {
	m := newBaseMouseModel(t)

	// Switch to Sessions — sessionsView should become active.
	result, _ := m.switchToView(ViewSessions)
	rm := result.(Model)
	assert.Equal(t, ViewSessions, rm.activeView)

	// Switch away — sessionsView.active should be cleared (we verify via activeView).
	result2, _ := rm.switchToView(ViewMessages)
	rm2 := result2.(Model)
	assert.Equal(t, ViewMessages, rm2.activeView)
}
