package queries

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/colonyops/hive/cmd/desktop/internal/app/data/models"
)

// Append, AppendSnapshot, ReadFrom and ListReplaySourceSnapshots are thin
// fixture wrappers over the generated event_log queries, kept only for the
// tests in this package that exercise Open, Prune and WithinTx directly
// against *DB rather than through stores.EventLogStore. Production code
// never sees this file.

func (db *DB) Append(ctx context.Context, topic, key string, payload []byte) (int64, error) {
	return db.AppendEvent(ctx, AppendEventParams{
		Topic: topic, Key: key, Payload: payload, CreatedAt: time.Now().UnixMilli(),
	})
}

func (db *DB) AppendSnapshot(ctx context.Context, topic, sourceKind, sourceScope string, items []models.SnapshotItem) (int64, error) {
	payload, err := json.Marshal(items)
	if err != nil {
		return 0, fmt.Errorf("encoding source snapshot for topic %q: %w", topic, err)
	}
	return db.AppendEvent(ctx, AppendEventParams{
		Topic: topic, Payload: payload, CreatedAt: time.Now().UnixMilli(),
		Snapshot: 1, SourceKind: sourceKind, SourceScope: sourceScope,
	})
}

func (db *DB) ReadFrom(ctx context.Context, offset int64, limit int) ([]models.Msg, int64, error) {
	rows, err := db.ReadEventsFrom(ctx, ReadEventsFromParams{Offset: offset, Limit: int64(limit)})
	if err != nil {
		return nil, offset, fmt.Errorf("reading events from offset %d: %w", offset, err)
	}
	msgs := make([]models.Msg, 0, len(rows))
	nextOffset := offset
	for _, row := range rows {
		msg := models.Msg{
			ID: strconv.FormatInt(row.Offset, 10), Key: row.Key, Topic: row.Topic, Ts: row.CreatedAt,
			Payload: json.RawMessage(row.Payload), SourceKind: row.SourceKind, SourceScope: row.SourceScope,
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

func (db *DB) ListReplaySourceSnapshots(ctx context.Context, profileID string, throughOffset int64) ([]models.Msg, error) {
	prefix := "source:" + profileID + "/"
	rows, err := db.ListLatestSourceSnapshotsByTopicPrefix(ctx, ListLatestSourceSnapshotsByTopicPrefixParams{ThroughOffset: throughOffset, TopicPrefix: prefix})
	if err != nil {
		return nil, fmt.Errorf("listing replay source snapshots for %q: %w", profileID, err)
	}
	messages := make([]models.Msg, 0, len(rows))
	for _, row := range rows {
		var snapshot []models.SnapshotItem
		if err := json.Unmarshal(row.Payload, &snapshot); err != nil {
			return nil, fmt.Errorf("decoding replay source snapshot at offset %d: %w", row.Offset, err)
		}
		messages = append(messages, models.Msg{
			ID: strconv.FormatInt(row.Offset, 10), Topic: row.Topic, Ts: row.CreatedAt,
			Payload: json.RawMessage(row.Payload), Snapshot: snapshot, SourceKind: row.SourceKind, SourceScope: row.SourceScope,
		})
	}
	return messages, nil
}

func (db *DB) NodeKVSet(ctx context.Context, flowID, nodeID, key, value string, expiresAt int64) error {
	var exp sql.NullInt64
	if expiresAt > 0 {
		exp = sql.NullInt64{Int64: expiresAt, Valid: true}
	}
	return db.UpsertNodeKV(ctx, UpsertNodeKVParams{
		FlowID: flowID, NodeID: nodeID, Scope: "node", Key: key, Value: value, ExpiresAt: exp, UpdatedAt: time.Now().UnixMilli(),
	})
}
