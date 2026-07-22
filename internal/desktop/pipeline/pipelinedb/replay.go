package pipelinedb

import (
	"context"
	"encoding/json"
	"fmt"
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

// FastForwardConsumer moves a consumer to the current supplied tail. The
// underlying upsert is monotonic, so a concurrent normal commit can never be
// regressed. A caller may not move beyond the durable log tail: accepting a
// future offset would permanently hide subsequently appended events.
func (db *DB) FastForwardConsumer(ctx context.Context, consumer string, tail int64) error {
	if tail < 0 {
		return fmt.Errorf("fast-forwarding consumer %q: negative tail", consumer)
	}
	return db.WithTx(ctx, func(q *Queries) error {
		currentTail, err := q.GetEventLogTailOffset(ctx)
		if err != nil {
			return fmt.Errorf("reading event log tail: %w", err)
		}
		if tail > currentTail {
			return fmt.Errorf("fast-forwarding consumer %q: supplied tail %d exceeds current event log tail %d", consumer, tail, currentTail)
		}
		if err := q.CommitConsumerOffset(ctx, CommitConsumerOffsetParams{Consumer: consumer, Offset: tail}); err != nil {
			return err
		}
		return nil
	})
}

// ListUnarchivedInboxItems returns exactly the Wails-safe items eligible for
// synthetic replay. Archived memberships are deliberately frozen and never
// returned.
func (db *DB) ListUnarchivedInboxItems(ctx context.Context, profileID string) ([]InboxItemView, error) {
	rows, err := db.queries.ListUnarchivedInboxItemsByProfile(ctx, profileID)
	if err != nil {
		return nil, fmt.Errorf("listing unarchived inbox items for %q: %w", profileID, err)
	}
	views := make([]InboxItemView, 0, len(rows))
	for _, row := range rows {
		view := InboxItemView{
			ID: row.ID, ProfileID: row.ProfileID, SourceKind: row.SourceKind,
			SourceScope: row.SourceScope, ExternalID: row.ExternalID, Title: row.Title,
			URL: row.Url, Payload: json.RawMessage(row.Payload), Revision: row.Revision,
			Unread: row.Unread != 0, Lifecycle: row.Lifecycle, FirstSeenAt: row.FirstSeenAt,
			LastEventAt: row.LastEventAt,
		}
		if row.ArchivedAt.Valid {
			archivedAt := row.ArchivedAt.Int64
			view.ArchivedAt = &archivedAt
		}
		if row.ArchivedActor.Valid {
			view.ArchivedActor = row.ArchivedActor.String
		}
		if row.ArchivedReason.Valid {
			view.ArchivedReason = row.ArchivedReason.String
		}
		if row.SourceState.Valid {
			view.SourceState = row.SourceState.String
		}
		views = append(views, view)
	}
	return views, nil
}

// RecomputeMemberships is the claims-only synthetic replay write path. It has
// no consumer offset, output command, or node run capability, so callers
// cannot accidentally turn a startup/deploy replay into an action invocation.
func (db *DB) RecomputeMemberships(ctx context.Context, profileID string, claims []FeedMembershipClaim) error {
	return db.WithTx(ctx, func(q *Queries) error {
		// A synthetic replay is complete for this profile's current unarchived
		// universe. Clearing that mutable portion first makes narrowed filters
		// hide items while archived claims remain frozen.
		if err := q.DeleteUnarchivedFeedMembershipClaimsByProfile(ctx, profileID); err != nil {
			return fmt.Errorf("clearing replayable memberships: %w", err)
		}
		for _, claim := range claims {
			if claim.ProfileID != "" && claim.ProfileID != profileID {
				return fmt.Errorf("recomputing memberships: claim profile %q does not match %q", claim.ProfileID, profileID)
			}
			if _, err := q.GetUnarchivedInboxItemByID(ctx, GetUnarchivedInboxItemByIDParams{ID: claim.ItemID, ProfileID: profileID}); err != nil {
				return fmt.Errorf("recomputing memberships: item %d is not an unarchived item in %q: %w", claim.ItemID, profileID, err)
			}
			if err := q.UpsertFeedMembershipClaim(ctx, UpsertFeedMembershipClaimParams{
				ProfileID: profileID,
				FeedID:    claim.FeedID,
				ItemID:    claim.ItemID,
				SourceID:  claim.SourceID,
			}); err != nil {
				return fmt.Errorf("recomputing membership %s/%d: %w", claim.FeedID, claim.ItemID, err)
			}
		}
		return nil
	})
}

// ReconcileFlowMembershipStructure deletes all claims for removed feeds. For
// removed or disabled sources, it preserves archived claims and deletes only
// unarchived claims.
func (db *DB) ReconcileFlowMembershipStructure(ctx context.Context, profileID string, feedIDs, sourceIDs []string) error {
	return db.WithTx(ctx, func(q *Queries) error {
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
