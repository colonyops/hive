package sessions

import (
	"testing"

	"github.com/colonyops/hive/internal/core/session"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGroupSessionsByRepo(t *testing.T) {
	tests := []struct {
		name        string
		sessions    []session.Session
		localRemote string
		wantGroups  []struct {
			name     string
			remote   string
			sessions []string // session names in expected order
		}
	}{
		{
			name:     "empty sessions returns nil",
			sessions: nil,
			wantGroups: []struct {
				name     string
				remote   string
				sessions []string
			}{},
		},
		{
			name: "single repo groups all sessions",
			sessions: []session.Session{
				{Name: "session-b", Remote: "git@github.com:user/hive.git"},
				{Name: "session-a", Remote: "git@github.com:user/hive.git"},
			},
			wantGroups: []struct {
				name     string
				remote   string
				sessions []string
			}{
				{name: "hive", remote: "git@github.com:user/hive.git", sessions: []string{"session-a", "session-b"}},
			},
		},
		{
			name: "multiple repos sorted alphabetically",
			sessions: []session.Session{
				{Name: "s1", Remote: "git@github.com:user/zebra.git"},
				{Name: "s2", Remote: "git@github.com:user/alpha.git"},
				{Name: "s3", Remote: "git@github.com:user/beta.git"},
			},
			wantGroups: []struct {
				name     string
				remote   string
				sessions []string
			}{
				{name: "alpha", remote: "git@github.com:user/alpha.git", sessions: []string{"s2"}},
				{name: "beta", remote: "git@github.com:user/beta.git", sessions: []string{"s3"}},
				{name: "zebra", remote: "git@github.com:user/zebra.git", sessions: []string{"s1"}},
			},
		},
		{
			name: "local repo comes first",
			sessions: []session.Session{
				{Name: "s1", Remote: "git@github.com:user/zebra.git"},
				{Name: "s2", Remote: "git@github.com:user/alpha.git"},
				{Name: "s3", Remote: "git@github.com:user/beta.git"},
			},
			localRemote: "git@github.com:user/beta.git",
			wantGroups: []struct {
				name     string
				remote   string
				sessions []string
			}{
				{name: "beta", remote: "git@github.com:user/beta.git", sessions: []string{"s3"}},
				{name: "alpha", remote: "git@github.com:user/alpha.git", sessions: []string{"s2"}},
				{name: "zebra", remote: "git@github.com:user/zebra.git", sessions: []string{"s1"}},
			},
		},
		{
			name: "sessions within groups sorted by name",
			sessions: []session.Session{
				{Name: "charlie", Remote: "git@github.com:user/repo.git"},
				{Name: "alpha", Remote: "git@github.com:user/repo.git"},
				{Name: "bravo", Remote: "git@github.com:user/repo.git"},
			},
			wantGroups: []struct {
				name     string
				remote   string
				sessions []string
			}{
				{name: "repo", remote: "git@github.com:user/repo.git", sessions: []string{"alpha", "bravo", "charlie"}},
			},
		},
		{
			name: "sessions with no remote grouped together",
			sessions: []session.Session{
				{Name: "s1", Remote: ""},
				{Name: "s2", Remote: "git@github.com:user/repo.git"},
				{Name: "s3", Remote: ""},
			},
			wantGroups: []struct {
				name     string
				remote   string
				sessions []string
			}{
				{name: "(no remote)", remote: "(no remote)", sessions: []string{"s1", "s3"}},
				{name: "repo", remote: "git@github.com:user/repo.git", sessions: []string{"s2"}},
			},
		},
		{
			name: "https remote format works",
			sessions: []session.Session{
				{Name: "s1", Remote: "https://github.com/user/my-repo.git"},
			},
			wantGroups: []struct {
				name     string
				remote   string
				sessions []string
			}{
				{name: "my-repo", remote: "https://github.com/user/my-repo.git", sessions: []string{"s1"}},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			groups := GroupSessionsByRepo(tt.sessions, tt.localRemote)

			if len(tt.wantGroups) == 0 {
				assert.Empty(t, groups)
				return
			}

			require.Len(t, groups, len(tt.wantGroups))

			for i, want := range tt.wantGroups {
				got := groups[i]
				assert.Equal(t, want.name, got.Name, "group %d name mismatch", i)
				assert.Equal(t, want.remote, got.Remote, "group %d remote mismatch", i)

				gotNames := make([]string, len(got.Sessions))
				for j, s := range got.Sessions {
					gotNames[j] = s.Name
				}
				assert.Equal(t, want.sessions, gotNames, "group %d sessions mismatch", i)
			}
		})
	}
}

func TestGroupSessionsByRepo_RecycledCount(t *testing.T) {
	sessions := []session.Session{
		{Name: "active1", Remote: "git@github.com:user/repo.git", State: session.StateActive},
		{Name: "active2", Remote: "git@github.com:user/repo.git", State: session.StateActive},
		{Name: "recycled1", Remote: "git@github.com:user/repo.git", State: session.StateRecycled},
		{Name: "recycled2", Remote: "git@github.com:user/repo.git", State: session.StateRecycled},
		{Name: "recycled3", Remote: "git@github.com:user/repo.git", State: session.StateRecycled},
	}

	groups := GroupSessionsByRepo(sessions, "")
	require.Len(t, groups, 1)

	group := groups[0]
	assert.Equal(t, "repo", group.Name)
	assert.Len(t, group.Sessions, 2, "should only contain active sessions")
	assert.Equal(t, 3, group.RecycledCount, "should count recycled sessions")

	// Verify only active sessions are in the slice
	for _, s := range group.Sessions {
		assert.Equal(t, session.StateActive, s.State)
	}
}

func TestGroupSessionsByTag(t *testing.T) {
	tests := []struct {
		name       string
		sessions   []session.Session
		wantGroups []struct {
			name     string
			sessions []string
		}
	}{
		{
			name:     "empty sessions returns nil",
			sessions: nil,
			wantGroups: []struct {
				name     string
				sessions []string
			}{},
		},
		{
			name: "sessions grouped by tag",
			sessions: []session.Session{
				{Name: "s1", Remote: "r1", Metadata: map[string]string{"group": "backend"}},
				{Name: "s2", Remote: "r2", Metadata: map[string]string{"group": "frontend"}},
				{Name: "s3", Remote: "r3", Metadata: map[string]string{"group": "backend"}},
			},
			wantGroups: []struct {
				name     string
				sessions []string
			}{
				{name: "backend", sessions: []string{"s1", "s3"}},
				{name: "frontend", sessions: []string{"s2"}},
			},
		},
		{
			name: "ungrouped sessions go to ungrouped last",
			sessions: []session.Session{
				{Name: "s1", Remote: "r1"},
				{Name: "s2", Remote: "r2", Metadata: map[string]string{"group": "backend"}},
				{Name: "s3", Remote: "r3"},
			},
			wantGroups: []struct {
				name     string
				sessions []string
			}{
				{name: "backend", sessions: []string{"s2"}},
				{name: "(ungrouped)", sessions: []string{"s1", "s3"}},
			},
		},
		{
			name: "groups sorted alphabetically",
			sessions: []session.Session{
				{Name: "s1", Remote: "r1", Metadata: map[string]string{"group": "zebra"}},
				{Name: "s2", Remote: "r2", Metadata: map[string]string{"group": "alpha"}},
				{Name: "s3", Remote: "r3", Metadata: map[string]string{"group": "middle"}},
			},
			wantGroups: []struct {
				name     string
				sessions []string
			}{
				{name: "alpha", sessions: []string{"s2"}},
				{name: "middle", sessions: []string{"s3"}},
				{name: "zebra", sessions: []string{"s1"}},
			},
		},
		{
			name: "recycled sessions separated",
			sessions: []session.Session{
				{Name: "active", Remote: "r1", State: session.StateActive, Metadata: map[string]string{"group": "backend"}},
				{Name: "recycled", Remote: "r2", State: session.StateRecycled, Metadata: map[string]string{"group": "backend"}},
			},
			wantGroups: []struct {
				name     string
				sessions []string
			}{
				{name: "backend", sessions: []string{"active"}},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			groups := GroupSessionsByTag(tt.sessions)

			if len(tt.wantGroups) == 0 {
				assert.Empty(t, groups)
				return
			}

			require.Len(t, groups, len(tt.wantGroups))

			for i, want := range tt.wantGroups {
				got := groups[i]
				assert.Equal(t, want.name, got.Name, "group %d name mismatch", i)

				gotNames := make([]string, len(got.Sessions))
				for j, s := range got.Sessions {
					gotNames[j] = s.Name
				}
				assert.Equal(t, want.sessions, gotNames, "group %d sessions mismatch", i)
			}
		})
	}
}

func TestGroupSessionsByTag_RecycledCount(t *testing.T) {
	sessions := []session.Session{
		{Name: "active1", Remote: "r1", State: session.StateActive, Metadata: map[string]string{"group": "backend"}},
		{Name: "active2", Remote: "r2", State: session.StateActive, Metadata: map[string]string{"group": "backend"}},
		{Name: "recycled1", Remote: "r3", State: session.StateRecycled, Metadata: map[string]string{"group": "backend"}},
		{Name: "recycled2", Remote: "r4", State: session.StateRecycled, Metadata: map[string]string{"group": "backend"}},
	}

	groups := GroupSessionsByTag(sessions)
	require.Len(t, groups, 1)

	group := groups[0]
	assert.Equal(t, "backend", group.Name)
	assert.Len(t, group.Sessions, 2, "should only contain active sessions")
	assert.Equal(t, 2, group.RecycledCount, "should count recycled sessions")
	require.Len(t, group.RecycledSessions, 2)
	assert.Equal(t, "recycled1", group.RecycledSessions[0].Name)
	assert.Equal(t, "recycled2", group.RecycledSessions[1].Name)
}

func TestExtractGroupName(t *testing.T) {
	tests := []struct {
		remote string
		want   string
	}{
		{"git@github.com:user/hive.git", "hive"},
		{"https://github.com/user/my-repo.git", "my-repo"},
		{"", "(no remote)"},
		{"(no remote)", "(no remote)"},
	}

	for _, tt := range tests {
		t.Run(tt.remote, func(t *testing.T) {
			got := extractGroupName(tt.remote)
			assert.Equal(t, tt.want, got)
		})
	}
}
