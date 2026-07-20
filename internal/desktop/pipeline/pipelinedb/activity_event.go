package pipelinedb

import (
	"context"
	"fmt"
	"math"
)

// ActivityRecord is the storage shape of one activity_event row. category and
// severity stay plain strings at this layer — the activity package owns the
// typed enums and converts at its boundary, keeping pipelinedb a leaf store
// (the same split as FeedItemView surfacing unread as a bool only above the
// row layer). Metadata is raw JSON, nil when the row carries none.
type ActivityRecord struct {
	ID        int64  `json:"id"`
	CreatedAt int64  `json:"createdAt"`
	Category  string `json:"category"`
	Severity  string `json:"severity"`
	Title     string `json:"title"`
	Body      string `json:"body"`
	Source    string `json:"source"`
	Metadata  []byte `json:"metadata,omitempty"`
}

// AppendActivityEvent persists one activity event and returns the stored row
// with its assigned id and durable created_at.
func (db *DB) AppendActivityEvent(ctx context.Context, rec ActivityRecord) (ActivityRecord, error) {
	row, err := db.queries.AppendActivityEvent(ctx, AppendActivityEventParams{
		CreatedAt: rec.CreatedAt,
		Category:  rec.Category,
		Severity:  rec.Severity,
		Title:     rec.Title,
		Body:      rec.Body,
		Source:    rec.Source,
		Metadata:  rec.Metadata,
	})
	if err != nil {
		return ActivityRecord{}, fmt.Errorf("appending activity event %q: %w", rec.Title, err)
	}
	return activityRecordFromRow(row), nil
}

// ListActivityEvents returns up to limit events with id < before, newest first.
// Pass before <= 0 to start from the most recent event.
func (db *DB) ListActivityEvents(ctx context.Context, before int64, limit int) ([]ActivityRecord, error) {
	if before <= 0 {
		before = math.MaxInt64
	}
	rows, err := db.queries.ListActivityEvents(ctx, ListActivityEventsParams{
		ID:    before,
		Limit: int64(limit),
	})
	if err != nil {
		return nil, fmt.Errorf("listing activity events: %w", err)
	}
	out := make([]ActivityRecord, 0, len(rows))
	for _, row := range rows {
		out = append(out, activityRecordFromRow(row))
	}
	return out, nil
}

// activityRecordFromRow adapts a generated row to the domain record. The two
// are field-identical today, so a direct conversion suffices; if a future
// column makes them diverge (e.g. a nullable column mapped to sql.NullString),
// this stops compiling and becomes an explicit mapping.
func activityRecordFromRow(row ActivityEvent) ActivityRecord {
	return ActivityRecord(row)
}
