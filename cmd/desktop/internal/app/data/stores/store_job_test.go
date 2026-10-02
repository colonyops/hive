package stores

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/desktop/internal/app/data/queries"
)

func TestJobs_LifecyclePagingAndActiveWindow(t *testing.T) {
	db, err := queries.Open(t.Context(), t.TempDir(), queries.DefaultOpenOptions())
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	clock := newTestClock()
	st := New(db, Options{Now: clock.Now})
	ctx := t.Context()

	clock.set(100)
	first, err := st.Jobs.Insert(ctx, JobCreate{
		Status: "queued", Label: "First", Step: "Queued", ActionID: "review", Target: "item-1",
	})
	require.NoError(t, err)
	assert.Nil(t, first.CommandID)

	clock.set(200)
	running, err := st.Jobs.SetRunning(ctx, first.ID, "Running…", 41)
	require.NoError(t, err)
	require.NotNil(t, running.CommandID)
	assert.Equal(t, int64(41), *running.CommandID)
	assert.Equal(t, "running", running.Status)

	clock.set(300)
	done, err := st.Jobs.SetStatus(ctx, first.ID, "done", "Completed", "")
	require.NoError(t, err)
	require.NotNil(t, done.CommandID)
	assert.Equal(t, int64(41), *done.CommandID, "terminal transition must preserve command_id")

	clock.set(400)
	failed, err := st.Jobs.Insert(ctx, JobCreate{
		Status: "failed", Label: "Failed", Step: "Failed", ActionID: "review", Target: "item-2", Error: "boom",
	})
	require.NoError(t, err)
	// Updated later than created, on purpose: the active-window boundary test
	// below asserts on updated_at, not created_at.
	clock.set(499)
	_, err = st.Jobs.SetStatus(ctx, failed.ID, "failed", "Failed", "boom")
	require.NoError(t, err)
	clock.set(500)
	queued, err := st.Jobs.Insert(ctx, JobCreate{Status: "queued", Label: "Queued", Step: "Queued"})
	require.NoError(t, err)

	page, err := st.Jobs.List(ctx, 0, 2)
	require.NoError(t, err)
	require.Len(t, page, 2)
	assert.Equal(t, []int64{queued.ID, failed.ID}, []int64{page[0].ID, page[1].ID})
	older, err := st.Jobs.List(ctx, page[1].ID, 2)
	require.NoError(t, err)
	require.Len(t, older, 1)
	assert.Equal(t, first.ID, older[0].ID)

	active, err := st.Jobs.ListActive(ctx, 500)
	require.NoError(t, err)
	require.Len(t, active, 1)
	assert.Equal(t, queued.ID, active[0].ID, "terminal row just outside the boundary is excluded")
	active, err = st.Jobs.ListActive(ctx, 499)
	require.NoError(t, err)
	require.Len(t, active, 2)
	assert.Equal(t, []int64{queued.ID, failed.ID}, []int64{active[0].ID, active[1].ID}, "boundary is inclusive")

	_, found, err := st.Jobs.FindRunningByCommand(ctx, 41)
	require.NoError(t, err)
	assert.False(t, found, "terminal jobs are not resumable")

	clock.set(600)
	second, err := st.Jobs.Insert(ctx, JobCreate{Status: "queued", Label: "Second"})
	require.NoError(t, err)
	_, err = st.Jobs.SetRunning(ctx, second.ID, "Running…", 42)
	require.NoError(t, err)
	resumed, found, err := st.Jobs.FindRunningByCommand(ctx, 42)
	require.NoError(t, err)
	require.True(t, found)
	assert.Equal(t, second.ID, resumed.ID)
}

func TestJobs_ActiveJobPersistsAcrossReopen(t *testing.T) {
	dir := t.TempDir()
	ctx := t.Context()
	db, err := queries.Open(t.Context(), dir, queries.DefaultOpenOptions())
	require.NoError(t, err)
	st := New(db, Options{})
	_, err = db.Conn().ExecContext(ctx, `
		INSERT INTO output_command (action_id, key, payload, status, created_at)
		VALUES ('review', 'item-1', X'7B7D', 'pending', 1)`)
	require.NoError(t, err)
	job, err := st.Jobs.Insert(ctx, JobCreate{Status: "queued", Label: "Persisted", Step: "Queued"})
	require.NoError(t, err)
	_, err = st.Jobs.SetRunning(ctx, job.ID, "Running…", 1)
	require.NoError(t, err)
	require.NoError(t, db.Close())

	reopened, err := queries.Open(t.Context(), dir, queries.DefaultOpenOptions())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, reopened.Close()) })
	st = New(reopened, Options{})
	active, err := st.Jobs.ListActive(ctx, 1_000)
	require.NoError(t, err)
	require.Len(t, active, 1)
	assert.Equal(t, job.ID, active[0].ID)
	require.NotNil(t, active[0].CommandID)
	assert.Equal(t, int64(1), *active[0].CommandID)
}

func TestJobs_SetRunningWithoutCommandLeavesLinkNull(t *testing.T) {
	db, err := queries.Open(t.Context(), t.TempDir(), queries.DefaultOpenOptions())
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	st := New(db, Options{Now: newTestClock().Now})
	ctx := t.Context()

	job, err := st.Jobs.Insert(ctx, JobCreate{
		Status: "queued", Label: "Create session", Step: "Queued", ActionID: "new-session", Target: "sess-1",
	})
	require.NoError(t, err)

	running, err := st.Jobs.SetRunning(ctx, job.ID, "Running…", 0)
	require.NoError(t, err)
	assert.Equal(t, "running", running.Status)
	assert.Nil(t, running.CommandID, "a zero command id marks the job running without a link")

	_, found, err := st.Jobs.FindRunningByCommand(ctx, 0)
	require.NoError(t, err)
	assert.False(t, found, "an unlinked job is not resumable by command")
}
