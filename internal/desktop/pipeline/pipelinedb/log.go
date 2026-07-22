package pipelinedb

import (
	"bytes"
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
// It mirrors the design's { id, key, topic, ts, payload } contract,
// with Snapshot populated only for an authoritative full-source snapshot,
// mapped onto the event_log schema (see migrations/0001_pipeline.up.sql):
//   - ID is derived from the row's "offset" (stable, unique per append; there
//     is no separate id column).
//   - Ts is the row's created_at (unix milliseconds).
//   - Snapshot is nil for ordinary item events and contains the full current
//     source item set for successful poll snapshots.
type Msg struct {
	ID            string
	Key           string
	Topic         string
	Ts            int64
	Payload       json.RawMessage
	Snapshot      []SnapshotItem `json:"Snapshot,omitempty"`
	SourceKind    string
	SourceScope   string
	OccurrenceKey string `json:"OccurrenceKey,omitempty"`
}

// SnapshotItem is one current source item carried by a successful source
// snapshot. A snapshot event is distinct from ordinary changed-item events:
// it is emitted on every successful poll, including when the source is empty.
type SnapshotItem struct {
	Key     string          `json:"key"`
	Payload json.RawMessage `json:"payload"`
}

// Append inserts a new event_log row under topic, keyed by key, and returns
// its offset. created_at is stamped as the current unix millisecond time.
func (db *DB) Append(ctx context.Context, topic, key string, payload []byte) (int64, error) {
	offset, err := db.queries.AppendEvent(ctx, AppendEventParams{
		Topic:      topic,
		Key:        key,
		Payload:    payload,
		CreatedAt:  time.Now().UnixMilli(),
		Snapshot:   0,
		SourceKind: "", SourceScope: "", OccurrenceKey: sql.NullString{},
	})
	if err != nil {
		return 0, fmt.Errorf("appending event to topic %q: %w", topic, err)
	}
	return offset, nil
}

// AppendIfChanged appends a source event only when its non-empty topic/key
// identity has no stored payload or has a different payload. The event append
// and source-head update happen in one transaction, so a failure leaves
// neither a new event nor a head that would incorrectly suppress a retry.
// Empty-key messages have no stable identity and always append.
func (db *DB) AppendIfChanged(ctx context.Context, topic, key string, payload []byte) (int64, bool, error) {
	if key == "" {
		offset, err := db.Append(ctx, topic, key, payload)
		return offset, err == nil, err
	}

	var (
		offset   int64
		appended bool
	)
	err := db.WithTx(ctx, func(q *Queries) error {
		previous, err := q.GetSourceHeadPayload(ctx, GetSourceHeadPayloadParams{
			Topic: topic,
			Key:   key,
		})
		if err == nil && bytes.Equal(previous, payload) {
			return nil
		}
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("reading source head for topic %q, key %q: %w", topic, key, err)
		}

		offset, err = q.AppendEvent(ctx, AppendEventParams{
			Topic:      topic,
			Key:        key,
			Payload:    payload,
			CreatedAt:  time.Now().UnixMilli(),
			Snapshot:   0,
			SourceKind: "", SourceScope: "", OccurrenceKey: sql.NullString{},
		})
		if err != nil {
			return fmt.Errorf("appending event to topic %q: %w", topic, err)
		}
		if err := q.UpsertSourceHead(ctx, UpsertSourceHeadParams{
			Topic:   topic,
			Key:     key,
			Payload: payload,
		}); err != nil {
			return fmt.Errorf("updating source head for topic %q, key %q: %w", topic, key, err)
		}
		appended = true
		return nil
	})
	if err != nil {
		return 0, false, fmt.Errorf("conditionally appending event to topic %q: %w", topic, err)
	}
	return offset, appended, nil
}

// AppendSnapshot appends a successful source poll's complete current item
// set. Unlike item events, snapshots are deliberately not deduplicated: each
// one is an authoritative reconciliation point, including an empty set.
func (db *DB) AppendSnapshot(ctx context.Context, topic string, items []SnapshotItem) (int64, error) {
	payload, err := json.Marshal(items)
	if err != nil {
		return 0, fmt.Errorf("encoding source snapshot for topic %q: %w", topic, err)
	}
	offset, err := db.queries.AppendEvent(ctx, AppendEventParams{
		Topic:      topic,
		Key:        "",
		Payload:    payload,
		CreatedAt:  time.Now().UnixMilli(),
		Snapshot:   1,
		SourceKind: "", SourceScope: "", OccurrenceKey: sql.NullString{},
	})
	if err != nil {
		return 0, fmt.Errorf("appending source snapshot for topic %q: %w", topic, err)
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
		msg := Msg{
			ID:            strconv.FormatInt(row.Offset, 10),
			Key:           row.Key,
			Topic:         row.Topic,
			Ts:            row.CreatedAt,
			Payload:       json.RawMessage(row.Payload),
			SourceKind:    row.SourceKind,
			SourceScope:   row.SourceScope,
			OccurrenceKey: row.OccurrenceKey.String,
		}
		if row.Snapshot != 0 {
			if err := json.Unmarshal(row.Payload, &msg.Snapshot); err != nil {
				return nil, offset, fmt.Errorf("decoding source snapshot at offset %d: %w", row.Offset, err)
			}
		}
		msgs = append(msgs, msg)
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
