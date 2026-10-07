package app

import (
	"context"

	"github.com/rs/zerolog"

	"github.com/colonyops/hive/cmd/desktop/internal/app/data/stores"
	"github.com/colonyops/hive/cmd/desktop/internal/app/events"
	"github.com/colonyops/hive/cmd/desktop/internal/app/jobs"
)

const (
	jobDefaultListLimit = 200
	jobMaxListLimit     = 1000
)

// JobService persists jobs and implements jobs.Recorder for background work.
type JobService struct {
	store         *stores.JobStore
	events        *events.Bus
	log           zerolog.Logger
	cancelCommand func(int64) bool
}

func newJobService(store *stores.JobStore, bus *events.Bus, logger zerolog.Logger) *JobService {
	return &JobService{store: store, events: bus, log: logger}
}

func (s *JobService) setCommandCanceller(cancel func(int64) bool) {
	s.cancelCommand = cancel
}

func (s *JobService) CancelCommand(_ context.Context, commandID int64) error {
	if s.cancelCommand == nil || !s.cancelCommand(commandID) {
		return Errorf(KindConflict, "action run %d is not running", commandID)
	}
	return nil
}

// List defaults limits outside 1..1000 to 200.
func (s *JobService) List(ctx context.Context, before int64, limit int) ([]jobs.Job, error) {
	if limit <= 0 || limit > jobMaxListLimit {
		limit = jobDefaultListLimit
	}
	rows, err := s.store.List(ctx, before, limit)
	if err != nil {
		return nil, Wrap(err, KindInternal, "listing jobs")
	}
	return jobsFromStore(rows), nil
}

// ListActive returns non-terminal jobs plus terminal jobs completed within
// the lingering window, so a just-finished run stays visible briefly.
func (s *JobService) ListActive(ctx context.Context) ([]jobs.Job, error) {
	rows, err := s.store.ListActiveWithin(ctx, jobs.DefaultLingerWindow)
	if err != nil {
		return nil, Wrap(err, KindInternal, "listing active jobs")
	}
	return jobsFromStore(rows), nil
}

// Persistence failures are logged and return zero.
func (s *JobService) Begin(ctx context.Context, label, actionID, target string) int64 {
	job, err := s.store.Insert(ctx, stores.JobCreate{
		Status: jobs.JobStatusQueued.String(), Label: label, Step: jobs.StepFor(jobs.JobStatusQueued),
		ActionID: actionID, Target: target,
	})
	if err != nil {
		s.log.Warn().Err(err).Str("label", label).Str("action_id", actionID).Msg("beginning job failed")
		return 0
	}
	s.events.Publish(ctx, events.JobsUpdated{JobID: job.ID})
	return job.ID
}

// Running links the job to its output command. A zero job ID is a no-op.
func (s *JobService) Running(ctx context.Context, id int64, commandID int64) {
	if id == 0 {
		return
	}
	if _, err := s.store.SetRunning(ctx, id, jobs.StepFor(jobs.JobStatusRunning), commandID); err != nil {
		s.log.Warn().Err(err).Int64("job_id", id).Int64("command_id", commandID).Msg("marking job running failed")
		return
	}
	s.events.Publish(ctx, events.JobsUpdated{JobID: id})
}

// Missing jobs and lookup failures return zero; failures are logged.
func (s *JobService) Resume(ctx context.Context, commandID int64) int64 {
	job, found, err := s.store.FindRunningByCommand(ctx, commandID)
	if err != nil {
		s.log.Warn().Err(err).Int64("command_id", commandID).Msg("resuming job failed")
		return 0
	}
	if !found {
		return 0
	}
	return job.ID
}

// A zero ID is a no-op.
func (s *JobService) Done(ctx context.Context, id int64) {
	s.setStatus(ctx, id, jobs.JobStatusDone, "")
}

// A zero ID is a no-op.
func (s *JobService) Fail(ctx context.Context, id int64, reason string) {
	s.setStatus(ctx, id, jobs.JobStatusFailed, reason)
}

// A zero ID is a no-op.
func (s *JobService) Cancel(ctx context.Context, id int64, reason string) {
	s.setStatus(ctx, id, jobs.JobStatusCancelled, reason)
}

// Track starts fn asynchronously on a context detached from caller
// cancellation. Persistence failures do not stop fn.
//
// Do not call Track inside Stores.WithinTx: context.WithoutCancel preserves
// transaction values, which must not cross into Track's goroutine.
func (s *JobService) Track(ctx context.Context, label, actionID, target string, fn func(context.Context) error) int64 {
	id := s.Begin(ctx, label, actionID, target)
	bg := context.WithoutCancel(ctx)
	go func() {
		s.setStatus(bg, id, jobs.JobStatusRunning, "")
		if err := fn(bg); err != nil {
			s.Fail(bg, id, err.Error())
			return
		}
		s.Done(bg, id)
	}()
	return id
}

func (s *JobService) setStatus(ctx context.Context, id int64, status jobs.JobStatus, errText string) {
	if id == 0 {
		return
	}
	if _, err := s.store.SetStatus(ctx, id, status.String(), jobs.StepFor(status), errText); err != nil {
		s.log.Warn().Err(err).Int64("job_id", id).Str("status", status.String()).Msg("updating job status failed")
		return
	}
	s.events.Publish(ctx, events.JobsUpdated{JobID: id})
}

func jobsFromStore(rows []stores.Job) []jobs.Job {
	out := make([]jobs.Job, 0, len(rows))
	for _, row := range rows {
		out = append(out, jobFromStore(row))
	}
	return out
}

func jobFromStore(row stores.Job) jobs.Job {
	return jobs.Job{
		ID: row.ID, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt, Status: jobs.JobStatus(row.Status),
		Label: row.Label, Step: row.Step, ActionID: row.ActionID, Target: row.Target,
		Error: row.Error, CommandID: row.CommandID,
	}
}
