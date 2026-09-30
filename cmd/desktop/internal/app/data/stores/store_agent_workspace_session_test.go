package stores

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAgentSessionStore(t *testing.T) {
	st, _ := openTestStores(t)
	ctx := t.Context()

	t.Run("CreateGetRoundTrip", func(t *testing.T) {
		created, err := st.AgentSessions.Create(ctx, AgentSessionCreate{
			Workspace: "demo", Name: "first pass", Agent: "claude", AgentSessionID: "sess-1",
		})
		require.NoError(t, err)
		assert.NotZero(t, created.ID)
		assert.Equal(t, "demo", created.Workspace)
		assert.Equal(t, "first pass", created.Name)
		assert.Equal(t, "claude", created.Agent)
		assert.Equal(t, "sess-1", created.AgentSessionID)
		assert.Regexp(t, `^[a-z0-9]{8}$`, created.TerminalID)
		assert.Equal(t, created.CreatedAt, created.LastOpenedAt, "creation stamps both from the same clock read")

		got, err := st.AgentSessions.Get(ctx, created.ID)
		require.NoError(t, err)
		assert.Equal(t, created, got)
	})

	t.Run("GetMissingIsNotFound", func(t *testing.T) {
		_, err := st.AgentSessions.Get(ctx, 999999)
		assert.True(t, IsNotFound(err))
	})

	t.Run("Touch", func(t *testing.T) {
		created, err := st.AgentSessions.Create(ctx, AgentSessionCreate{Workspace: "demo", Name: "touch me", Agent: "claude"})
		require.NoError(t, err)

		require.NoError(t, st.AgentSessions.Touch(ctx, created.ID, 2000))
		got, err := st.AgentSessions.Get(ctx, created.ID)
		require.NoError(t, err)
		assert.Equal(t, int64(2000), got.LastOpenedAt)
		assert.Equal(t, created.CreatedAt, got.CreatedAt, "touch never changes created_at")
	})

	t.Run("SetAgentID", func(t *testing.T) {
		created, err := st.AgentSessions.Create(ctx, AgentSessionCreate{Workspace: "demo", Name: "rotate id", Agent: "codex"})
		require.NoError(t, err)
		assert.Empty(t, created.AgentSessionID, "the default is empty until a launch sets it")

		require.NoError(t, st.AgentSessions.SetAgentID(ctx, created.ID, "minted-later"))
		got, err := st.AgentSessions.Get(ctx, created.ID)
		require.NoError(t, err)
		assert.Equal(t, "minted-later", got.AgentSessionID)
	})

	t.Run("Delete", func(t *testing.T) {
		created, err := st.AgentSessions.Create(ctx, AgentSessionCreate{Workspace: "demo", Name: "delete me", Agent: "claude"})
		require.NoError(t, err)

		require.NoError(t, st.AgentSessions.Delete(ctx, created.ID))
		_, err = st.AgentSessions.Get(ctx, created.ID)
		assert.True(t, IsNotFound(err), "a deleted session is not found")

		require.NoError(t, st.AgentSessions.Delete(ctx, created.ID))
	})

	t.Run("ListIsNewestRecordFirstAndScopedToOneWorkspace", func(t *testing.T) {
		// The first record gets the later last_opened_at on purpose: the order
		// is creation order, so touching a session (resuming it) must not move
		// its row.
		older, err := st.AgentSessions.Create(ctx, AgentSessionCreate{Workspace: "list-scope", Name: "older", Agent: "claude"})
		require.NoError(t, err)
		newer, err := st.AgentSessions.Create(ctx, AgentSessionCreate{Workspace: "list-scope", Name: "newer", Agent: "claude"})
		require.NoError(t, err)
		require.NoError(t, st.AgentSessions.Touch(ctx, older.ID, 9000))
		require.NoError(t, st.AgentSessions.Touch(ctx, newer.ID, 2000))
		_, err = st.AgentSessions.Create(ctx, AgentSessionCreate{Workspace: "other-workspace", Name: "elsewhere", Agent: "claude"})
		require.NoError(t, err)

		sessions, err := st.AgentSessions.List(ctx, "list-scope")
		require.NoError(t, err)
		require.Len(t, sessions, 2)
		assert.Equal(t, "newer", sessions[0].Name)
		assert.Equal(t, "older", sessions[1].Name)
	})

	t.Run("DeleteByWorkspaceLeavesOtherWorkspacesAlone", func(t *testing.T) {
		_, err := st.AgentSessions.Create(ctx, AgentSessionCreate{Workspace: "delete-scope-a", Name: "a1", Agent: "claude"})
		require.NoError(t, err)
		_, err = st.AgentSessions.Create(ctx, AgentSessionCreate{Workspace: "delete-scope-a", Name: "a2", Agent: "claude"})
		require.NoError(t, err)
		_, err = st.AgentSessions.Create(ctx, AgentSessionCreate{Workspace: "delete-scope-b", Name: "b1", Agent: "claude"})
		require.NoError(t, err)

		require.NoError(t, st.AgentSessions.DeleteByWorkspace(ctx, "delete-scope-a"))

		sessionsA, err := st.AgentSessions.List(ctx, "delete-scope-a")
		require.NoError(t, err)
		assert.Empty(t, sessionsA)

		sessionsB, err := st.AgentSessions.List(ctx, "delete-scope-b")
		require.NoError(t, err)
		assert.Len(t, sessionsB, 1)
	})

	t.Run("TwoSessionsMayShareAName", func(t *testing.T) {
		first, err := st.AgentSessions.Create(ctx, AgentSessionCreate{Workspace: "shared-name-scope", Name: "duplicate", Agent: "claude"})
		require.NoError(t, err)
		second, err := st.AgentSessions.Create(ctx, AgentSessionCreate{Workspace: "shared-name-scope", Name: "duplicate", Agent: "claude"})
		require.NoError(t, err)
		assert.NotEqual(t, first.ID, second.ID, "the row id is the record identity, not the name")
		assert.NotEqual(t, first.TerminalID, second.TerminalID, "each record gets its own tmux identity")

		sessions, err := st.AgentSessions.List(ctx, "shared-name-scope")
		require.NoError(t, err)
		assert.Len(t, sessions, 2)
	})

	t.Run("ListAllOrdersAcrossWorkspacesByNewestRecord", func(t *testing.T) {
		// last_opened_at values run counter to insertion order on purpose: the
		// cross-workspace list keeps creation order too, so a resume never
		// reshuffles it.
		first, err := st.AgentSessions.Create(ctx, AgentSessionCreate{Workspace: "all-scope-a", Name: "first", Agent: "claude"})
		require.NoError(t, err)
		second, err := st.AgentSessions.Create(ctx, AgentSessionCreate{Workspace: "all-scope-b", Name: "second", Agent: "claude"})
		require.NoError(t, err)
		third, err := st.AgentSessions.Create(ctx, AgentSessionCreate{Workspace: "all-scope-a", Name: "third", Agent: "claude"})
		require.NoError(t, err)
		require.NoError(t, st.AgentSessions.Touch(ctx, first.ID, 3000))
		require.NoError(t, st.AgentSessions.Touch(ctx, second.ID, 1000))
		require.NoError(t, st.AgentSessions.Touch(ctx, third.ID, 2000))

		all, err := st.AgentSessions.ListAll(ctx)
		require.NoError(t, err)
		require.GreaterOrEqual(t, len(all), 3, "carries every prior t.Run's rows too, since ListAll is unscoped")

		var names []string
		for _, s := range all {
			if s.Workspace == "all-scope-a" || s.Workspace == "all-scope-b" {
				names = append(names, s.Name)
			}
		}
		assert.Equal(t, []string{"third", "second", "first"}, names, "newest record first regardless of workspace or last_opened_at")
	})

	t.Run("Rename", func(t *testing.T) {
		created, err := st.AgentSessions.Create(ctx, AgentSessionCreate{Workspace: "demo", Name: "New Chat", Agent: "claude"})
		require.NoError(t, err)

		require.NoError(t, st.AgentSessions.Rename(ctx, created.ID, "triage the flaky test"))
		got, err := st.AgentSessions.Get(ctx, created.ID)
		require.NoError(t, err)
		assert.Equal(t, "triage the flaky test", got.Name)
		assert.Equal(t, created.AgentSessionID, got.AgentSessionID, "a rename touches only the name")
		assert.Equal(t, created.TerminalID, got.TerminalID)
	})
}
