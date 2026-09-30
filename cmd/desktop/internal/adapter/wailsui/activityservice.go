package wailsui

import (
	"context"

	"github.com/colonyops/hive/cmd/desktop/internal/app"
	"github.com/colonyops/hive/cmd/desktop/internal/app/activity"
)

// ActivityService exposes the desktop's activity log to the frontend: the
// Activity view reads pages through List and records its own events through
// Record. Backend subsystems do not go through here — they hold the recorder.
type ActivityService struct {
	activity *app.ActivityService
}

func NewActivityService(a *app.ActivityService) *ActivityService {
	return &ActivityService{activity: a}
}

// List returns up to limit activity events with id < before, newest first.
// The frontend passes 0 for the first page.
func (s *ActivityService) List(ctx context.Context, before int64, limit int) ([]activity.Event, error) {
	return s.activity.List(ctx, before, limit)
}

// Record appends a frontend-originated event and returns the stored row.
func (s *ActivityService) Record(ctx context.Context, input activity.RecordInput) (activity.Event, error) {
	event, err := input.Event()
	if err != nil {
		return activity.Event{}, err
	}
	return s.activity.Append(ctx, event)
}
