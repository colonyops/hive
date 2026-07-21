package pipelinedb

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestJobs_LifecyclePagingAndActiveWindow(t *testing.T) {
	database := openTestDB(t)
	ctx := context.Background()

	first, err := database.InsertJob(ctx, JobRecord{
		CreatedAt: 100, UpdatedAt: 100, Status: "queued", Label: "First",
		Step: "Queued", ActionID: "review", Target: "item-1",
	})
	require.NoError(t, err)
	assert.Nil(t, first.CommandID)

	running, err := database.SetJobRunning(ctx, first.ID, 200, "Running…", 41)
	require.NoError(t, err)
	require.NotNil(t, running.CommandID)
	assert.Equal(t, int64(41), *running.CommandID)
	assert.Equal(t, "running", running.Status)

	done, err := database.SetJobStatus(ctx, first.ID, 300, "done", "Completed", "")
	require.NoError(t, err)
	require.NotNil(t, done.CommandID)
	assert.Equal(t, int64(41), *done.CommandID, "terminal transition must preserve command_id")

	failed, err := database.InsertJob(ctx, JobRecord{
		CreatedAt: 400, UpdatedAt: 499, Status: "failed", Label: "Failed",
		Step: "Failed", ActionID: "review", Target: "item-2", Error: "boom",
	})
	require.NoError(t, err)
	queued, err := database.InsertJob(ctx, JobRecord{
		CreatedAt: 500, UpdatedAt: 1, Status: "queued", Label: "Queued", Step: "Queued",
	})
	require.NoError(t, err)

	page, err := database.ListJobs(ctx, 0, 2)
	require.NoError(t, err)
	require.Len(t, page, 2)
	assert.Equal(t, []int64{queued.ID, failed.ID}, []int64{page[0].ID, page[1].ID})
	older, err := database.ListJobs(ctx, page[1].ID, 2)
	require.NoError(t, err)
	require.Len(t, older, 1)
	assert.Equal(t, first.ID, older[0].ID)

	active, err := database.ListActiveJobs(ctx, 500)
	require.NoError(t, err)
	require.Len(t, active, 1)
	assert.Equal(t, queued.ID, active[0].ID, "terminal row just outside the boundary is excluded")
	active, err = database.ListActiveJobs(ctx, 499)
	require.NoError(t, err)
	require.Len(t, active, 2)
	assert.Equal(t, []int64{queued.ID, failed.ID}, []int64{active[0].ID, active[1].ID}, "boundary is inclusive")

	_, found, err := database.FindRunningJobByCommandID(ctx, 41)
	require.NoError(t, err)
	assert.False(t, found, "terminal jobs are not resumable")

	second, err := database.InsertJob(ctx, JobRecord{CreatedAt: 600, UpdatedAt: 600, Status: "queued", Label: "Second"})
	require.NoError(t, err)
	_, err = database.SetJobRunning(ctx, second.ID, 601, "Running…", 42)
	require.NoError(t, err)
	resumed, found, err := database.FindRunningJobByCommandID(ctx, 42)
	require.NoError(t, err)
	require.True(t, found)
	assert.Equal(t, second.ID, resumed.ID)
}

func TestJobs_ActiveJobPersistsAcrossReopen(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()
	database, err := Open(dir, DefaultOpenOptions())
	require.NoError(t, err)
	_, err = database.Conn().ExecContext(ctx, `
		INSERT INTO output_command (action_id, key, payload, status, created_at)
		VALUES ('review', 'item-1', X'7B7D', 'pending', 1)`)
	require.NoError(t, err)
	job, err := database.InsertJob(ctx, JobRecord{
		CreatedAt: 100, UpdatedAt: 100, Status: "queued", Label: "Persisted", Step: "Queued",
	})
	require.NoError(t, err)
	_, err = database.SetJobRunning(ctx, job.ID, 101, "Running…", 1)
	require.NoError(t, err)
	require.NoError(t, database.Close())

	reopened, err := Open(dir, DefaultOpenOptions())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, reopened.Close()) })
	active, err := reopened.ListActiveJobs(ctx, 1_000)
	require.NoError(t, err)
	require.Len(t, active, 1)
	assert.Equal(t, job.ID, active[0].ID)
	require.NotNil(t, active[0].CommandID)
	assert.Equal(t, int64(1), *active[0].CommandID)
}
