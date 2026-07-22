package pipelinedb

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

const getEventLogTailOffset = `
SELECT CAST(COALESCE((SELECT seq FROM sqlite_sequence WHERE name = 'event_log'), 0) AS INTEGER)
`

// GetEventLogTailOffset returns the AUTOINCREMENT high-water mark. Unlike
// MAX(offset), it remains stable after event-log retention deletes rows.
func (q *Queries) GetEventLogTailOffset(ctx context.Context) (int64, error) {
	var tail int64
	err := q.db.QueryRowContext(ctx, getEventLogTailOffset).Scan(&tail)
	return tail, err
}

// EventLogTailOffset returns the current append-only log tail.
func (db *DB) EventLogTailOffset(ctx context.Context) (int64, error) {
	tail, err := db.queries.GetEventLogTailOffset(ctx)
	if err != nil {
		return 0, fmt.Errorf("getting event log tail: %w", err)
	}
	return tail, nil
}

// ListUnarchivedInboxItems returns exactly the Wails-safe items eligible for
// synthetic replay. Archived memberships are deliberately frozen and never
// returned.
func (db *DB) ListUnarchivedInboxItems(ctx context.Context, profileID string) ([]InboxItemView, error) {
	rows, err := db.queries.ListUnarchivedInboxItemsByProfile(ctx, profileID)
	if err != nil {
		return nil, fmt.Errorf("listing unarchived inbox items for %q: %w", profileID, err)
	}
	return inboxItemViews(rows), nil
}

// ListReplaySourceSnapshots returns each profile source's newest authoritative
// snapshot at or before throughOffset. Keeping the source topic on each message
// preserves provenance when a deployed graph recomputes feed memberships.
func (db *DB) ListReplaySourceSnapshots(ctx context.Context, profileID string, throughOffset int64) ([]Msg, error) {
	if throughOffset < 0 {
		return nil, fmt.Errorf("listing replay source snapshots for %q: negative offset", profileID)
	}
	prefix := "source:" + profileID + "/"
	rows, err := db.queries.ListLatestSourceSnapshotsByTopicPrefix(ctx, ListLatestSourceSnapshotsByTopicPrefixParams{ThroughOffset: throughOffset, TopicPrefix: prefix})
	if err != nil {
		return nil, fmt.Errorf("listing replay source snapshots for %q: %w", profileID, err)
	}

	messages := make([]Msg, 0, len(rows))
	for _, row := range rows {
		var snapshot []SnapshotItem
		if err := json.Unmarshal(row.Payload, &snapshot); err != nil {
			return nil, fmt.Errorf("decoding replay source snapshot at offset %d: %w", row.Offset, err)
		}
		messages = append(messages, Msg{
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

// ActivateReplay atomically installs a prepared synthetic replay: it advances
// the consumer past stale action-bound events, replaces unarchived feed
// memberships, and removes claims for deleted flow structure. A failed
// activation leaves the last-known-good runtime's offset and claims intact.
func (db *DB) ActivateReplay(ctx context.Context, profileID string, tail int64, claims []FeedMembershipClaim, feedIDs, sourceIDs []string) error {
	if tail < 0 {
		return fmt.Errorf("activating replay for %q: negative tail", profileID)
	}
	return db.WithTx(ctx, func(q *Queries) error {
		currentTail, err := q.GetEventLogTailOffset(ctx)
		if err != nil {
			return fmt.Errorf("reading event log tail: %w", err)
		}
		if tail > currentTail {
			return fmt.Errorf("activating replay for %q: supplied tail %d exceeds current event log tail %d", profileID, tail, currentTail)
		}

		if err := q.DeleteUnarchivedFeedMembershipClaimsByProfile(ctx, profileID); err != nil {
			return fmt.Errorf("clearing replayable memberships: %w", err)
		}
		for _, claim := range claims {
			if claim.ProfileID != "" && claim.ProfileID != profileID {
				return fmt.Errorf("activating replay: claim profile %q does not match %q", claim.ProfileID, profileID)
			}
			if _, err := q.GetUnarchivedInboxItemByID(ctx, GetUnarchivedInboxItemByIDParams{ID: claim.ItemID, ProfileID: profileID}); err != nil {
				return fmt.Errorf("activating replay: item %d is not an unarchived item in %q: %w", claim.ItemID, profileID, err)
			}
			if err := q.UpsertFeedMembershipClaim(ctx, UpsertFeedMembershipClaimParams{
				ProfileID: profileID,
				FeedID:    claim.FeedID,
				ItemID:    claim.ItemID,
				SourceID:  claim.SourceID,
			}); err != nil {
				return fmt.Errorf("activating replay membership %s/%d: %w", claim.FeedID, claim.ItemID, err)
			}
		}

		if len(feedIDs) == 0 {
			if err := q.DeleteFeedMembershipClaimsForFeedsAll(ctx, profileID); err != nil {
				return fmt.Errorf("clearing flow feeds: %w", err)
			}
		} else if err := q.DeleteFeedMembershipClaimsForFeeds(ctx, DeleteFeedMembershipClaimsForFeedsParams{ProfileID: profileID, FeedIds: feedIDs}); err != nil {
			return fmt.Errorf("removing obsolete feeds: %w", err)
		}
		if len(sourceIDs) == 0 {
			if err := q.DeleteFeedMembershipClaimsForRemovedSourcesAll(ctx, profileID); err != nil {
				return fmt.Errorf("clearing removed sources: %w", err)
			}
		} else if err := q.DeleteFeedMembershipClaimsForRemovedSources(ctx, DeleteFeedMembershipClaimsForRemovedSourcesParams{ProfileID: profileID, SourceIds: sourceIDs}); err != nil {
			return fmt.Errorf("removing obsolete sources: %w", err)
		}

		if err := q.CommitConsumerOffset(ctx, CommitConsumerOffsetParams{Consumer: profileID, Offset: tail}); err != nil {
			return fmt.Errorf("advancing replay consumer offset: %w", err)
		}
		return nil
	})
}

// PurgeProfile removes all durable state owned by a deleted flow. The topic
// prefix is escaped for LIKE so profile IDs cannot accidentally widen a purge.
func (db *DB) PurgeProfile(ctx context.Context, profileID string) error {
	prefix := "source:" + escapeLike(profileID) + "/%"
	return db.WithTx(ctx, func(q *Queries) error {
		if err := q.DeleteInboxItemsByProfile(ctx, profileID); err != nil {
			return fmt.Errorf("purging inbox items: %w", err)
		}
		if err := q.DeleteConsumerOffsetByConsumer(ctx, profileID); err != nil {
			return fmt.Errorf("purging consumer offset: %w", err)
		}
		if err := q.DeleteEventLogByTopicPrefix(ctx, prefix); err != nil {
			return fmt.Errorf("purging event log: %w", err)
		}
		if err := q.DeleteSourceHeadByTopicPrefix(ctx, prefix); err != nil {
			return fmt.Errorf("purging source head: %w", err)
		}
		return nil
	})
}

func escapeLike(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `%`, `\%`)
	s = strings.ReplaceAll(s, `_`, `\_`)
	return s
}
