package stores

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"time"

	"github.com/rs/zerolog"

	"github.com/colonyops/hive/cmd/desktop/internal/app/data/queries"
)

// Category and Severity remain strings because ActivityService owns their
// validation and defaults.
type ActivityEventStore struct {
	q      *queries.DB
	now    func() time.Time
	logger zerolog.Logger
}

func NewActivityEventStore(q *queries.DB, opts Options) *ActivityEventStore {
	return &ActivityEventStore{q: q, now: opts.Now, logger: opts.Logger}
}

// Metadata is encoded here because its database representation is a
// persistence concern.
func (s *ActivityEventStore) Append(ctx context.Context, in ActivityEventCreate) (ActivityEvent, error) {
	encoded, err := encodeActivityMetadata(in.Metadata)
	if err != nil {
		return ActivityEvent{}, err
	}
	row, err := s.q.Ctx(ctx).AppendActivityEvent(ctx, queries.AppendActivityEventParams{
		CreatedAt: s.now().UnixMilli(),
		Category:  in.Category,
		Severity:  in.Severity,
		Title:     in.Title,
		Body:      in.Body,
		Source:    in.Source,
		Metadata:  encoded,
	})
	if err != nil {
		return ActivityEvent{}, wrap(fmt.Sprintf("appending activity event %q", in.Title), err)
	}
	return ActivityEvent{
		ID: row.ID, CreatedAt: row.CreatedAt, Category: row.Category, Severity: row.Severity,
		Title: row.Title, Body: row.Body, Source: row.Source, Metadata: in.Metadata,
	}, nil
}

// A metadata decode failure is logged and skipped instead of failing the page.
func (s *ActivityEventStore) List(ctx context.Context, before int64, limit int) ([]ActivityEvent, error) {
	if before <= 0 {
		before = math.MaxInt64
	}
	rows, err := s.q.Ctx(ctx).ListActivityEvents(ctx, queries.ListActivityEventsParams{
		ID:    before,
		Limit: int64(limit),
	})
	if err != nil {
		return nil, wrap("listing activity events", err)
	}
	out := make([]ActivityEvent, 0, len(rows))
	for _, row := range rows {
		event, err := activityEventFromRow(row)
		if err != nil {
			s.logger.Warn().Err(err).Int64("id", row.ID).Msg("skipping undecodable activity event")
			continue
		}
		out = append(out, event)
	}
	return out, nil
}

func activityEventFromRow(row queries.ActivityEvent) (ActivityEvent, error) {
	event := ActivityEvent{
		ID: row.ID, CreatedAt: row.CreatedAt, Category: row.Category, Severity: row.Severity,
		Title: row.Title, Body: row.Body, Source: row.Source,
	}
	if len(row.Metadata) > 0 {
		if err := json.Unmarshal(row.Metadata, &event.Metadata); err != nil {
			return ActivityEvent{}, fmt.Errorf("decoding activity metadata for event %d: %w", row.ID, err)
		}
	}
	return event, nil
}

func encodeActivityMetadata(meta map[string]string) ([]byte, error) {
	if len(meta) == 0 {
		return nil, nil
	}
	encoded, err := json.Marshal(meta)
	if err != nil {
		return nil, fmt.Errorf("encoding activity metadata: %w", err)
	}
	return encoded, nil
}
