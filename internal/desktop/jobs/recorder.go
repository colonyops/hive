package jobs

import (
	"context"
	"log/slog"
	"time"

	"github.com/colonyops/hive/internal/desktop/pipeline/pipelinedb"
)

const (
	defaultListLimit = 200
	maxListLimit     = 1000
)

// Recorder is the fire-and-forget write side handed to the output worker. Job
// persistence failures are logged and never derail an action run.
type Recorder interface {
	// Begin creates a queued job and returns its id, or zero when persistence
	// fails. Subsequent transitions reference this id.
	Begin(ctx context.Context, label, actionID, target string) int64
	// Running marks a job running and links its output_command id.
	Running(ctx context.Context, id int64, commandID int64)
	// Resume returns the running job linked to commandID, or zero when none can
	// be restored. It keeps automatic retries on one lifecycle across restarts.
	Resume(ctx context.Context, commandID int64) int64
	// Done marks a job completed.
	Done(ctx context.Context, id int64)
	// Fail marks a job failed with a reason.
	Fail(ctx context.Context, id int64, reason string)
}

// Store persists and reads jobs in the shared desktop pipeline database.
type Store struct {
	db   *pipelinedb.DB
	now  func() time.Time
	emit func(id int64)
	log  *slog.Logger
}

// Options configures a Store. All fields are optional.
type Options struct {
	// Now supplies transition timestamps; defaults to time.Now.
	Now func() time.Time
	// Emit is called with the job id after each successful transition.
	Emit func(id int64)
	// Log receives persistence failures; defaults to slog.Default().
	Log *slog.Logger
}

// NewStore builds a Store over db.
func NewStore(db *pipelinedb.DB, opts Options) *Store {
	s := &Store{db: db, now: opts.Now, emit: opts.Emit, log: opts.Log}
	if s.now == nil {
		s.now = time.Now
	}
	if s.log == nil {
		s.log = slog.Default()
	}
	return s
}

// Begin creates a queued job. It returns zero after logging a persistence
// failure, preserving the fire-and-forget Recorder contract.
func (s *Store) Begin(ctx context.Context, label, actionID, target string) int64 {
	now := s.now().UnixMilli()
	rec, err := s.db.InsertJob(ctx, pipelinedb.JobRecord{
		CreatedAt: now,
		UpdatedAt: now,
		Status:    JobStatusQueued.String(),
		Label:     label,
		Step:      stepFor(JobStatusQueued),
		ActionID:  actionID,
		Target:    target,
	})
	if err != nil {
		s.log.Warn("beginning job failed", "label", label, "action_id", actionID, "error", err)
		return 0
	}
	s.emitUpdate(rec.ID)
	return rec.ID
}

// Running advances a job to running and links its output_command id. A zero job
// id is a safe no-op for callers whose Begin failed or whose recorder is off.
func (s *Store) Running(ctx context.Context, id int64, commandID int64) {
	if id == 0 {
		return
	}
	if _, err := s.db.SetJobRunning(ctx, id, s.now().UnixMilli(), stepFor(JobStatusRunning), commandID); err != nil {
		s.log.Warn("marking job running failed", "job_id", id, "command_id", commandID, "error", err)
		return
	}
	s.emitUpdate(id)
}

// Resume returns the running job linked to commandID. Lookup failures are
// logged and treated as no match so job tracking never derails command work.
func (s *Store) Resume(ctx context.Context, commandID int64) int64 {
	rec, found, err := s.db.FindRunningJobByCommandID(ctx, commandID)
	if err != nil {
		s.log.Warn("resuming job failed", "command_id", commandID, "error", err)
		return 0
	}
	if !found {
		return 0
	}
	return rec.ID
}

// Done advances a job to done. A zero job id is a safe no-op.
func (s *Store) Done(ctx context.Context, id int64) {
	s.setStatus(ctx, id, JobStatusDone, "")
}

// Fail advances a job to failed with reason. A zero job id is a safe no-op.
func (s *Store) Fail(ctx context.Context, id int64, reason string) {
	s.setStatus(ctx, id, JobStatusFailed, reason)
}

// List returns up to limit jobs with id < before, newest first.
func (s *Store) List(ctx context.Context, before int64, limit int) ([]Job, error) {
	if limit <= 0 || limit > maxListLimit {
		limit = defaultListLimit
	}
	recs, err := s.db.ListJobs(ctx, before, limit)
	if err != nil {
		return nil, err
	}
	return jobsFromRecords(recs), nil
}

// ListActive returns non-terminal jobs plus terminal jobs updated within
// window, newest first.
func (s *Store) ListActive(ctx context.Context, window time.Duration) ([]Job, error) {
	since := s.now().Add(-window).UnixMilli()
	recs, err := s.db.ListActiveJobs(ctx, since)
	if err != nil {
		return nil, err
	}
	return jobsFromRecords(recs), nil
}

func (s *Store) setStatus(ctx context.Context, id int64, status JobStatus, errText string) {
	if id == 0 {
		return
	}
	if _, err := s.db.SetJobStatus(ctx, id, s.now().UnixMilli(), status.String(), stepFor(status), errText); err != nil {
		s.log.Warn("updating job status failed", "job_id", id, "status", status, "error", err)
		return
	}
	s.emitUpdate(id)
}

func (s *Store) emitUpdate(id int64) {
	if s.emit != nil {
		s.emit(id)
	}
}

func jobsFromRecords(recs []pipelinedb.JobRecord) []Job {
	out := make([]Job, 0, len(recs))
	for _, rec := range recs {
		out = append(out, jobFromRecord(rec))
	}
	return out
}

func jobFromRecord(rec pipelinedb.JobRecord) Job {
	return Job{
		ID:        rec.ID,
		CreatedAt: rec.CreatedAt,
		UpdatedAt: rec.UpdatedAt,
		Status:    JobStatus(rec.Status),
		Label:     rec.Label,
		Step:      rec.Step,
		ActionID:  rec.ActionID,
		Target:    rec.Target,
		Error:     rec.Error,
		CommandID: rec.CommandID,
	}
}
