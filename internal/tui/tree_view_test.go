package tui

import (
	"testing"

	"github.com/hay-kot/hive/internal/core/session"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildTreeItems(t *testing.T) {
	tests := []struct {
		name        string
		groups      []RepoGroup
		localRemote string
		wantHeaders int
		wantItems   int
	}{
		{
			name:        "empty groups returns nil",
			groups:      nil,
			wantHeaders: 0,
			wantItems:   0,
		},
		{
			name: "single group with sessions",
			groups: []RepoGroup{
				{
					Remote: "git@github.com:user/repo.git",
					Name:   "repo",
					Sessions: []session.Session{
						{ID: "abc1", Name: "session-a", State: session.StateActive},
						{ID: "abc2", Name: "session-b", State: session.StateRecycled},
					},
				},
			},
			wantHeaders: 1,
			wantItems:   3, // 1 header + 2 sessions
		},
		{
			name: "multiple groups",
			groups: []RepoGroup{
				{
					Remote:   "git@github.com:user/alpha.git",
					Name:     "alpha",
					Sessions: []session.Session{{ID: "abc1", Name: "s1", State: session.StateActive}},
				},
				{
					Remote:   "git@github.com:user/beta.git",
					Name:     "beta",
					Sessions: []session.Session{{ID: "abc2", Name: "s2", State: session.StateActive}},
				},
			},
			wantHeaders: 2,
			wantItems:   4, // 2 headers + 2 sessions
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			items := BuildTreeItems(tt.groups, tt.localRemote)

			if tt.wantItems == 0 {
				assert.Empty(t, items)
				return
			}

			require.Len(t, items, tt.wantItems)

			// Count headers
			headerCount := 0
			for _, item := range items {
				treeItem := item.(TreeItem)
				if treeItem.IsHeader {
					headerCount++
				}
			}
			assert.Equal(t, tt.wantHeaders, headerCount)
		})
	}
}

func TestBuildTreeItems_HeaderFields(t *testing.T) {
	groups := []RepoGroup{
		{
			Remote: "git@github.com:user/local.git",
			Name:   "local",
			Sessions: []session.Session{
				{ID: "abc1", Name: "active1", State: session.StateActive},
				{ID: "abc2", Name: "active2", State: session.StateActive},
				{ID: "abc3", Name: "recycled1", State: session.StateRecycled},
			},
		},
	}

	items := BuildTreeItems(groups, "git@github.com:user/local.git")
	require.Len(t, items, 4)

	header := items[0].(TreeItem)
	assert.True(t, header.IsHeader)
	assert.Equal(t, "local", header.RepoName)
	assert.True(t, header.IsCurrentRepo)
}

func TestBuildTreeItems_SessionFields(t *testing.T) {
	groups := []RepoGroup{
		{
			Remote: "git@github.com:user/repo.git",
			Name:   "repo",
			Sessions: []session.Session{
				{ID: "abc1", Name: "first", State: session.StateActive},
				{ID: "abc2", Name: "last", State: session.StateRecycled},
			},
		},
	}

	items := BuildTreeItems(groups, "")
	require.Len(t, items, 3)

	// First session
	first := items[1].(TreeItem)
	assert.False(t, first.IsHeader)
	assert.Equal(t, "first", first.Session.Name)
	assert.False(t, first.IsLastInRepo)
	assert.Equal(t, "repo", first.RepoPrefix)

	// Last session
	last := items[2].(TreeItem)
	assert.False(t, last.IsHeader)
	assert.Equal(t, "last", last.Session.Name)
	assert.True(t, last.IsLastInRepo)
	assert.Equal(t, "repo", last.RepoPrefix)
}

func TestTreeItem_FilterValue(t *testing.T) {
	tests := []struct {
		name string
		item TreeItem
		want string
	}{
		{
			name: "header returns empty",
			item: TreeItem{IsHeader: true, RepoName: "repo"},
			want: "",
		},
		{
			name: "session returns repoName + sessionName",
			item: TreeItem{
				IsHeader:   false,
				RepoPrefix: "myrepo",
				Session:    session.Session{Name: "my-session"},
			},
			want: "myrepo my-session",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.item.FilterValue()
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestPadRight(t *testing.T) {
	tests := []struct {
		input string
		width int
		want  string
	}{
		{"abc", 5, "abc  "},
		{"abcde", 5, "abcde"},
		{"abcdef", 5, "abcdef"},
		{"", 3, "   "},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := PadRight(tt.input, tt.width)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestCalculateColumnWidths(t *testing.T) {
	sessions := []session.Session{
		{ID: "abcd1234", Name: "short", Path: "/path1"},
		{ID: "efgh5678", Name: "much-longer-name", Path: "/path2"},
		{ID: "ijkl9012", Name: "medium", Path: "/path3"},
	}

	gitBranches := map[string]string{
		"/path1": "main",
		"/path2": "feature/very-long-branch-name",
		"/path3": "develop",
	}

	widths := CalculateColumnWidths(sessions, gitBranches)

	assert.Equal(t, len("much-longer-name"), widths.Name)
	assert.Equal(t, len("feature/very-long-branch-name"), widths.Branch)
	assert.Equal(t, 4, widths.ID) // All IDs are truncated to 4 chars
}

func TestTreeDelegate_RenderAnimation(t *testing.T) {
	tests := []struct {
		name      string
		sessionID string
		setup     func(*AnimationStore)
		wantEmpty bool
		wantSend  bool
		wantRecv  bool
	}{
		{
			name:      "no animation store",
			sessionID: "session-1",
			setup:     nil,
			wantEmpty: true,
		},
		{
			name:      "no animation for session",
			sessionID: "session-1",
			setup: func(s *AnimationStore) {
				// No animation recorded
			},
			wantEmpty: true,
		},
		{
			name:      "send animation",
			sessionID: "session-1",
			setup: func(s *AnimationStore) {
				s.animations["session-1"] = SessionAnimation{
					Direction: AnimationSend,
					Topic:     "test-topic",
					TicksLeft: 3,
				}
			},
			wantSend: true,
		},
		{
			name:      "recv animation",
			sessionID: "session-1",
			setup: func(s *AnimationStore) {
				s.animations["session-1"] = SessionAnimation{
					Direction: AnimationRecv,
					Topic:     "inbox",
					TicksLeft: 3,
				}
			},
			wantRecv: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := NewTreeDelegate()

			if tt.setup != nil {
				d.AnimationStore = NewAnimationStore(4)
				tt.setup(d.AnimationStore)
			}

			result := d.renderAnimation(tt.sessionID)

			if tt.wantEmpty {
				assert.Empty(t, result)
				return
			}

			if tt.wantSend {
				assert.Contains(t, result, ">")
				assert.Contains(t, result, "{")
			}

			if tt.wantRecv {
				assert.Contains(t, result, "<")
				assert.Contains(t, result, "{")
			}
		})
	}
}

func TestTreeDelegate_RenderAnimation_TopicTruncation(t *testing.T) {
	d := NewTreeDelegate()
	d.AnimationStore = NewAnimationStore(4)

	// Set a very long topic
	d.AnimationStore.animations["session-1"] = SessionAnimation{
		Direction: AnimationSend,
		Topic:     "very-long-topic-name-that-exceeds-limit",
		TicksLeft: 3,
	}

	result := d.renderAnimation("session-1")

	// Should be truncated with ellipsis
	assert.Contains(t, result, "…")
	// Should not contain the full topic name
	assert.NotContains(t, result, "very-long-topic-name-that-exceeds-limit")
}

func TestIsInboxTopic(t *testing.T) {
	tests := []struct {
		topic string
		want  bool
	}{
		{"agent.abc123.inbox", true},
		{"agent.session-id.inbox", true},
		{"agent..inbox", true}, // edge case but matches pattern
		{"inbox", false},
		{"agent.abc123", false},
		{"build.result", false},
		{"agent.abc123.inbox.extra", false},
		{"other.agent.abc123.inbox", false},
	}

	for _, tt := range tests {
		t.Run(tt.topic, func(t *testing.T) {
			got := isInboxTopic(tt.topic)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestTreeDelegate_RenderAnimation_InboxDistinction(t *testing.T) {
	d := NewTreeDelegate()
	d.AnimationStore = NewAnimationStore(4)

	// Test inbox receive animation uses ◀ arrow
	d.AnimationStore.animations["session-1"] = SessionAnimation{
		Direction: AnimationRecv,
		Topic:     "agent.abc123.inbox",
		TicksLeft: 3,
	}

	result := d.renderAnimation("session-1")
	assert.Contains(t, result, "◀") // Inbox uses filled arrow

	// Test regular topic receive animation uses < arrow
	d.AnimationStore.animations["session-2"] = SessionAnimation{
		Direction: AnimationRecv,
		Topic:     "build.result",
		TicksLeft: 3,
	}

	result2 := d.renderAnimation("session-2")
	assert.Contains(t, result2, "<")    // Regular uses standard arrow
	assert.NotContains(t, result2, "◀") // Not the filled arrow
}
