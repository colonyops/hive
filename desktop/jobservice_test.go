package main

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/internal/desktop/jobs"
	"github.com/colonyops/hive/internal/desktop/pipeline/pipelinedb"
)

func TestJobService_ListAndListActive(t *testing.T) {
	db, err := pipelinedb.Open(t.TempDir(), pipelinedb.DefaultOpenOptions())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })

	now := time.Date(2026, 7, 21, 12, 0, 0, 0, time.UTC)
	store := jobs.NewStore(db, jobs.Options{Now: func() time.Time { return now }})
	service := NewJobService(store)
	ctx := context.Background()

	outside, err := db.InsertJob(ctx, pipelinedb.JobRecord{
		CreatedAt: now.Add(-time.Minute).UnixMilli(), UpdatedAt: now.Add(-jobs.DefaultLingerWindow - time.Millisecond).UnixMilli(),
		Status: "done", Label: "Outside", Step: "Completed",
	})
	require.NoError(t, err)
	inside, err := db.InsertJob(ctx, pipelinedb.JobRecord{
		CreatedAt: now.Add(-time.Minute).UnixMilli(), UpdatedAt: now.Add(-jobs.DefaultLingerWindow + time.Millisecond).UnixMilli(),
		Status: "failed", Label: "Inside", Step: "Failed",
	})
	require.NoError(t, err)
	queued, err := db.InsertJob(ctx, pipelinedb.JobRecord{
		CreatedAt: now.Add(-time.Hour).UnixMilli(), UpdatedAt: now.Add(-time.Hour).UnixMilli(),
		Status: "queued", Label: "Queued", Step: "Queued",
	})
	require.NoError(t, err)

	active, err := service.ListActive()
	require.NoError(t, err)
	require.Len(t, active, 2)
	assert.Equal(t, []int64{queued.ID, inside.ID}, []int64{active[0].ID, active[1].ID})

	page, err := service.List(0, 2)
	require.NoError(t, err)
	require.Len(t, page, 2)
	assert.Equal(t, []int64{queued.ID, inside.ID}, []int64{page[0].ID, page[1].ID})

	older, err := service.List(page[1].ID, 2)
	require.NoError(t, err)
	require.Len(t, older, 1)
	assert.Equal(t, outside.ID, older[0].ID)
}
