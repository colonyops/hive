package jobs

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/internal/desktop/pipeline/pipelinedb"
)

func openJobsTestDB(t *testing.T) *pipelinedb.DB {
	t.Helper()
	database, err := pipelinedb.Open(t.TempDir(), pipelinedb.DefaultOpenOptions())
	require.NoError(t, err)
	t.Cleanup(func() { _ = database.Close() })
	return database
}

func TestStore_RecordsLifecycleLabelsAndEmits(t *testing.T) {
	database := openJobsTestDB(t)
	ctx := context.Background()
	now := time.UnixMilli(1_000)
	var emitted []int64
	store := NewStore(database, Options{
		Now:  func() time.Time { return now },
		Emit: func(id int64) { emitted = append(emitted, id) },
	})

	id := store.Begin(ctx, "Review PR", "review", "pr-1")
	require.Positive(t, id)
	now = time.UnixMilli(2_000)
	store.Running(ctx, id, 44)
	assert.Equal(t, id, store.Resume(ctx, 44))
	now = time.UnixMilli(3_000)
	store.Done(ctx, id)
	assert.Zero(t, store.Resume(ctx, 44))
	assert.Equal(t, []int64{id, id, id}, emitted)

	rows, err := store.List(ctx, 0, 10)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	job := rows[0]
	assert.Equal(t, JobStatusDone, job.Status)
	assert.Equal(t, "Completed", job.Step)
	assert.Equal(t, int64(1_000), job.CreatedAt)
	assert.Equal(t, int64(3_000), job.UpdatedAt)
	require.NotNil(t, job.CommandID)
	assert.Equal(t, int64(44), *job.CommandID)

	failedID := store.Begin(ctx, "Deploy", "deploy", "pr-2")
	store.Running(ctx, failedID, 45)
	store.Fail(ctx, failedID, "boom")
	rows, err = store.List(ctx, 0, 10)
	require.NoError(t, err)
	require.Len(t, rows, 2)
	assert.Equal(t, JobStatusFailed, rows[0].Status)
	assert.Equal(t, "Failed", rows[0].Step)
	assert.Equal(t, "boom", rows[0].Error)
	require.NotNil(t, rows[0].CommandID)
	assert.Equal(t, int64(45), *rows[0].CommandID)
	assert.Len(t, emitted, 6)
}

func TestStepFor(t *testing.T) {
	tests := []struct {
		status JobStatus
		want   string
	}{
		{JobStatusQueued, "Queued"},
		{JobStatusRunning, "Running…"},
		{JobStatusDone, "Completed"},
		{JobStatusFailed, "Failed"},
	}
	for _, test := range tests {
		t.Run(test.status.String(), func(t *testing.T) {
			assert.Equal(t, test.want, stepFor(test.status))
		})
	}
}

func TestStore_ListActiveUsesBackendClockWindow(t *testing.T) {
	database := openJobsTestDB(t)
	ctx := context.Background()
	now := time.UnixMilli(10_000)
	store := NewStore(database, Options{Now: func() time.Time { return now }})

	_, err := database.InsertJob(ctx, pipelinedb.JobRecord{
		CreatedAt: 1, UpdatedAt: now.Add(-DefaultLingerWindow).UnixMilli(), Status: "done", Label: "Boundary",
	})
	require.NoError(t, err)
	_, err = database.InsertJob(ctx, pipelinedb.JobRecord{
		CreatedAt: 2, UpdatedAt: now.Add(-DefaultLingerWindow - time.Millisecond).UnixMilli(), Status: "failed", Label: "Outside",
	})
	require.NoError(t, err)
	queued, err := database.InsertJob(ctx, pipelinedb.JobRecord{
		CreatedAt: 3, UpdatedAt: 1, Status: "queued", Label: "Queued",
	})
	require.NoError(t, err)

	active, err := store.ListActive(ctx, DefaultLingerWindow)
	require.NoError(t, err)
	require.Len(t, active, 2)
	assert.Equal(t, queued.ID, active[0].ID)
	assert.Equal(t, "Boundary", active[1].Label)
}

func TestStore_BeginFailureAndZeroTransitionsAreNoOps(t *testing.T) {
	database := openJobsTestDB(t)
	require.NoError(t, database.Close())
	store := NewStore(database, Options{})
	id := store.Begin(context.Background(), "Review", "review", "pr-1")
	assert.Zero(t, id)
	assert.NotPanics(t, func() {
		store.Running(context.Background(), 0, 1)
		store.Done(context.Background(), 0)
		store.Fail(context.Background(), 0, "ignored")
	})
}
