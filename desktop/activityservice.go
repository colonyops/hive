package main

import (
	"context"

	"github.com/colonyops/hive/internal/desktop/activity"
)

// ActivityService is the Wails service exposing the desktop's activity log to
// the frontend: the Activity view reads pages of events through List, and the
// UI records its own events through Record. Backend subsystems don't go through
// here — they hold the activity.Recorder directly (see main.go's wiring).
//
// Like FlowsService and PipelineService, this is thin wire glue; the real logic
// lives in internal/desktop/activity.
type ActivityService struct {
	store *activity.Store
}

func NewActivityService(store *activity.Store) *ActivityService {
	return &ActivityService{store: store}
}

// List returns up to limit activity events with id < before, newest first.
// Pass before <= 0 (and the frontend passes 0 for the first page) to start from
// the most recent event; page older history by passing the smallest id seen.
func (s *ActivityService) List(before int64, limit int) ([]activity.Event, error) {
	return s.store.List(context.Background(), before, limit)
}

// Record appends a frontend-originated event and returns the stored row. This
// is the UI's path for surfacing things only it knows about (a failed save, a
// deleted profile); it emits the same activity:appended wake-up as backend
// recording, so every open Activity view refreshes.
func (s *ActivityService) Record(input activity.RecordInput) (activity.Event, error) {
	event, err := input.Event()
	if err != nil {
		return activity.Event{}, err
	}
	return s.store.Append(context.Background(), event)
}
