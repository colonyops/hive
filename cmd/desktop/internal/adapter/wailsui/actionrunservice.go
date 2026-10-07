package wailsui

import (
	"context"

	"github.com/colonyops/hive/cmd/desktop/internal/app"
)

// ActionRunService exposes the action run history and each run's log to the
// run viewer.
type ActionRunService struct {
	runs *app.ActionRunsService
}

func NewActionRunService(r *app.ActionRunsService) *ActionRunService {
	return &ActionRunService{runs: r}
}

// List returns up to limit runs with id < before, newest first. Zero before
// starts at the newest.
func (s *ActionRunService) List(ctx context.Context, before int64, limit int) ([]app.ActionRunSummary, error) {
	return s.runs.List(ctx, before, limit)
}

func (s *ActionRunService) Get(ctx context.Context, commandID int64) (app.ActionRunSummary, error) {
	return s.runs.Get(ctx, commandID)
}

// Log returns up to limit lines after afterID, oldest first.
func (s *ActionRunService) Log(ctx context.Context, commandID, afterID int64, limit int) (app.ActionRunLogPage, error) {
	return s.runs.Log(ctx, commandID, afterID, limit)
}
