package stores

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/rs/zerolog"
	"go.opentelemetry.io/otel/attribute"

	"github.com/colonyops/hive/cmd/desktop/internal/app/data/models"
	"github.com/colonyops/hive/cmd/desktop/internal/app/data/queries"
	"github.com/colonyops/hive/cmd/desktop/internal/app/observe"
)

// EventLogStore owns event_log and consumer_offset; commit and replay use
// sibling stores in one transaction.
type EventLogStore struct {
	q        *queries.DB
	now      func() time.Time
	logger   zerolog.Logger
	items    *InboxItemStore
	claims   *FeedClaimStore
	kv       *NodeKVStore
	runs     *NodeRunStore
	commands *OutputCommandStore
}

func NewEventLogStore(q *queries.DB, opts Options, items *InboxItemStore, claims *FeedClaimStore, kv *NodeKVStore, runs *NodeRunStore, commands *OutputCommandStore) *EventLogStore {
	return &EventLogStore{
		q: q, now: opts.Now, logger: opts.Logger,
		items: items, claims: claims, kv: kv, runs: runs, commands: commands,
	}
}

func (s *EventLogStore) Append(ctx context.Context, topic, key string, payload []byte) (int64, error) {
	offset, err := s.q.Ctx(ctx).AppendEvent(ctx, queries.AppendEventParams{
		Topic:      topic,
		Key:        key,
		Payload:    payload,
		CreatedAt:  s.now().UnixMilli(),
		Snapshot:   0,
		SourceKind: "", SourceScope: "", OccurrenceKey: sql.NullString{},
	})
	return offset, wrap(fmt.Sprintf("appending event to topic %q", topic), err)
}

// AppendSnapshot appends a successful source poll's complete current item
// set. Unlike item events, snapshots are deliberately not deduplicated: each
// one is an authoritative reconciliation point, including an empty set.
func (s *EventLogStore) AppendSnapshot(ctx context.Context, topic, sourceKind, sourceScope string, items []models.SnapshotItem) (int64, error) {
	payload, err := json.Marshal(items)
	if err != nil {
		return 0, fmt.Errorf("encoding source snapshot for topic %q: %w", topic, err)
	}
	offset, err := s.q.Ctx(ctx).AppendEvent(ctx, queries.AppendEventParams{
		Topic:      topic,
		Key:        "",
		Payload:    payload,
		CreatedAt:  s.now().UnixMilli(),
		Snapshot:   1,
		SourceKind: sourceKind, SourceScope: sourceScope, OccurrenceKey: sql.NullString{},
	})
	return offset, wrap(fmt.Sprintf("appending source snapshot for topic %q", topic), err)
}

// nextOffset is the last returned offset, or the input offset for an empty
// page.
func (s *EventLogStore) ReadFrom(ctx context.Context, offset int64, limit int) ([]models.Msg, int64, error) {
	rows, err := s.q.Ctx(ctx).ReadEventsFrom(ctx, queries.ReadEventsFromParams{
		Offset: offset,
		Limit:  int64(limit),
	})
	if err != nil {
		return nil, offset, fmt.Errorf("reading events from offset %d: %w", offset, err)
	}

	msgs := make([]models.Msg, 0, len(rows))
	nextOffset := offset
	for _, row := range rows {
		msg := models.Msg{
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

// Reads resume from the consumer's last successful commit, including after
// runtime restarts.
func (s *EventLogStore) ReadForConsumer(ctx context.Context, consumer string, limit int) ([]models.Msg, error) {
	offset, err := s.ConsumerOffset(ctx, consumer)
	if err != nil {
		return nil, err
	}
	msgs, _, err := s.ReadFrom(ctx, offset, limit)
	return msgs, err
}

// An unknown consumer has offset 0.
func (s *EventLogStore) ConsumerOffset(ctx context.Context, consumer string) (int64, error) {
	row, err := s.q.Ctx(ctx).GetConsumerOffset(ctx, consumer)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("reading committed offset for consumer %q: %w", consumer, err)
	}
	return row.Offset, nil
}

// Uses the AUTOINCREMENT high-water mark so retention deletes cannot lower
// the tail.
func (s *EventLogStore) TailOffset(ctx context.Context) (int64, error) {
	tail, err := s.q.Ctx(ctx).GetEventLogTailOffset(ctx)
	return tail, wrap("getting event log tail", err)
}

// Returned messages retain source topics so replayed graph evaluation
// preserves provenance.
func (s *EventLogStore) ListLatestSnapshots(ctx context.Context, profileID string, throughOffset int64) ([]models.Msg, error) {
	if throughOffset < 0 {
		return nil, fmt.Errorf("listing replay source snapshots for %q: negative offset", profileID)
	}
	prefix := "source:" + profileID + "/"
	rows, err := s.q.Ctx(ctx).ListLatestSourceSnapshotsByTopicPrefix(ctx, queries.ListLatestSourceSnapshotsByTopicPrefixParams{ThroughOffset: throughOffset, TopicPrefix: prefix})
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
			ID:          strconv.FormatInt(row.Offset, 10),
			Topic:       row.Topic,
			Ts:          row.CreatedAt,
			Payload:     json.RawMessage(row.Payload),
			Snapshot:    snapshot,
			SourceKind:  row.SourceKind,
			SourceScope: row.SourceScope,
		})
	}
	return messages, nil
}

// topicPrefix is matched literally.
func (s *EventLogStore) DeleteByTopicPrefix(ctx context.Context, topicPrefix string) error {
	return wrap("deleting event log by topic prefix", s.q.Ctx(ctx).DeleteEventLogByTopicPrefix(ctx, likePrefix(topicPrefix)))
}

func (s *EventLogStore) DeleteConsumerOffset(ctx context.Context, consumer string) error {
	return wrap("deleting consumer offset", s.q.Ctx(ctx).DeleteConsumerOffsetByConsumer(ctx, consumer))
}

// The caller timestamp keeps event_log, inbox_item, and inbox_event rows from
// one ingest aligned.
func (s *EventLogStore) AppendObservation(ctx context.Context, topic, key string, payload []byte, sourceKind, sourceScope, occurrenceKey string, now int64) (int64, error) {
	offset, err := s.q.Ctx(ctx).AppendEvent(ctx, queries.AppendEventParams{
		Topic: topic, Key: key, Payload: payload, CreatedAt: now,
		Snapshot: 0, SourceKind: sourceKind, SourceScope: sourceScope, OccurrenceKey: null(occurrenceKey),
	})
	if err != nil {
		return 0, wrap(fmt.Sprintf("appending event to topic %q", topic), err)
	}
	return offset, nil
}

// A missing classifier occurrence key falls back to the inserted event's
// offset.
func (s *EventLogStore) BackfillOccurrenceKey(ctx context.Context, offset int64, occurrenceKey string) error {
	return wrap("backfilling occurrence key", s.q.Ctx(ctx).UpdateEventOccurrenceKey(ctx, queries.UpdateEventOccurrenceKeyParams{
		OccurrenceKey: null(occurrenceKey), Offset: offset,
	}))
}

// Commit applies a batch and advances its consumer offset atomically. A batch
// at or below the committed offset is a no-op; output commands also
// deduplicate across distinct batches.
func (s *EventLogStore) Commit(ctx context.Context, b models.CommitBatch) error {
	ctx, span := observe.StartConditionalSpan(ctx, tracer, "db.CommitBatch")
	defer span.End()
	span.SetAttributes(attribute.Int("db.batch.outputs", len(b.Outputs)))

	s.q.DebugPauseCommit(ctx)
	if b.UpToOffset < 0 {
		return fmt.Errorf("commit offset must not be negative: %d", b.UpToOffset)
	}

	return s.q.WithinTx(ctx, func(ctx context.Context, _ *queries.DB) error {
		current, err := s.ConsumerOffset(ctx, b.Consumer)
		if err != nil {
			return fmt.Errorf("reading committed offset for consumer %q: %w", b.Consumer, err)
		}
		if b.UpToOffset <= current {
			return nil
		}

		now := s.now().UnixMilli()

		for _, out := range b.Outputs {
			switch out.Sink.Kind {
			case models.SinkKindFeed:
				itemID, err := s.items.ResolveScopedID(ctx, b.Consumer, out.SourceKind, out.SourceScope, out.Key)
				if errors.Is(err, sql.ErrNoRows) {
					// Feed outputs may use synthesized keys that never passed through
					// ingest. Keyless outputs are skipped so the consumer can advance.
					if out.Key == "" {
						s.logger.Warn().
							Ctx(ctx).
							Str("consumer", b.Consumer).
							Str("sourceKind", out.SourceKind).
							Str("sourceScope", out.SourceScope).
							Msg("commit: feed output has no key; skipping so the offset can advance")
						continue
					}
					item, err := s.items.CreateSynthesized(ctx, InboxItemSynthesize{
						ProfileID: b.Consumer, SourceKind: out.SourceKind, SourceScope: out.SourceScope,
						ExternalID: out.Key, Payload: out.Payload, Now: now,
					})
					if err != nil {
						return fmt.Errorf("minting inbox item %s/%s/%s: %w", out.SourceKind, out.SourceScope, out.Key, err)
					}
					itemID = item.ID
				} else if err != nil {
					return fmt.Errorf("resolving inbox item %s/%s/%s: %w", out.SourceKind, out.SourceScope, out.Key, err)
				}
				if err := s.claims.Upsert(ctx, models.FeedClaim{
					ProfileID: b.Consumer, FeedID: out.Sink.TargetID, ItemID: itemID, SourceID: out.SourceTopic,
				}); err != nil {
					return fmt.Errorf("claiming feed membership %s/%s: %w", out.Sink.TargetID, out.Key, err)
				}
			case models.SinkKindAction:
				// The dedup key is the occurrence key, so the row cannot be
				// traced back to its item by key alone.
				ref := models.ItemRef{ProfileID: b.Consumer, SourceKind: out.SourceKind, SourceScope: out.SourceScope, ExternalID: out.Key}
				if err := s.commands.Enqueue(ctx, out.Sink.TargetID, out.OccurrenceKey, []byte(out.Payload), now, ref); err != nil {
					return fmt.Errorf("enqueuing output_command %s/%s: %w", out.Sink.TargetID, out.OccurrenceKey, err)
				}
			case models.SinkKindNotify:
				payload, err := json.Marshal(models.NotifyCommand{
					ProfileID:   b.Consumer,
					ExternalID:  out.Key,
					SourceKind:  out.SourceKind,
					SourceScope: out.SourceScope,
					Item:        out.Payload,
				})
				if err != nil {
					return fmt.Errorf("encoding notify command %s/%s: %w", out.Sink.TargetID, out.Key, err)
				}
				key := notifyDedupKey(out)
				if err := s.commands.Enqueue(ctx, models.NotifyActionID(out.Sink.TargetID), key, payload, now, models.ItemRef{}); err != nil {
					return fmt.Errorf("enqueuing notify command %s/%s: %w", out.Sink.TargetID, key, err)
				}
			default:
				return fmt.Errorf("commit batch: unknown sink kind %q", out.Sink.Kind)
			}
		}

		for _, snapshot := range b.FeedSnapshots {
			itemIDs := make([]int64, 0)
			for _, out := range b.Outputs {
				if out.Sink.Kind != models.SinkKindFeed || out.Sink.TargetID != snapshot.FeedID || out.SourceTopic != snapshot.SourceTopic || out.SnapshotID != snapshot.SnapshotID {
					continue
				}
				itemID, err := s.items.ResolveScopedID(ctx, b.Consumer, out.SourceKind, out.SourceScope, out.Key)
				if errors.Is(err, sql.ErrNoRows) {
					// A keyless output cannot claim membership.
					continue
				}
				if err != nil {
					return fmt.Errorf("resolving snapshot inbox item %s: %w", out.Key, err)
				}
				itemIDs = append(itemIDs, itemID)
			}
			if len(itemIDs) == 0 {
				if err := s.claims.DeleteForSourceAll(ctx, snapshot.FeedID, snapshot.SourceTopic); err != nil {
					return fmt.Errorf("clearing empty feed snapshot: %w", err)
				}
			} else if err := s.claims.DeleteNotInSnapshot(ctx, snapshot.FeedID, snapshot.SourceTopic, itemIDs); err != nil {
				return fmt.Errorf("reconciling feed snapshot: %w", err)
			}
		}

		for _, m := range b.KVMutations {
			if m.Delete {
				if err := s.kv.Delete(ctx, b.Consumer, m.NodeID, m.Key); err != nil {
					return fmt.Errorf("deleting node kv %s/%s: %w", m.NodeID, m.Key, err)
				}
				continue
			}
			if err := s.kv.Set(ctx, b.Consumer, m.NodeID, m.Key, m.Value, m.ExpiresAt); err != nil {
				return fmt.Errorf("writing node kv %s/%s: %w", m.NodeID, m.Key, err)
			}
		}

		for _, nr := range b.NodeRuns {
			if err := s.runs.Insert(ctx, nr, now); err != nil {
				return fmt.Errorf("inserting node_run for %s/%s: %w", nr.FlowID, nr.NodeID, err)
			}
		}

		if err := s.q.Ctx(ctx).CommitConsumerOffset(ctx, queries.CommitConsumerOffsetParams{
			Consumer: b.Consumer,
			Offset:   b.UpToOffset,
		}); err != nil {
			return fmt.Errorf("advancing consumer offset for %q: %w", b.Consumer, err)
		}

		return nil
	})
}

// Prefer the classifier occurrence key so unchanged polls deduplicate. Events
// without one use item identity plus a payload digest; an empty fallback would
// silence all later notifications for the action.
func notifyDedupKey(out models.Output) string {
	if out.OccurrenceKey != "" {
		return out.OccurrenceKey
	}
	sum := sha256.Sum256(out.Payload)
	return out.Key + "@" + hex.EncodeToString(sum[:8])
}

// Activation updates offsets, memberships, and node KV atomically; failure
// preserves the last-known-good runtime state.
func (s *EventLogStore) ActivateReplay(ctx context.Context, profileID string, tail int64, claims []models.FeedClaim, feedIDs, sourceIDs, kvNodeIDs []string) error {
	if tail < 0 {
		return fmt.Errorf("activating replay for %q: negative tail", profileID)
	}
	return s.q.WithinTx(ctx, func(ctx context.Context, _ *queries.DB) error {
		currentTail, err := s.TailOffset(ctx)
		if err != nil {
			return fmt.Errorf("reading event log tail: %w", err)
		}
		if tail > currentTail {
			return fmt.Errorf("activating replay for %q: supplied tail %d exceeds current event log tail %d", profileID, tail, currentTail)
		}

		if err := s.claims.DeleteUnarchivedByProfile(ctx, profileID); err != nil {
			return fmt.Errorf("clearing replayable memberships: %w", err)
		}
		for _, claim := range claims {
			if claim.ProfileID != "" && claim.ProfileID != profileID {
				return fmt.Errorf("activating replay: claim profile %q does not match %q", claim.ProfileID, profileID)
			}
			claim.ProfileID = profileID
			if _, err := s.items.GetUnarchivedByID(ctx, claim.ItemID, profileID); err != nil {
				return fmt.Errorf("activating replay: item %d is not an unarchived item in %q: %w", claim.ItemID, profileID, err)
			}
			if err := s.claims.Upsert(ctx, claim); err != nil {
				return fmt.Errorf("activating replay membership %s/%d: %w", claim.FeedID, claim.ItemID, err)
			}
		}

		if err := s.claims.DeleteForFeeds(ctx, profileID, feedIDs); err != nil {
			return err
		}
		if err := s.claims.DeleteForRemovedSources(ctx, profileID, sourceIDs); err != nil {
			return err
		}

		if err := s.q.Ctx(ctx).CommitConsumerOffset(ctx, queries.CommitConsumerOffsetParams{Consumer: profileID, Offset: tail}); err != nil {
			return fmt.Errorf("advancing replay consumer offset: %w", err)
		}

		if len(kvNodeIDs) == 0 {
			if err := s.kv.DeleteByFlow(ctx, profileID); err != nil {
				return fmt.Errorf("clearing flow node kv: %w", err)
			}
		} else if err := s.kv.DeleteForFlowExceptNodes(ctx, profileID, kvNodeIDs); err != nil {
			return fmt.Errorf("removing obsolete node kv: %w", err)
		}
		return nil
	})
}
