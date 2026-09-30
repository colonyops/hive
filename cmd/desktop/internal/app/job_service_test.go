package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rs/zerolog"

	"github.com/colonyops/hive/cmd/desktop/internal/app/data/queries"
	"github.com/colonyops/hive/cmd/desktop/internal/app/data/stores"
	"github.com/colonyops/hive/cmd/desktop/internal/app/events"
	"github.com/colonyops/hive/cmd/desktop/internal/app/jobs"
)

func newTestJobService(t *testing.T) (*JobService, <-chan events.JobsUpdated) {
	t.Helper()
	db, err := queries.Open(t.Context(), t.TempDir(), queries.DefaultOpenOptions())
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	bus := newTestBus(t)
	ch := subscribeEvents[events.JobsUpdated](t, bus)
	return newJobService(stores.New(db, stores.Options{}).Jobs, bus, zerolog.Nop()), ch
}

func TestJobService_ListAndListActive(t *testing.T) {
	db, err := queries.Open(t.Context(), t.TempDir(), queries.DefaultOpenOptions())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })

	now := time.Date(2026, 7, 21, 12, 0, 0, 0, time.UTC)
	st := stores.New(db, stores.Options{Now: func() time.Time { return now }})
	service := newJobService(st.Jobs, newTestBus(t), zerolog.Nop())
	ctx := t.Context()

	outside, err := db.InsertJob(ctx, queries.InsertJobParams{
		CreatedAt: now.Add(-time.Minute).UnixMilli(), UpdatedAt: now.Add(-jobs.DefaultLingerWindow - time.Millisecond).UnixMilli(),
		Status: "done", Label: "Outside", Step: "Completed",
	})
	require.NoError(t, err)
	inside, err := db.InsertJob(ctx, queries.InsertJobParams{
		CreatedAt: now.Add(-time.Minute).UnixMilli(), UpdatedAt: now.Add(-jobs.DefaultLingerWindow + time.Millisecond).UnixMilli(),
		Status: "failed", Label: "Inside", Step: "Failed",
	})
	require.NoError(t, err)
	queued, err := db.InsertJob(ctx, queries.InsertJobParams{
		CreatedAt: now.Add(-time.Hour).UnixMilli(), UpdatedAt: now.Add(-time.Hour).UnixMilli(),
		Status: "queued", Label: "Queued", Step: "Queued",
	})
	require.NoError(t, err)

	active, err := service.ListActive(ctx)
	require.NoError(t, err)
	require.Len(t, active, 2)
	assert.Equal(t, []int64{queued.ID, inside.ID}, []int64{active[0].ID, active[1].ID})

	page, err := service.List(ctx, 0, 2)
	require.NoError(t, err)
	require.Len(t, page, 2)
	assert.Equal(t, []int64{queued.ID, inside.ID}, []int64{page[0].ID, page[1].ID})

	older, err := service.List(ctx, page[1].ID, 2)
	require.NoError(t, err)
	require.Len(t, older, 1)
	assert.Equal(t, outside.ID, older[0].ID)
}

func TestJobService_RecordsLifecycleLabelsAndPublishes(t *testing.T) {
	db, err := queries.Open(t.Context(), t.TempDir(), queries.DefaultOpenOptions())
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	ctx := t.Context()
	now := time.UnixMilli(1_000)
	st := stores.New(db, stores.Options{Now: func() time.Time { return now }})
	bus := newTestBus(t)
	ch := subscribeEvents[events.JobsUpdated](t, bus)
	service := newJobService(st.Jobs, bus, zerolog.Nop())

	id := service.Begin(ctx, "Review PR", "review", "pr-1")
	require.Positive(t, id)
	now = time.UnixMilli(2_000)
	service.Running(ctx, id, 44)
	assert.Equal(t, id, service.Resume(ctx, 44))
	now = time.UnixMilli(3_000)
	service.Done(ctx, id)
	assert.Zero(t, service.Resume(ctx, 44))
	for _, e := range requireEvents(t, ch, 3) {
		assert.Equal(t, id, e.JobID)
	}

	rows, err := service.List(ctx, 0, 10)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	job := rows[0]
	assert.Equal(t, jobs.JobStatusDone, job.Status)
	assert.Equal(t, "Completed", job.Step)
	assert.Equal(t, int64(1_000), job.CreatedAt)
	assert.Equal(t, int64(3_000), job.UpdatedAt)
	require.NotNil(t, job.CommandID)
	assert.Equal(t, int64(44), *job.CommandID)

	failedID := service.Begin(ctx, "Deploy", "deploy", "pr-2")
	service.Running(ctx, failedID, 45)
	service.Fail(ctx, failedID, "boom")
	requireEvents(t, ch, 3)
	rows, err = service.List(ctx, 0, 10)
	require.NoError(t, err)
	require.Len(t, rows, 2)
	assert.Equal(t, jobs.JobStatusFailed, rows[0].Status)
	assert.Equal(t, "Failed", rows[0].Step)
	assert.Equal(t, "boom", rows[0].Error)
	require.NotNil(t, rows[0].CommandID)
	assert.Equal(t, int64(45), *rows[0].CommandID)
}

func TestJobService_TrackRunsToCompletionUnlinkedToACommand(t *testing.T) {
	service, _ := newTestJobService(t)

	ran := make(chan struct{})
	id := service.Track(t.Context(), "Create session", "new-session", "sess-1", func(context.Context) error {
		close(ran)
		return nil
	})
	require.Positive(t, id)
	<-ran

	require.Eventually(t, func() bool {
		rows, err := service.List(t.Context(), 0, 10)
		return err == nil && len(rows) == 1 && rows[0].Status == jobs.JobStatusDone
	}, time.Second, 5*time.Millisecond)

	rows, err := service.List(t.Context(), 0, 10)
	require.NoError(t, err)
	assert.Nil(t, rows[0].CommandID, "a tracked background job has no output_command link")
	assert.Equal(t, "sess-1", rows[0].Target)
}

func TestJobService_TrackRecordsFailure(t *testing.T) {
	service, _ := newTestJobService(t)

	id := service.Track(t.Context(), "Create session", "new-session", "sess-2", func(context.Context) error {
		return errors.New("clone failed")
	})
	require.Positive(t, id)

	require.Eventually(t, func() bool {
		rows, err := service.List(t.Context(), 0, 10)
		return err == nil && len(rows) == 1 && rows[0].Status == jobs.JobStatusFailed && rows[0].Error == "clone failed"
	}, time.Second, 5*time.Millisecond)
}

// Begin runs before Track forks, while Done or Fail runs on the background
// goroutine after fn returns. Assert ordering, not only eventual delivery.
func TestJobService_TrackPublishesTerminalTransitionAfterFnReturns(t *testing.T) {
	service, ch := newTestJobService(t)

	release := make(chan struct{})
	fnReturned := make(chan struct{})
	id := service.Track(t.Context(), "Long task", "action", "target", func(context.Context) error {
		<-release
		close(fnReturned)
		return nil
	})
	require.Positive(t, id)

	// Begin (synchronous, before Track forks) and Running (the goroutine's
	// first act, before it calls fn) have both published by now.
	for _, e := range requireEvents(t, ch, 2) {
		assert.Equal(t, id, e.JobID)
	}
	// fn is still blocked on release: nothing more may have published yet.
	requireNoMoreEvents(t, ch)

	close(release)
	<-fnReturned

	final := requireEvents(t, ch, 1)
	assert.Equal(t, id, final[0].JobID, "the terminal transition publishes only once fn has returned")
}

func TestJobService_ListActiveUsesBackendClockWindow(t *testing.T) {
	db, err := queries.Open(t.Context(), t.TempDir(), queries.DefaultOpenOptions())
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	ctx := t.Context()
	now := time.UnixMilli(10_000)
	st := stores.New(db, stores.Options{Now: func() time.Time { return now }})
	service := newJobService(st.Jobs, newTestBus(t), zerolog.Nop())

	_, err = db.InsertJob(ctx, queries.InsertJobParams{
		CreatedAt: 1, UpdatedAt: now.Add(-jobs.DefaultLingerWindow).UnixMilli(), Status: "done", Label: "Boundary",
	})
	require.NoError(t, err)
	_, err = db.InsertJob(ctx, queries.InsertJobParams{
		CreatedAt: 2, UpdatedAt: now.Add(-jobs.DefaultLingerWindow - time.Millisecond).UnixMilli(), Status: "failed", Label: "Outside",
	})
	require.NoError(t, err)
	queued, err := db.InsertJob(ctx, queries.InsertJobParams{
		CreatedAt: 3, UpdatedAt: 1, Status: "queued", Label: "Queued",
	})
	require.NoError(t, err)

	active, err := service.ListActive(ctx)
	require.NoError(t, err)
	require.Len(t, active, 2)
	assert.Equal(t, queued.ID, active[0].ID)
	assert.Equal(t, "Boundary", active[1].Label)
}

// A failed write and a zero id both leave the bus quiet: JobsUpdated promises
// a row changed, and neither changed one.
func TestJobService_FailedWritesAndZeroTransitionsPublishNothing(t *testing.T) {
	db, err := queries.Open(t.Context(), t.TempDir(), queries.DefaultOpenOptions())
	require.NoError(t, err)
	require.NoError(t, db.Close())
	st := stores.New(db, stores.Options{})
	bus := newTestBus(t)
	ch := subscribeEvents[events.JobsUpdated](t, bus)
	service := newJobService(st.Jobs, bus, zerolog.Nop())

	id := service.Begin(t.Context(), "Review", "review", "pr-1")
	assert.Zero(t, id)
	assert.NotPanics(t, func() {
		service.Running(t.Context(), 0, 1)
		service.Done(t.Context(), 0)
		service.Fail(t.Context(), 0, "ignored")
	})
	service.Running(t.Context(), 1, 1)
	service.Done(t.Context(), 1)
	requireNoMoreEvents(t, ch)
}
