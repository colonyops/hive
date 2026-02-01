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
			name: "single group with sessions and recycled",
			groups: []RepoGroup{
				{
					Remote: "git@github.com:user/repo.git",
					Name:   "repo",
					Sessions: []session.Session{
						{ID: "abc1", Name: "session-a", State: session.StateActive},
					},
					RecycledCount: 1,
				},
			},
			wantHeaders: 1,
			wantItems:   3, // 1 header + 1 session + 1 recycled placeholder
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
			},
			RecycledCount: 1,
		},
	}

	items := BuildTreeItems(groups, "git@github.com:user/local.git")
	require.Len(t, items, 4) // 1 header + 2 active sessions + 1 recycled placeholder

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
				{ID: "abc2", Name: "second", State: session.StateActive},
			},
			RecycledCount: 1,
		},
	}

	items := BuildTreeItems(groups, "")
	require.Len(t, items, 4) // 1 header + 2 sessions + 1 recycled placeholder

	// First session
	first := items[1].(TreeItem)
	assert.False(t, first.IsHeader)
	assert.Equal(t, "first", first.Session.Name)
	assert.False(t, first.IsLastInRepo)
	assert.Equal(t, "repo", first.RepoPrefix)

	// Second session (not last because recycled placeholder follows)
	second := items[2].(TreeItem)
	assert.False(t, second.IsHeader)
	assert.Equal(t, "second", second.Session.Name)
	assert.False(t, second.IsLastInRepo)
	assert.Equal(t, "repo", second.RepoPrefix)

	// Recycled placeholder is last
	recycled := items[3].(TreeItem)
	assert.True(t, recycled.IsRecycledPlaceholder)
	assert.True(t, recycled.IsLastInRepo)
	assert.Equal(t, 1, recycled.RecycledCount)
	assert.Equal(t, "repo", recycled.RepoPrefix)
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
		{
			name: "recycled placeholder returns repoName + recycled",
			item: TreeItem{
				IsRecycledPlaceholder: true,
				RepoPrefix:            "myrepo",
				RecycledCount:         5,
			},
			want: "myrepo recycled",
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
