package stores

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestScheduleStoreCursors(t *testing.T) {
	st, _ := openTestStores(t)
	ctx := t.Context()

	_, ok, err := st.Schedules.Cursor(ctx, "ws-1", "weekly")
	require.NoError(t, err)
	assert.False(t, ok, "a cursor that was never written is absent, not an error")

	require.NoError(t, st.Schedules.SaveCursor(ctx, ScheduleCursor{Workspace: "ws-1", ScheduleID: "weekly", EvaluatedThrough: 100, Cron: "0 9 * * 5"}))
	cursor, ok, err := st.Schedules.Cursor(ctx, "ws-1", "weekly")
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, ScheduleCursor{Workspace: "ws-1", ScheduleID: "weekly", EvaluatedThrough: 100, Cron: "0 9 * * 5"}, cursor)

	require.NoError(t, st.Schedules.SaveCursor(ctx, ScheduleCursor{Workspace: "ws-1", ScheduleID: "weekly", EvaluatedThrough: 200, Cron: "0 10 * * 5"}))
	cursor, _, err = st.Schedules.Cursor(ctx, "ws-1", "weekly")
	require.NoError(t, err)
	assert.Equal(t, int64(200), cursor.EvaluatedThrough, "a second save replaces the row")
	assert.Equal(t, "0 10 * * 5", cursor.Cron)

	require.NoError(t, st.Schedules.SaveCursor(ctx, ScheduleCursor{Workspace: "ws-2", ScheduleID: "daily", EvaluatedThrough: 50, Cron: "@daily"}))
	cursors, err := st.Schedules.ListCursors(ctx)
	require.NoError(t, err)
	assert.Len(t, cursors, 2)
}

// PruneCursors must drop every cursor outside the live set, but only inside
// the workspaces the caller could read: a workspace whose manifest did not
// parse contributed no schedules to keep, and pruning it would delete the
// state that says how far its schedules got.
func TestScheduleStorePrunesCursorsOutsideKeepWithinScope(t *testing.T) {
	st, _ := openTestStores(t)
	ctx := t.Context()

	for _, cursor := range []ScheduleCursor{
		{Workspace: "demo", ScheduleID: "keep", EvaluatedThrough: 1, Cron: "@daily"},
		{Workspace: "demo", ScheduleID: "gone", EvaluatedThrough: 1, Cron: "@daily"},
		{Workspace: "emptied", ScheduleID: "gone", EvaluatedThrough: 1, Cron: "@daily"},
		{Workspace: "broken", ScheduleID: "held", EvaluatedThrough: 1, Cron: "@daily"},
	} {
		require.NoError(t, st.Schedules.SaveCursor(ctx, cursor))
	}

	require.NoError(t, st.Schedules.PruneCursors(ctx, []string{"demo", "emptied"}, []ScheduleRef{{Workspace: "demo", ScheduleID: "keep"}}))

	cursors, err := st.Schedules.ListCursors(ctx)
	require.NoError(t, err)
	refs := make([]ScheduleRef, 0, len(cursors))
	for _, c := range cursors {
		refs = append(refs, ScheduleRef{Workspace: c.Workspace, ScheduleID: c.ScheduleID})
	}
	assert.ElementsMatch(t, []ScheduleRef{{Workspace: "demo", ScheduleID: "keep"}, {Workspace: "broken", ScheduleID: "held"}}, refs)
}

func TestScheduleStoreInsertRunRoundTripsWithTheAssignedID(t *testing.T) {
	for _, tt := range []struct {
		name      string
		status    string
		sessionID int64
		wantNull  bool
	}{
		{name: "a launched run keeps its chat id", status: "launched", sessionID: 7},
		{name: "a run with no chat stores NULL, not 0", status: "skipped", wantNull: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			st, db := openTestStores(t)
			ctx := t.Context()

			run, err := st.Schedules.InsertRun(ctx, ScheduleRun{
				Workspace: "ws-1", ScheduleID: "weekly", ScheduleName: "Weekly summary",
				ScheduledFor: 100, StartedAt: 100, Reason: "due", Status: tt.status,
				SessionID: tt.sessionID, Prompt: "Summarize.",
			})
			require.NoError(t, err)
			assert.NotZero(t, run.ID)
			assert.Equal(t, tt.sessionID, run.SessionID)

			stored, err := st.Schedules.ListRuns(ctx, "ws-1", "weekly", 10)
			require.NoError(t, err)
			require.Len(t, stored, 1)
			assert.Equal(t, run, stored[0])

			var raw any
			require.NoError(t, db.Conn().QueryRowContext(ctx, `SELECT session_id FROM schedule_run WHERE id = ?`, run.ID).Scan(&raw))
			if tt.wantNull {
				assert.Nil(t, raw, "session 0 must not read back as a chat id")
			} else {
				assert.NotNil(t, raw)
			}
		})
	}
}

func TestScheduleStoreListRunsIsScopedToOneScheduleNewestFirst(t *testing.T) {
	st, _ := openTestStores(t)
	ctx := t.Context()

	weeklyOld := insertScheduleRun(t, st, "ws-1", "weekly", 100, "launched")
	insertScheduleRun(t, st, "ws-1", "daily", 150, "launched")
	weeklyNew := insertScheduleRun(t, st, "ws-1", "weekly", 200, "launched")
	insertScheduleRun(t, st, "ws-2", "weekly", 300, "launched")

	runs, err := st.Schedules.ListRuns(ctx, "ws-1", "weekly", 10)
	require.NoError(t, err)
	require.Len(t, runs, 2)
	assert.Equal(t, []int64{weeklyNew.ID, weeklyOld.ID}, []int64{runs[0].ID, runs[1].ID})

	limited, err := st.Schedules.ListRuns(ctx, "ws-1", "weekly", 1)
	require.NoError(t, err)
	require.Len(t, limited, 1)
	assert.Equal(t, weeklyNew.ID, limited[0].ID)
}

func TestScheduleStoreLastLaunchedRunIgnoresFailedAndSkipped(t *testing.T) {
	st, _ := openTestStores(t)
	ctx := t.Context()

	_, ok, err := st.Schedules.LastLaunchedRun(ctx, "ws-1", "weekly")
	require.NoError(t, err)
	assert.False(t, ok)

	launched := insertScheduleRun(t, st, "ws-1", "weekly", 100, "launched")
	insertScheduleRun(t, st, "ws-1", "weekly", 200, "failed")
	insertScheduleRun(t, st, "ws-1", "weekly", 300, "skipped")

	last, ok, err := st.Schedules.LastLaunchedRun(ctx, "ws-1", "weekly")
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, launched.ID, last.ID, "later failed/skipped runs must not shadow the last launched run")

	newer := insertScheduleRun(t, st, "ws-1", "weekly", 400, "launched")
	last, _, err = st.Schedules.LastLaunchedRun(ctx, "ws-1", "weekly")
	require.NoError(t, err)
	assert.Equal(t, newer.ID, last.ID)
}

func TestScheduleStoreInsertRunPrunesToTheLimitPerSchedule(t *testing.T) {
	st, _ := openTestStores(t)
	ctx := t.Context()

	const total = ScheduleRunLimit + 5
	ids := make([]int64, 0, total)
	for i := range total {
		ids = append(ids, insertScheduleRun(t, st, "ws-1", "weekly", int64(i), "launched").ID)
	}
	other := insertScheduleRun(t, st, "ws-1", "daily", 1, "launched")

	runs, err := st.Schedules.ListRuns(ctx, "ws-1", "weekly", total+10)
	require.NoError(t, err)
	require.Len(t, runs, ScheduleRunLimit)
	kept := make(map[int64]bool, len(runs))
	for _, run := range runs {
		kept[run.ID] = true
	}
	for i, id := range ids {
		assert.Equal(t, i >= total-ScheduleRunLimit, kept[id], "run %d (index %d) retention mismatch", id, i)
	}

	daily, err := st.Schedules.ListRuns(ctx, "ws-1", "daily", 10)
	require.NoError(t, err)
	require.Len(t, daily, 1, "pruning one schedule must not touch another schedule's runs")
	assert.Equal(t, other.ID, daily[0].ID)
}

func TestScheduleStoreDeleteByWorkspaceLeavesOtherWorkspacesAlone(t *testing.T) {
	st, _ := openTestStores(t)
	ctx := t.Context()

	insertScheduleRun(t, st, "ws-1", "weekly", 100, "launched")
	insertScheduleRun(t, st, "ws-1", "daily", 200, "launched")
	kept := insertScheduleRun(t, st, "ws-2", "weekly", 300, "launched")
	for _, cursor := range []ScheduleCursor{
		{Workspace: "ws-1", ScheduleID: "weekly", EvaluatedThrough: 100, Cron: "@weekly"},
		{Workspace: "ws-1", ScheduleID: "daily", EvaluatedThrough: 100, Cron: "@daily"},
		{Workspace: "ws-2", ScheduleID: "weekly", EvaluatedThrough: 100, Cron: "@weekly"},
	} {
		require.NoError(t, st.Schedules.SaveCursor(ctx, cursor))
	}

	require.NoError(t, st.Schedules.DeleteByWorkspace(ctx, "ws-1"))

	for _, id := range []string{"weekly", "daily"} {
		runs, err := st.Schedules.ListRuns(ctx, "ws-1", id, 10)
		require.NoError(t, err)
		assert.Empty(t, runs, "schedule %q should have no runs left", id)
	}
	runs, err := st.Schedules.ListRuns(ctx, "ws-2", "weekly", 10)
	require.NoError(t, err)
	require.Len(t, runs, 1)
	assert.Equal(t, kept.ID, runs[0].ID)

	cursors, err := st.Schedules.ListCursors(ctx)
	require.NoError(t, err)
	require.Len(t, cursors, 1)
	assert.Equal(t, "ws-2", cursors[0].Workspace)
}

func insertScheduleRun(t *testing.T, st *Stores, workspace, scheduleID string, startedAt int64, status string) ScheduleRun {
	t.Helper()
	run, err := st.Schedules.InsertRun(t.Context(), ScheduleRun{
		Workspace: workspace, ScheduleID: scheduleID, ScheduleName: scheduleID,
		ScheduledFor: startedAt, StartedAt: startedAt, Reason: "due", Status: status, Prompt: "Summarize.",
	})
	require.NoError(t, err)
	return run
}
