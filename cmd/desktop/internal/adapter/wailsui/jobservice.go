package wailsui

import (
	"context"

	"github.com/colonyops/hive/cmd/desktop/internal/app"
	"github.com/colonyops/hive/cmd/desktop/internal/app/jobs"
)

// JobService exposes live action-run jobs. The titlebar reads active and
// briefly lingering jobs through ListActive; List pages history.
type JobService struct {
	jobs *app.JobService
}

func NewJobService(j *app.JobService) *JobService { return &JobService{jobs: j} }

// List returns up to limit jobs with id < before, newest first.
func (s *JobService) List(ctx context.Context, before int64, limit int) ([]jobs.Job, error) {
	return s.jobs.List(ctx, before, limit)
}

// ListActive returns non-terminal jobs plus terminal jobs completed within
// the lingering window.
func (s *JobService) ListActive(ctx context.Context) ([]jobs.Job, error) {
	return s.jobs.ListActive(ctx)
}

// Cancel stops a running action command. Its job update arrives through jobs:updated.
func (s *JobService) Cancel(ctx context.Context, commandID int64) error {
	return s.jobs.CancelCommand(ctx, commandID)
}
