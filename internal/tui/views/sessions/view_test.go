package sessions

import (
	"testing"

	"charm.land/bubbles/v2/list"
	"github.com/colonyops/hive/internal/core/config"
	"github.com/colonyops/hive/internal/core/session"
	"github.com/colonyops/hive/internal/core/terminal"
	"github.com/colonyops/hive/internal/hive"
	"github.com/colonyops/hive/pkg/kv"
	"github.com/stretchr/testify/assert"
)

// newTestView creates a minimal View with a list for navigation tests.
func newTestView(items []list.Item, startIdx int) *View {
	delegate := NewTreeDelegate()
	l := list.New(items, delegate, 80, 24)
	l.Select(startIdx)
	return &View{list: l}
}

// --- navigateSkippingPlaceholders ---

func TestNavigateSkippingPlaceholders_DownPastRecycled(t *testing.T) {
	sess := session.Session{ID: "s1", Name: "session"}
	items := []list.Item{
		TreeItem{Session: sess},
		TreeItem{IsRecycledPlaceholder: true, RepoPrefix: "repo"},
		TreeItem{Session: session.Session{ID: "s2", Name: "other"}},
	}
	v := newTestView(items, 0)
	v.navigateSkippingPlaceholders(1)
	assert.Equal(t, 2, v.list.Index(), "should skip recycled placeholder and land on s2")
}

func TestNavigateSkippingPlaceholders_UpPastRecycled(t *testing.T) {
	sess := session.Session{ID: "s1", Name: "session"}
	items := []list.Item{
		TreeItem{Session: sess},
		TreeItem{IsRecycledPlaceholder: true, RepoPrefix: "repo"},
		TreeItem{Session: session.Session{ID: "s2", Name: "other"}},
	}
	v := newTestView(items, 2)
	v.navigateSkippingPlaceholders(-1)
	assert.Equal(t, 0, v.list.Index(), "should skip recycled placeholder and land on s1")
}

func TestNavigateSkippingPlaceholders_AtEndGoingDown(t *testing.T) {
	items := []list.Item{
		TreeItem{Session: session.Session{ID: "s1"}},
		TreeItem{Session: session.Session{ID: "s2"}},
	}
	v := newTestView(items, 1)
	v.navigateSkippingPlaceholders(1)
	assert.Equal(t, 1, v.list.Index(), "at end going down should stay at current position")
}

func TestNavigateSkippingPlaceholders_AtStartGoingUp(t *testing.T) {
	items := []list.Item{
		TreeItem{Session: session.Session{ID: "s1"}},
		TreeItem{Session: session.Session{ID: "s2"}},
	}
	v := newTestView(items, 0)
	v.navigateSkippingPlaceholders(-1)
	assert.Equal(t, 0, v.list.Index(), "at start going up should stay at current position")
}

func TestNavigateSkippingPlaceholders_AllPlaceholders(t *testing.T) {
	items := []list.Item{
		TreeItem{Session: session.Session{ID: "s1"}},
		TreeItem{IsRecycledPlaceholder: true},
		TreeItem{IsRecycledPlaceholder: true},
	}
	v := newTestView(items, 0)
	v.navigateSkippingPlaceholders(1)
	assert.Equal(t, 0, v.list.Index(), "all remaining items are placeholders — stay at current")
}

// --- expandWindowItems ---

func TestExpandWindowItems_NilTerminalStatuses(t *testing.T) {
	items := []list.Item{
		TreeItem{Session: session.Session{ID: "s1"}},
	}
	v := &View{terminalStatuses: nil}
	got := v.expandWindowItems(items)
	assert.Equal(t, items, got, "nil terminalStatuses returns items unchanged")
}

func TestExpandWindowItems_ZeroWindows(t *testing.T) {
	ts := kv.New[string, TerminalStatus]()
	ts.Set("s1", TerminalStatus{Status: terminal.StatusActive})
	v := &View{terminalStatuses: ts}

	items := []list.Item{TreeItem{Session: session.Session{ID: "s1"}}}
	got := v.expandWindowItems(items)
	assert.Len(t, got, 1, "session with 0 windows should not be expanded")
}

func TestExpandWindowItems_OneWindow(t *testing.T) {
	ts := kv.New[string, TerminalStatus]()
	ts.Set("s1", TerminalStatus{
		Windows: []WindowStatus{{WindowIndex: "0", WindowName: "main"}},
	})
	v := &View{terminalStatuses: ts}

	items := []list.Item{TreeItem{Session: session.Session{ID: "s1"}}}
	got := v.expandWindowItems(items)
	assert.Len(t, got, 1, "session with 1 window should not be expanded (threshold is >1)")
}

func TestExpandWindowItems_MultipleWindows(t *testing.T) {
	ts := kv.New[string, TerminalStatus]()
	ts.Set("s1", TerminalStatus{
		Windows: []WindowStatus{
			{WindowIndex: "0", WindowName: "claude"},
			{WindowIndex: "1", WindowName: "aider"},
		},
	})
	v := &View{terminalStatuses: ts}

	sess := session.Session{ID: "s1", Name: "my-session"}
	items := []list.Item{TreeItem{Session: sess, RepoPrefix: "repo"}}
	got := v.expandWindowItems(items)

	assert.Len(t, got, 3, "session with 2 windows should expand to 3 items")
	sessionItem := got[0].(TreeItem)
	assert.Equal(t, "s1", sessionItem.Session.ID)
	w0 := got[1].(TreeItem)
	assert.True(t, w0.IsWindowItem)
	assert.Equal(t, "claude", w0.WindowName)
	w1 := got[2].(TreeItem)
	assert.True(t, w1.IsWindowItem)
	assert.True(t, w1.IsLastWindow, "last window should be marked")
}

func TestExpandWindowItems_NonSessionPassthrough(t *testing.T) {
	ts := kv.New[string, TerminalStatus]()
	v := &View{terminalStatuses: ts}

	items := []list.Item{
		TreeItem{IsHeader: true, RepoName: "repo"},
		TreeItem{IsRecycledPlaceholder: true},
	}
	got := v.expandWindowItems(items)
	assert.Len(t, got, 2, "non-session items should pass through unchanged")
}

// --- applyFilter ---

func newFilterTestView(sessions []session.Session, statusFilter terminal.Status, statuses *kv.Store[string, TerminalStatus]) *View {
	delegate := NewTreeDelegate()
	l := list.New([]list.Item{}, delegate, 80, 24)
	columnWidths := &ColumnWidths{}
	ts := statuses
	if ts == nil {
		ts = kv.New[string, TerminalStatus]()
	}
	gitStatuses := kv.New[string, GitStatus]()
	// new(hive.SessionService) gives a zero-valued service whose Git() returns nil.
	// applyFilter returns a tea.Cmd that captures the nil git client but never executes
	// it in tests — so no nil-dereference occurs during the test.
	return &View{
		list:             l,
		allSessions:      sessions,
		statusFilter:     statusFilter,
		groupBy:          config.GroupByRepo,
		terminalStatuses: ts,
		gitStatuses:      gitStatuses,
		columnWidths:     columnWidths,
		service:          new(hive.SessionService),
		gitWorkers:       1,
		cfg:              &config.Config{TUI: config.TUIConfig{GroupBy: config.GroupByRepo}},
	}
}

// newSess creates a session with an empty path to avoid triggering git fetch in applyFilter.
func newSess(id, name string) session.Session {
	return session.Session{ID: id, Name: name} // Path intentionally empty
}

func TestApplyFilter_NoStatusFilter(t *testing.T) {
	sessions := []session.Session{
		newSess("s1", "alpha"),
		newSess("s2", "beta"),
	}
	v := newFilterTestView(sessions, "", nil)
	v.applyFilter()

	var sessionCount int
	for _, item := range v.list.Items() {
		if ti, ok := item.(TreeItem); ok && ti.IsSession() {
			sessionCount++
		}
	}
	assert.Equal(t, 2, sessionCount, "all sessions shown when no filter")
}

func TestApplyFilter_StatusFilterMatches(t *testing.T) {
	ts := kv.New[string, TerminalStatus]()
	ts.Set("s1", TerminalStatus{Status: terminal.StatusActive})
	ts.Set("s2", TerminalStatus{Status: terminal.StatusReady})

	sessions := []session.Session{
		newSess("s1", "active-session"),
		newSess("s2", "ready-session"),
	}
	v := newFilterTestView(sessions, terminal.StatusActive, ts)
	v.applyFilter()

	var sessionIDs []string
	for _, item := range v.list.Items() {
		if ti, ok := item.(TreeItem); ok && ti.IsSession() {
			sessionIDs = append(sessionIDs, ti.Session.ID)
		}
	}
	assert.Equal(t, []string{"s1"}, sessionIDs, "only active session shown")
}

func TestApplyFilter_StatusFilterNoMatches(t *testing.T) {
	ts := kv.New[string, TerminalStatus]()
	ts.Set("s1", TerminalStatus{Status: terminal.StatusReady})

	sessions := []session.Session{newSess("s1", "ready-session")}
	v := newFilterTestView(sessions, terminal.StatusActive, ts)
	v.applyFilter()

	var sessionCount int
	for _, item := range v.list.Items() {
		if ti, ok := item.(TreeItem); ok && ti.IsSession() {
			sessionCount++
		}
	}
	assert.Equal(t, 0, sessionCount, "no sessions shown when none match filter")
}

func TestApplyFilter_FilterThenClear(t *testing.T) {
	ts := kv.New[string, TerminalStatus]()
	ts.Set("s1", TerminalStatus{Status: terminal.StatusActive})
	ts.Set("s2", TerminalStatus{Status: terminal.StatusReady})

	sessions := []session.Session{
		newSess("s1", "active"),
		newSess("s2", "ready"),
	}
	v := newFilterTestView(sessions, terminal.StatusActive, ts)
	v.applyFilter()

	// Clear filter
	v.statusFilter = ""
	v.applyFilter()

	var sessionCount int
	for _, item := range v.list.Items() {
		if ti, ok := item.(TreeItem); ok && ti.IsSession() {
			sessionCount++
		}
	}
	assert.Equal(t, 2, sessionCount, "all sessions restored after filter cleared")
}

func TestApplyFilter_NoTerminalStatusExcluded(t *testing.T) {
	ts := kv.New[string, TerminalStatus]()
	// neither session has a terminal status entry

	sessions := []session.Session{
		newSess("s1", "unknown1"),
		newSess("s2", "unknown2"),
	}
	v := newFilterTestView(sessions, terminal.StatusActive, ts)
	v.applyFilter()

	var sessionIDs []string
	for _, item := range v.list.Items() {
		if ti, ok := item.(TreeItem); ok && ti.IsSession() {
			sessionIDs = append(sessionIDs, ti.Session.ID)
		}
	}
	assert.Empty(t, sessionIDs, "sessions without terminal status are excluded from status filter")
}
