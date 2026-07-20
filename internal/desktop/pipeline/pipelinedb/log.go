package pipelinedb

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"
)

// Msg is the pipeline's generic log record, appended by sources and consumed
// by the frontend graph runtime.
//
// It mirrors the design's { id, key, topic, ts, payload, meta } contract,
// mapped onto the event_log schema (see migrations/0001_pipeline.up.sql):
//   - ID is derived from the row's "offset" (stable, unique per append; there
//     is no separate id column).
//   - Ts is the row's created_at (unix nanoseconds).
//   - Meta is not persisted by this phase: event_log has no meta column.
//     Later source phases may populate Meta on the in-memory Msg before it
//     reaches the frontend, but Append here takes no meta and ReadFrom always
//     returns a nil Meta. The schema is not extended speculatively.
type Msg struct {
	ID      string
	Key     string
	Topic   string
	Ts      int64
	Payload json.RawMessage
	Meta    map[string]any
}

// Append inserts a new event_log row under topic, keyed by key, and returns
// its offset. created_at is stamped as the current unix nanosecond time.
func (db *DB) Append(ctx context.Context, topic, key string, payload []byte) (int64, error) {
	offset, err := db.queries.AppendEvent(ctx, AppendEventParams{
		Topic:     topic,
		Key:       key,
		Payload:   payload,
		CreatedAt: time.Now().UnixNano(),
	})
	if err != nil {
		return 0, fmt.Errorf("appending event to topic %q: %w", topic, err)
	}
	return offset, nil
}

// ReadFrom returns up to limit event_log rows with offset > offset, ordered
// ascending, along with the offset of the last row returned (nextOffset).
// If no rows are found, nextOffset is the offset argument unchanged, so
// callers can always resume with ReadFrom(ctx, nextOffset, limit).
func (db *DB) ReadFrom(ctx context.Context, offset int64, limit int) ([]Msg, int64, error) {
	rows, err := db.queries.ReadEventsFrom(ctx, ReadEventsFromParams{
		Offset: offset,
		Limit:  int64(limit),
	})
	if err != nil {
		return nil, offset, fmt.Errorf("reading events from offset %d: %w", offset, err)
	}

	msgs := make([]Msg, 0, len(rows))
	nextOffset := offset
	for _, row := range rows {
		msgs = append(msgs, Msg{
			ID:      strconv.FormatInt(row.Offset, 10),
			Key:     row.Key,
			Topic:   row.Topic,
			Ts:      row.CreatedAt,
			Payload: json.RawMessage(row.Payload),
		})
		nextOffset = row.Offset
	}

	return msgs, nextOffset, nil
}

// ReadForConsumer returns up to limit events after consumer's persisted
// checkpoint. Consumers therefore resume from their last successful commit,
// including after the frontend runtime restarts.
func (db *DB) ReadForConsumer(ctx context.Context, consumer string, limit int) ([]Msg, error) {
	offset, err := db.ConsumerOffset(ctx, consumer)
	if err != nil {
		return nil, err
	}
	msgs, _, err := db.ReadFrom(ctx, offset, limit)
	return msgs, err
}

// ConsumerOffset returns the last offset committed by consumer, or 0 if the
// consumer has never committed.
func (db *DB) ConsumerOffset(ctx context.Context, consumer string) (int64, error) {
	row, err := db.queries.GetConsumerOffset(ctx, consumer)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("reading committed offset for consumer %q: %w", consumer, err)
	}
	return row.Offset, nil
}

// Commit records offset as the last-read position for consumer. It is
// monotonic: a commit at or below the currently stored offset is a no-op
// (enforced in SQL, see CommitConsumerOffset), so out-of-order or replayed
// commits never move a consumer's checkpoint backwards.
func (db *DB) Commit(ctx context.Context, consumer string, offset int64) error {
	if err := db.queries.CommitConsumerOffset(ctx, CommitConsumerOffsetParams{
		Consumer: consumer,
		Offset:   offset,
	}); err != nil {
		return fmt.Errorf("committing offset %d for consumer %q: %w", offset, consumer, err)
	}
	return nil
}

// Compact reclaims event_log space in three independent passes, using the
// retention configured at Open (see OpenOptions.Compact). Each pass is safe
// to run at any time: consumers resume from their own committed offset, so
// compaction never needs coordination with in-flight readers.
//
//  1. Key-compaction: for every non-empty key, keep only the highest-offset
//     row (the current value for that key) — the log-compaction semantic the
//     table is named for.
//  2. Age retention: drop rows older than db.compact.MaxAge (skipped if zero).
//  3. Count retention: if still over db.compact.MaxRows, drop the oldest
//     rows until the cap is met (skipped if zero).
func (db *DB) Compact(ctx context.Context) error {
	if err := db.queries.CompactEventLogByKey(ctx); err != nil {
		return fmt.Errorf("compacting event_log by key: %w", err)
	}

	if db.compact.MaxAge > 0 {
		cutoff := time.Now().Add(-db.compact.MaxAge).UnixNano()
		if err := db.queries.DeleteEventLogOlderThan(ctx, cutoff); err != nil {
			return fmt.Errorf("applying event_log age retention: %w", err)
		}
	}

	if db.compact.MaxRows > 0 {
		count, err := db.queries.CountEventLog(ctx)
		if err != nil {
			return fmt.Errorf("counting event_log rows: %w", err)
		}
		if excess := count - int64(db.compact.MaxRows); excess > 0 {
			if err := db.queries.DeleteOldestEventLog(ctx, excess); err != nil {
				return fmt.Errorf("applying event_log count retention: %w", err)
			}
		}
	}

	return nil
}
