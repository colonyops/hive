package main

import (
	"context"

	"github.com/colonyops/hive/internal/desktop/jobs"
)

// JobService exposes live action-run jobs to the desktop frontend. The
// titlebar reads active and briefly lingering jobs through ListActive, while
// List provides cursor-based history paging.
type JobService struct {
	store *jobs.Store
}

// NewJobService builds a JobService over store.
func NewJobService(store *jobs.Store) *JobService {
	return &JobService{store: store}
}

// List returns up to limit jobs with id < before, newest first.
func (s *JobService) List(before int64, limit int) ([]jobs.Job, error) {
	return s.store.List(context.Background(), before, limit)
}

// ListActive returns non-terminal jobs plus terminal jobs completed within the
// backend-owned lingering window.
func (s *JobService) ListActive() ([]jobs.Job, error) {
	return s.store.ListActive(context.Background(), jobs.DefaultLingerWindow)
}
