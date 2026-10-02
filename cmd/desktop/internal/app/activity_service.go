package app

import (
	"context"

	"github.com/rs/zerolog"

	"github.com/colonyops/hive/cmd/desktop/internal/app/activity"
	"github.com/colonyops/hive/cmd/desktop/internal/app/data/stores"
	"github.com/colonyops/hive/cmd/desktop/internal/app/events"
)

const (
	activityDefaultListLimit = 200
	activityMaxListLimit     = 1000
)

// ActivityService validates and persists activity events. Its Recorder is the
// metered view of the store-backed record, so every backend emitter that holds
// the port is counted through one path. The frontend appends through Append,
// which returns the row and its error, and is not counted. Validation stays
// here because data stores must not import activity's enums.
type ActivityService struct {
	activity.Recorder
	store  *stores.ActivityEventStore
	events *events.Bus
	log    zerolog.Logger
}

func newActivityService(store *stores.ActivityEventStore, bus *events.Bus, logger zerolog.Logger) *ActivityService {
	s := &ActivityService{store: store, events: bus, log: logger}
	s.Recorder = activity.Metered(recorderFunc(s.record))
	return s
}

// recorderFunc adapts a plain function to activity.Recorder, the same shape
// http.HandlerFunc gives http.Handler.
type recorderFunc func(ctx context.Context, e activity.Event)

func (f recorderFunc) Record(ctx context.Context, e activity.Event) { f(ctx, e) }

// List returns up to limit events with id < before, newest first. Pass
// before <= 0 to start from the most recent event.
func (s *ActivityService) List(ctx context.Context, before int64, limit int) ([]activity.Event, error) {
	if limit <= 0 || limit > activityMaxListLimit {
		limit = activityDefaultListLimit
	}
	rows, err := s.store.List(ctx, before, limit)
	if err != nil {
		return nil, Wrap(err, KindInternal, "listing activity events")
	}
	out := make([]activity.Event, 0, len(rows))
	for _, row := range rows {
		out = append(out, activityEventFromStore(row))
	}
	return out, nil
}

// Append returns validation and persistence errors. Backend emitters that
// cannot fail use Record.
func (s *ActivityService) Append(ctx context.Context, e activity.Event) (activity.Event, error) {
	if e.Title == "" {
		return activity.Event{}, Errorf(KindInvalid, "activity event requires a title")
	}
	if e.Category == "" {
		e.Category = activity.CategorySystem
	}
	if e.Severity == "" {
		e.Severity = activity.SeverityInfo
	}
	if !e.Category.IsValid() {
		return activity.Event{}, Errorf(KindInvalid, "invalid activity category %q", e.Category)
	}
	if !e.Severity.IsValid() {
		return activity.Event{}, Errorf(KindInvalid, "invalid activity severity %q", e.Severity)
	}

	stored, err := s.store.Append(ctx, stores.ActivityEventCreate{
		Category: e.Category.String(),
		Severity: e.Severity.String(),
		Title:    e.Title,
		Body:     e.Body,
		Source:   e.Source,
		Metadata: e.Metadata,
	})
	if err != nil {
		return activity.Event{}, Wrap(err, KindInternal, "recording an activity event")
	}
	s.events.Publish(ctx, events.ActivityAppended{ID: stored.ID})
	return activityEventFromStore(stored), nil
}

// record is the activity.Recorder under the meter: an emit site should never
// fail or block because the audit log couldn't be written, so a persistence
// failure is logged and swallowed rather than returned.
func (s *ActivityService) record(ctx context.Context, e activity.Event) {
	if _, err := s.Append(ctx, e); err != nil {
		s.log.Warn().Ctx(ctx).Err(err).Str("title", e.Title).Msg("recording activity event failed")
	}
}

func activityEventFromStore(row stores.ActivityEvent) activity.Event {
	return activity.Event{
		ID:        row.ID,
		CreatedAt: row.CreatedAt,
		Category:  activity.Category(row.Category),
		Severity:  activity.Severity(row.Severity),
		Title:     row.Title,
		Body:      row.Body,
		Source:    row.Source,
		Metadata:  row.Metadata,
	}
}
