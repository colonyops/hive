package commands

import (
	"testing"
	"time"

	"github.com/colonyops/hive/internal/core/session"
	"github.com/colonyops/hive/internal/core/terminal"
	"github.com/stretchr/testify/assert"
)

func TestPickItem_DisplayName(t *testing.T) {
	tests := []struct {
		name string
		item pickItem
		want string
	}{
		{
			name: "session only",
			item: pickItem{Session: session.Session{Name: "my-session"}},
			want: "my-session",
		},
		{
			name: "with window name",
			item: pickItem{
				Session:    session.Session{Name: "my-session"},
				WindowName: "claude",
			},
			want: "my-session/claude",
		},
		{
			name: "empty window name treated as session only",
			item: pickItem{
				Session:    session.Session{Name: "my-session"},
				WindowName: "",
			},
			want: "my-session",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.item.DisplayName())
		})
	}
}

func TestPickItem_StatusKey(t *testing.T) {
	tests := []struct {
		name string
		item pickItem
		want string
	}{
		{
			name: "session only uses ID",
			item: pickItem{Session: session.Session{ID: "abc123"}},
			want: "abc123",
		},
		{
			name: "window item uses ID/windowIndex",
			item: pickItem{
				Session:     session.Session{ID: "abc123"},
				WindowIndex: "2",
			},
			want: "abc123/2",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.item.statusKey())
		})
	}
}

func TestSortItemsInitial(t *testing.T) {
	now := time.Now()

	items := []pickItem{
		{Session: session.Session{ID: "z", Name: "zebra"}},
		{Session: session.Session{ID: "a", Name: "alpha"}},
		{Session: session.Session{ID: "r1", Name: "recent-old"}, IsRecent: true},
		{Session: session.Session{ID: "m", Name: "middle"}},
		{Session: session.Session{ID: "r2", Name: "recent-new"}, IsRecent: true},
	}
	recents := map[string]time.Time{
		"r1": now.Add(-10 * time.Minute),
		"r2": now.Add(-2 * time.Minute),
	}

	sortItemsInitial(items, recents)

	names := make([]string, len(items))
	for i, item := range items {
		names[i] = item.Session.Name
	}

	// Recents first (most recent first), then alphabetical
	assert.Equal(t, []string{"recent-new", "recent-old", "alpha", "middle", "zebra"}, names)
}

func TestSortItemsInitial_CapsRecentsToMax(t *testing.T) {
	now := time.Now()
	items := make([]pickItem, 5)
	recents := make(map[string]time.Time)

	for i := range items {
		id := string(rune('a' + i))
		items[i] = pickItem{
			Session:  session.Session{ID: id, Name: "session-" + id},
			IsRecent: true,
		}
		recents[id] = now.Add(-time.Duration(i) * time.Minute)
	}

	sortItemsInitial(items, recents)

	recentCount := 0
	for _, item := range items {
		if item.IsRecent {
			recentCount++
		}
	}
	assert.Equal(t, maxRecents, recentCount, "should cap IsRecent to maxRecents")
}

func TestApplyFilter_SubstringMatch(t *testing.T) {
	m := pickModel{
		items: []pickItem{
			{Session: session.Session{ID: "1", Name: "auth-service", Remote: "git@github.com:org/auth.git"}},
			{Session: session.Session{ID: "2", Name: "web-frontend", Remote: "git@github.com:org/web.git"}},
			{Session: session.Session{ID: "3", Name: "auth-worker", Remote: "git@github.com:org/auth.git"}},
		},
		statuses:     map[string]terminal.Status{},
		statusFilter: "all",
	}

	tests := []struct {
		name     string
		query    string
		expected []string
	}{
		{
			name:     "empty query returns all",
			query:    "",
			expected: []string{"auth-service", "web-frontend", "auth-worker"},
		},
		{
			name:     "filter by name",
			query:    "auth",
			expected: []string{"auth-service", "auth-worker"},
		},
		{
			name:     "case insensitive",
			query:    "AUTH",
			expected: []string{"auth-service", "auth-worker"},
		},
		{
			name:     "filter by repo name",
			query:    "web",
			expected: []string{"web-frontend"},
		},
		{
			name:     "no matches",
			query:    "zzz",
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m.input.SetValue(tt.query)
			m.applyFilter()

			var names []string
			for _, item := range m.filtered {
				names = append(names, item.Session.Name)
			}
			assert.Equal(t, tt.expected, names)
		})
	}
}

func TestApplyFilter_StatusFilter(t *testing.T) {
	m := pickModel{
		items: []pickItem{
			{Session: session.Session{ID: "1", Name: "active-session"}},
			{Session: session.Session{ID: "2", Name: "approval-session"}},
			{Session: session.Session{ID: "3", Name: "missing-session"}},
		},
		statuses: map[string]terminal.Status{
			"1": terminal.StatusActive,
			"2": terminal.StatusApproval,
			"3": terminal.StatusMissing,
		},
	}

	tests := []struct {
		name     string
		filter   string
		expected []string
	}{
		{
			name:     "all shows everything",
			filter:   "all",
			expected: []string{"active-session", "approval-session", "missing-session"},
		},
		{
			name:     "default hides missing",
			filter:   "",
			expected: []string{"active-session", "approval-session"},
		},
		{
			name:     "filter active only",
			filter:   "active",
			expected: []string{"active-session"},
		},
		{
			name:     "filter approval only",
			filter:   "approval",
			expected: []string{"approval-session"},
		},
		{
			name:     "filter missing only",
			filter:   "missing",
			expected: []string{"missing-session"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m.statusFilter = tt.filter
			m.input.SetValue("")
			m.applyFilter()

			var names []string
			for _, item := range m.filtered {
				names = append(names, item.Session.Name)
			}
			assert.Equal(t, tt.expected, names)
		})
	}
}

func TestApplyFilter_DefaultShowsAllBeforeStatusLoaded(t *testing.T) {
	m := pickModel{
		items: []pickItem{
			{Session: session.Session{ID: "1", Name: "session-a"}},
			{Session: session.Session{ID: "2", Name: "session-b"}},
		},
		statuses:     map[string]terminal.Status{}, // empty = not loaded yet
		statusFilter: "",                           // default
	}

	m.input.SetValue("")
	m.applyFilter()

	assert.Len(t, m.filtered, 2, "should show all items when statuses haven't loaded")
}

func TestApplyFilter_CursorClamp(t *testing.T) {
	m := pickModel{
		items: []pickItem{
			{Session: session.Session{ID: "1", Name: "aaa"}},
			{Session: session.Session{ID: "2", Name: "bbb"}},
			{Session: session.Session{ID: "3", Name: "ccc"}},
		},
		statuses:     map[string]terminal.Status{},
		statusFilter: "all",
		cursor:       2,
	}

	// Filter to 1 item — cursor should clamp
	m.input.SetValue("aaa")
	m.applyFilter()

	assert.Equal(t, 0, m.cursor)
}

func TestApplyFilter_CombinedTextAndStatus(t *testing.T) {
	m := pickModel{
		items: []pickItem{
			{Session: session.Session{ID: "1", Name: "auth-active"}},
			{Session: session.Session{ID: "2", Name: "auth-missing"}},
			{Session: session.Session{ID: "3", Name: "web-active"}},
		},
		statuses: map[string]terminal.Status{
			"1": terminal.StatusActive,
			"2": terminal.StatusMissing,
			"3": terminal.StatusActive,
		},
		statusFilter: "active",
	}

	m.input.SetValue("auth")
	m.applyFilter()

	var names []string
	for _, item := range m.filtered {
		names = append(names, item.Session.Name)
	}
	assert.Equal(t, []string{"auth-active"}, names)
}

func TestSortItemsInitial_NoRecents(t *testing.T) {
	items := []pickItem{
		{Session: session.Session{ID: "z", Name: "zebra"}},
		{Session: session.Session{ID: "a", Name: "alpha"}},
		{Session: session.Session{ID: "m", Name: "middle"}},
	}

	sortItemsInitial(items, nil)

	names := make([]string, len(items))
	for i, item := range items {
		names[i] = item.Session.Name
	}
	assert.Equal(t, []string{"alpha", "middle", "zebra"}, names)
}

func TestSortItemsInitial_Stable(t *testing.T) {
	// Items with same sort key should maintain relative order
	items := []pickItem{
		{Session: session.Session{ID: "1", Name: "alpha"}},
		{Session: session.Session{ID: "2", Name: "alpha"}},
	}

	sortItemsInitial(items, nil)

	assert.Equal(t, "1", items[0].Session.ID)
	assert.Equal(t, "2", items[1].Session.ID)
}

// TestStatusCycle verifies the tab cycling order matches the defined cycle.
func TestStatusCycle(t *testing.T) {
	cycle := []string{"", "all", "active", "approval", "ready", "missing"}

	for i, current := range cycle {
		next := cycle[(i+1)%len(cycle)]

		// Find current in cycle
		idx := 0
		for j, s := range cycle {
			if s == current {
				idx = j
				break
			}
		}

		got := cycle[(idx+1)%len(cycle)]
		assert.Equal(t, next, got, "from %q should cycle to %q", current, next)
	}

	// Verify it wraps around: last element cycles back to first
	assert.Equal(t, cycle[0], cycle[(len(cycle)-1+1)%len(cycle)],
		"last status should wrap to first")
}

func TestApplyFilter_WindowItemStatusKey(t *testing.T) {
	// Window items use "sessionID/windowIndex" as status key
	m := pickModel{
		items: []pickItem{
			{
				Session:     session.Session{ID: "s1", Name: "my-session"},
				WindowName:  "claude",
				WindowIndex: "0",
			},
			{
				Session:     session.Session{ID: "s1", Name: "my-session"},
				WindowName:  "shell",
				WindowIndex: "1",
			},
		},
		statuses: map[string]terminal.Status{
			"s1/0": terminal.StatusActive,
			"s1/1": terminal.StatusMissing,
		},
		statusFilter: "active",
	}

	m.input.SetValue("")
	m.applyFilter()

	assert.Len(t, m.filtered, 1)
	assert.Equal(t, "claude", m.filtered[0].WindowName)
}

func TestApplyFilter_SearchIncludesRepoName(t *testing.T) {
	m := pickModel{
		items: []pickItem{
			{Session: session.Session{
				ID:     "1",
				Name:   "fix-bug",
				Remote: "git@github.com:myorg/special-repo.git",
			}},
			{Session: session.Session{
				ID:     "2",
				Name:   "add-feature",
				Remote: "git@github.com:myorg/other-repo.git",
			}},
		},
		statuses:     map[string]terminal.Status{},
		statusFilter: "all",
	}

	m.input.SetValue("special")
	m.applyFilter()

	assert.Len(t, m.filtered, 1)
	assert.Equal(t, "fix-bug", m.filtered[0].Session.Name)
}

// Verify the filter uses strings.Contains not exact match
func TestApplyFilter_PartialMatch(t *testing.T) {
	m := pickModel{
		items: []pickItem{
			{Session: session.Session{ID: "1", Name: "authentication-service"}},
		},
		statuses:     map[string]terminal.Status{},
		statusFilter: "all",
	}

	m.input.SetValue("auth")
	m.applyFilter()

	assert.Len(t, m.filtered, 1)
}
