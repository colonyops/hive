package stores

import (
	"context"

	"github.com/colonyops/hive/cmd/desktop/internal/app/data/models"
	"github.com/colonyops/hive/cmd/desktop/internal/app/data/queries"
)

type FeedClaimStore struct {
	q *queries.DB
}

func NewFeedClaimStore(q *queries.DB, _ Options) *FeedClaimStore {
	return &FeedClaimStore{q: q}
}

// Existing claims are a no-op.
func (s *FeedClaimStore) Upsert(ctx context.Context, claim models.FeedClaim) error {
	return wrap("claiming feed membership", s.q.Ctx(ctx).UpsertFeedMembershipClaim(ctx, queries.UpsertFeedMembershipClaimParams{
		ProfileID: claim.ProfileID, FeedID: claim.FeedID, ItemID: claim.ItemID, SourceID: claim.SourceID,
	}))
}

func (s *FeedClaimStore) DeleteNotInSnapshot(ctx context.Context, feedID, sourceID string, itemIDs []int64) error {
	return wrap("reconciling feed snapshot", s.q.Ctx(ctx).DeleteFeedMembershipClaimsNotInSnapshot(ctx, queries.DeleteFeedMembershipClaimsNotInSnapshotParams{
		FeedID: feedID, SourceID: sourceID, ItemIds: itemIDs,
	}))
}

// This separate path handles an empty snapshot, which the NOT IN query cannot
// express.
func (s *FeedClaimStore) DeleteForSourceAll(ctx context.Context, feedID, sourceID string) error {
	return wrap("clearing empty feed snapshot", s.q.Ctx(ctx).DeleteFeedMembershipClaimsForSourceAll(ctx, queries.DeleteFeedMembershipClaimsForSourceAllParams{
		FeedID: feedID, SourceID: sourceID,
	}))
}

// An empty keepFeedIDs clears all profile feeds.
func (s *FeedClaimStore) DeleteForFeeds(ctx context.Context, profileID string, keepFeedIDs []string) error {
	q := s.q.Ctx(ctx)
	if len(keepFeedIDs) == 0 {
		return wrap("clearing flow feeds", q.DeleteFeedMembershipClaimsForFeedsAll(ctx, profileID))
	}
	return wrap("removing obsolete feeds", q.DeleteFeedMembershipClaimsForFeeds(ctx, queries.DeleteFeedMembershipClaimsForFeedsParams{
		ProfileID: profileID, FeedIds: keepFeedIDs,
	}))
}

// An empty keepSourceIDs clears all profile sources.
func (s *FeedClaimStore) DeleteForRemovedSources(ctx context.Context, profileID string, keepSourceIDs []string) error {
	q := s.q.Ctx(ctx)
	if len(keepSourceIDs) == 0 {
		return wrap("clearing removed sources", q.DeleteFeedMembershipClaimsForRemovedSourcesAll(ctx, profileID))
	}
	return wrap("removing obsolete sources", q.DeleteFeedMembershipClaimsForRemovedSources(ctx, queries.DeleteFeedMembershipClaimsForRemovedSourcesParams{
		ProfileID: profileID, SourceIds: keepSourceIDs,
	}))
}

func (s *FeedClaimStore) DeleteUnarchivedByProfile(ctx context.Context, profileID string) error {
	return wrap("clearing replayable memberships", s.q.Ctx(ctx).DeleteUnarchivedFeedMembershipClaimsByProfile(ctx, profileID))
}

// Archived-item claims are included.
func (s *FeedClaimStore) DeleteByProfile(ctx context.Context, profileID string) error {
	return wrap("clearing profile memberships", s.q.Ctx(ctx).DeleteFeedMembershipClaimsForFeedsAll(ctx, profileID))
}
