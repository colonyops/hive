package activity

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/colonyops/hive/internal/desktop/pipeline/pipelinedb"
)

const (
	defaultListLimit = 200
	maxListLimit     = 1000
)

// Recorder is the write side handed to backend subsystems. It is deliberately
// fire-and-forget: an emit site should never fail or block because the audit
// log couldn't be written, so errors are logged, not returned. The concrete
// *Store satisfies it.
type Recorder interface {
	Record(ctx context.Context, e Event)
}

// Store persists and reads activity events, backed by the shared pipeline
// SQLite database. It is the single implementation behind both the Recorder
// (backend emit sites) and the ActivityService (frontend reads/writes).
type Store struct {
	db   *pipelinedb.DB
	now  func() time.Time
	emit func(id int64)
	log  *slog.Logger
}

// Options configures a Store. All fields are optional.
type Options struct {
	// Now supplies the append timestamp; defaults to time.Now. Tests inject a
	// fixed clock for deterministic ordering.
	Now func() time.Time
	// Emit is invoked with the new event's id after each successful append, so
	// the frontend can re-read and detect what it hasn't seen. nil is valid
	// (e.g. before the Wails app is running, or in tests).
	Emit func(id int64)
	// Log receives append/decode failures; defaults to slog.Default().
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

// Append validates, persists, and returns the stored event (with its assigned
// id and timestamp), firing the emit callback on success. It is the error
// -returning path used by the frontend RPC; backend sites use Record.
func (s *Store) Append(ctx context.Context, e Event) (Event, error) {
	if e.Title == "" {
		return Event{}, fmt.Errorf("activity event requires a title")
	}
	if e.Category == "" {
		e.Category = CategorySystem
	}
	if e.Severity == "" {
		e.Severity = SeverityInfo
	}
	if !e.Category.IsValid() {
		return Event{}, fmt.Errorf("invalid activity category %q", e.Category)
	}
	if !e.Severity.IsValid() {
		return Event{}, fmt.Errorf("invalid activity severity %q", e.Severity)
	}

	var meta []byte
	if len(e.Metadata) > 0 {
		encoded, err := json.Marshal(e.Metadata)
		if err != nil {
			return Event{}, fmt.Errorf("encoding activity metadata: %w", err)
		}
		meta = encoded
	}

	rec, err := s.db.AppendActivityEvent(ctx, pipelinedb.ActivityRecord{
		CreatedAt: s.now().UnixMilli(),
		Category:  e.Category.String(),
		Severity:  e.Severity.String(),
		Title:     e.Title,
		Body:      e.Body,
		Source:    e.Source,
		Metadata:  meta,
	})
	if err != nil {
		return Event{}, err
	}

	stored, err := eventFromRecord(rec)
	if err != nil {
		return Event{}, err
	}
	if s.emit != nil {
		s.emit(stored.ID)
	}
	return stored, nil
}

// Record persists e and swallows errors after logging them. This is the
// Recorder path for background emit sites, which must not be derailed by a
// failed audit write.
func (s *Store) Record(ctx context.Context, e Event) {
	if _, err := s.Append(ctx, e); err != nil {
		s.log.Warn("recording activity event failed", "title", e.Title, "error", err)
	}
}

// List returns up to limit events with id < before, newest first. Pass
// before <= 0 to start from the most recent event. A row whose metadata fails
// to decode is skipped (and logged) rather than failing the whole page.
func (s *Store) List(ctx context.Context, before int64, limit int) ([]Event, error) {
	if limit <= 0 || limit > maxListLimit {
		limit = defaultListLimit
	}
	recs, err := s.db.ListActivityEvents(ctx, before, limit)
	if err != nil {
		return nil, err
	}
	out := make([]Event, 0, len(recs))
	for _, rec := range recs {
		ev, err := eventFromRecord(rec)
		if err != nil {
			s.log.Warn("skipping undecodable activity event", "id", rec.ID, "error", err)
			continue
		}
		out = append(out, ev)
	}
	return out, nil
}

func eventFromRecord(rec pipelinedb.ActivityRecord) (Event, error) {
	ev := Event{
		ID:        rec.ID,
		CreatedAt: rec.CreatedAt,
		Category:  Category(rec.Category),
		Severity:  Severity(rec.Severity),
		Title:     rec.Title,
		Body:      rec.Body,
		Source:    rec.Source,
	}
	if len(rec.Metadata) > 0 {
		if err := json.Unmarshal(rec.Metadata, &ev.Metadata); err != nil {
			return Event{}, fmt.Errorf("decoding activity metadata for event %d: %w", rec.ID, err)
		}
	}
	return ev, nil
}
