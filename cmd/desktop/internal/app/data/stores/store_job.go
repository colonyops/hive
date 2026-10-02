package stores

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/colonyops/hive/cmd/desktop/internal/app/data/queries"
)

// Status and Step remain strings because JobService owns their typed mapping.
type JobStore struct {
	q      *queries.DB
	now    func() time.Time
	mapper MapFunc[queries.Job, Job]
}

func NewJobStore(q *queries.DB, opts Options) *JobStore {
	return &JobStore{q: q, now: opts.Now, mapper: mapJobFromDB}
}

func (s *JobStore) Insert(ctx context.Context, in JobCreate) (Job, error) {
	now := s.now().UnixMilli()
	row, err := s.q.Ctx(ctx).InsertJob(ctx, queries.InsertJobParams{
		CreatedAt: now,
		UpdatedAt: now,
		Status:    in.Status,
		Label:     in.Label,
		Step:      in.Step,
		ActionID:  in.ActionID,
		Target:    in.Target,
		Error:     in.Error,
	})
	return s.mapper.Err(row, wrap(fmt.Sprintf("inserting job %q", in.Label), err))
}

// SetRunning is the only job update that writes CommandID. A zero commandID
// leaves it NULL: a background job tracked outside the output worker has no
// command to link.
func (s *JobStore) SetRunning(ctx context.Context, id int64, step string, commandID int64) (Job, error) {
	row, err := s.q.Ctx(ctx).SetJobRunning(ctx, queries.SetJobRunningParams{
		UpdatedAt: s.now().UnixMilli(),
		Status:    "running",
		Step:      step,
		CommandID: sql.NullInt64{Int64: commandID, Valid: commandID != 0},
		ID:        id,
	})
	return s.mapper.Err(row, wrap(fmt.Sprintf("setting job %d running", id), err))
}

func (s *JobStore) SetStatus(ctx context.Context, id int64, status, step, errText string) (Job, error) {
	row, err := s.q.Ctx(ctx).SetJobStatus(ctx, queries.SetJobStatusParams{
		UpdatedAt: s.now().UnixMilli(),
		Status:    status,
		Step:      step,
		Error:     errText,
		ID:        id,
	})
	return s.mapper.Err(row, wrap(fmt.Sprintf("setting job %d status to %q", id, status), err))
}

// A missing running job returns found=false without an error.
func (s *JobStore) FindRunningByCommand(ctx context.Context, commandID int64) (Job, bool, error) {
	row, err := s.q.Ctx(ctx).FindRunningJobByCommandID(ctx, sql.NullInt64{Int64: commandID, Valid: true})
	if errors.Is(err, sql.ErrNoRows) {
		return Job{}, false, nil
	}
	if err != nil {
		return Job{}, false, wrap(fmt.Sprintf("finding running job for command %d", commandID), err)
	}
	return s.mapper(row), true, nil
}

// before <= 0 starts at the most recent job.
func (s *JobStore) List(ctx context.Context, before int64, limit int) ([]Job, error) {
	if before <= 0 {
		before = math.MaxInt64
	}
	rows, err := s.q.Ctx(ctx).ListJobs(ctx, queries.ListJobsParams{
		ID:    before,
		Limit: int64(limit),
	})
	return s.mapper.SliceErr(rows, wrap("listing jobs", err))
}

func (s *JobStore) ListActive(ctx context.Context, since int64) ([]Job, error) {
	rows, err := s.q.Ctx(ctx).ListActiveJobs(ctx, since)
	return s.mapper.SliceErr(rows, wrap("listing active jobs", err))
}

func (s *JobStore) ListActiveWithin(ctx context.Context, window time.Duration) ([]Job, error) {
	return s.ListActive(ctx, s.now().Add(-window).UnixMilli())
}
