package app

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/desktop/internal/app/agentws"
	"github.com/colonyops/hive/cmd/desktop/internal/app/data/queries"
	"github.com/colonyops/hive/cmd/desktop/internal/app/data/stores"
	"github.com/colonyops/hive/cmd/desktop/internal/app/schedule"
)

func newTestScheduleStore(t *testing.T) scheduleStore {
	t.Helper()
	db, err := queries.Open(t.Context(), t.TempDir(), queries.DefaultOpenOptions())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	return scheduleStore{schedules: stores.New(db, stores.Options{}).Schedules}
}

// The adapter is the only place time.Time meets the tables' unix milliseconds,
// so a round trip is what proves neither direction silently truncates or
// shifts.
func TestScheduleStoreAdapterRoundTripsTimes(t *testing.T) {
	adapter := newTestScheduleStore(t)
	at := time.UnixMilli(1_764_500_100_000)

	require.NoError(t, adapter.SaveCursor(t.Context(), schedule.Cursor{
		Workspace: "demo", ID: "weekly", EvaluatedThrough: at, Cron: "0 9 * * 5",
	}))

	cursor, ok, err := adapter.Cursor(t.Context(), "demo", "weekly")
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, "demo", cursor.Workspace)
	assert.Equal(t, "weekly", cursor.ID)
	assert.Equal(t, "0 9 * * 5", cursor.Cron)
	assert.True(t, cursor.EvaluatedThrough.Equal(at), "the cursor comes back at the instant it was saved")

	_, ok, err = adapter.Cursor(t.Context(), "demo", "never-saved")
	require.NoError(t, err)
	assert.False(t, ok, "an unknown schedule is absent, not an error")

	scheduledFor := time.UnixMilli(1_764_500_000_000)
	startedAt := time.UnixMilli(1_764_500_060_000)
	stored, err := adapter.InsertRun(t.Context(), schedule.Run{
		Workspace: "demo", ScheduleID: "weekly", ScheduleName: "Weekly summary",
		ScheduledFor: scheduledFor, StartedAt: startedAt,
		Reason: schedule.ReasonCatchUp, Missed: 2, Status: schedule.StatusLaunched,
		SessionID: 42, Prompt: "summarize",
	})
	require.NoError(t, err)
	assert.NotZero(t, stored.ID, "the store assigns the id")
	assert.True(t, stored.ScheduledFor.Equal(scheduledFor))
	assert.True(t, stored.StartedAt.Equal(startedAt))
	assert.Equal(t, schedule.ReasonCatchUp, stored.Reason)
	assert.Equal(t, schedule.StatusLaunched, stored.Status)
	assert.Equal(t, 2, stored.Missed)
	assert.Equal(t, int64(42), stored.SessionID)

	last, ok, err := adapter.LastLaunchedRun(t.Context(), "demo", "weekly")
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, stored.ID, last.ID)
}

// The scheduler hands over whole cursors; only their identity reaches the
// store's prune.
func TestScheduleStoreAdapterPrunesByCursorIdentity(t *testing.T) {
	adapter := newTestScheduleStore(t)
	now := time.UnixMilli(1_764_500_100_000)

	for _, cursor := range []schedule.Cursor{
		{Workspace: "demo", ID: "keep", EvaluatedThrough: now, Cron: "@daily"},
		{Workspace: "demo", ID: "gone", EvaluatedThrough: now, Cron: "@daily"},
	} {
		require.NoError(t, adapter.SaveCursor(t.Context(), cursor))
	}

	require.NoError(t, adapter.PruneCursors(t.Context(), []string{"demo"}, []schedule.Cursor{
		{Workspace: "demo", ID: "keep", EvaluatedThrough: now.Add(time.Hour), Cron: "@hourly"},
	}))

	_, ok, err := adapter.Cursor(t.Context(), "demo", "keep")
	require.NoError(t, err)
	assert.True(t, ok, "a cursor is kept by identity, whatever else the scheduler's copy says")
	_, ok, err = adapter.Cursor(t.Context(), "demo", "gone")
	require.NoError(t, err)
	assert.False(t, ok)
}

// TestScheduleSnapshotListsOnlyTheValidWorkspaces: the workspaces are the
// prune scope, so a workspace whose manifest does not parse has to be absent
// from them -- and absent from the same read that skipped its schedules, so
// the two can never disagree about which workspaces exist.
func TestScheduleSnapshotListsOnlyTheValidWorkspaces(t *testing.T) {
	root := t.TempDir()
	writeAgentWorkspaceManifest(t, root, "demo", agentWorkspaceYAML("Demo", "claude", ""))
	writeAgentWorkspaceManifest(t, root, "broken", brokenScheduleManifest)

	workspaces := agentws.NewStore(root)
	require.NoError(t, workspaces.Reload())

	snapshot := scheduleWorkspaces{store: workspaces}.Snapshot()
	assert.Equal(t, []string{"demo"}, snapshot.Workspaces)
	assert.Empty(t, snapshot.Specs)
}

// TestScheduleLauncherFailsOnAnImmediateExit: a scheduled launch has no pane
// to show the "exited immediately" notice on, so an agent CLI that is not on
// PATH has to reach the run history as a failure rather than as a launched run
// carrying a chat id nothing is running behind.
func TestScheduleLauncherFailsOnAnImmediateExit(t *testing.T) {
	isolateConfig(t)
	root := t.TempDir()
	writeAgentWorkspaceManifest(t, root, "demo", agentWorkspaceYAML("Demo", "this-binary-does-not-exist-anywhere-12345"+agentws.PromptTail, ""))
	svc := newTestAgentWorkspacesService(t, root, nil)

	launcher := scheduleLauncher{workspaces: svc}
	_, err := launcher.Launch(t.Context(), schedule.LaunchRequest{Workspace: "demo", Name: "weekly", Prompt: "go"})

	require.Error(t, err)
	assert.Equal(t, KindInternal, KindOf(err))
	assert.Contains(t, err.Error(), "exited immediately")
}
