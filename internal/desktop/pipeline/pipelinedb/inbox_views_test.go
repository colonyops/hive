package pipelinedb

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func itemIDs(items []InboxItemView) []int64 {
	ids := make([]int64, 0, len(items))
	for _, item := range items {
		ids = append(ids, item.ID)
	}
	return ids
}

func TestFeedViewsTriageAndCounts(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	first, err := db.Queries().InsertInboxItem(ctx, InsertInboxItemParams{ProfileID: "p", SourceKind: "github", ExternalID: "one", Title: "one", Payload: []byte(`{}`), Unread: 1, Lifecycle: "active", FirstSeenAt: 1, LastEventAt: 2})
	require.NoError(t, err)
	second, err := db.Queries().InsertInboxItem(ctx, InsertInboxItemParams{ProfileID: "p", SourceKind: "github", ExternalID: "two", Title: "two", Payload: []byte(`{}`), Lifecycle: "active", FirstSeenAt: 1, LastEventAt: 2})
	require.NoError(t, err)
	unmatched, err := db.Queries().InsertInboxItem(ctx, InsertInboxItemParams{ProfileID: "p", SourceKind: "github", ExternalID: "outside", Title: "outside", Payload: []byte(`{}`), Unread: 1, Lifecycle: "active", FirstSeenAt: 1, LastEventAt: 3})
	require.NoError(t, err)
	require.NoError(t, db.Queries().UpsertFeedMembershipClaim(ctx, UpsertFeedMembershipClaimParams{ProfileID: "p", FeedID: "feed-a", ItemID: first.ID, SourceID: "source-a"}))
	// A feed is a set of inbox items, not source claims: two sources may claim
	// the same item into one feed without duplicating its row or its unread count.
	require.NoError(t, db.Queries().UpsertFeedMembershipClaim(ctx, UpsertFeedMembershipClaimParams{ProfileID: "p", FeedID: "feed-a", ItemID: first.ID, SourceID: "source-b"}))
	require.NoError(t, db.Queries().UpsertFeedMembershipClaim(ctx, UpsertFeedMembershipClaimParams{ProfileID: "p", FeedID: "feed-b", ItemID: second.ID, SourceID: "source-a"}))

	byFeed, err := db.ListInboxItemsByFeed(ctx, "p", "feed-a", 10)
	require.NoError(t, err)
	assert.Equal(t, []int64{first.ID}, itemIDs(byFeed))
	byOtherFeed, err := db.ListInboxItemsByFeed(ctx, "p", "feed-b", 10)
	require.NoError(t, err)
	assert.Equal(t, []int64{second.ID}, itemIDs(byOtherFeed))
	counts, err := db.FeedCounts(ctx, "p")
	require.NoError(t, err)
	assert.Equal(t, []FeedInboxCount{{FeedID: "feed-a", Total: 1, Unread: 1}, {FeedID: "feed-b", Total: 1, Unread: 0}}, counts)

	// Items that never reached a feed terminal are visible only in Trash.
	trash, err := db.ListInboxItemsTrash(ctx, "p", 10)
	require.NoError(t, err)
	assert.Equal(t, []int64{unmatched.ID}, itemIDs(trash), "unrouted items land in trash, not feeds")

	updated, err := db.SetInboxItemUnread(ctx, first.ID, first.Revision, false)
	require.NoError(t, err)
	assert.Equal(t, first.Revision+1, updated.Revision)
	_, err = db.SetInboxItemUnread(ctx, first.ID, first.Revision, true)
	require.ErrorIs(t, err, ErrStaleInboxItem)

	// Archiving demotes the item into the feed's archived section: it leaves
	// the active list but stays reachable in the same feed.
	archived, err := db.ToggleInboxItemArchived(ctx, first.ID, updated.Revision, 99)
	require.NoError(t, err)
	assert.Equal(t, updated.Revision+1, archived.Revision)
	_, err = db.ToggleInboxItemArchived(ctx, first.ID, updated.Revision, 99)
	require.ErrorIs(t, err, ErrStaleInboxItem)
	byFeed, err = db.ListInboxItemsByFeed(ctx, "p", "feed-a", 10)
	require.NoError(t, err)
	assert.Empty(t, byFeed)
	archivedByFeed, err := db.ListArchivedInboxItemsByFeed(ctx, "p", "feed-a", 10)
	require.NoError(t, err)
	assert.Equal(t, []int64{first.ID}, itemIDs(archivedByFeed))
	counts, err = db.FeedCounts(ctx, "p")
	require.NoError(t, err)
	assert.Equal(t, []FeedInboxCount{{FeedID: "feed-a", Total: 0, Unread: 0, Archived: 1}, {FeedID: "feed-b", Total: 1, Unread: 0}}, counts)

	// Archived ordering: newest archive first, id breaks ties.
	third, err := db.Queries().InsertInboxItem(ctx, InsertInboxItemParams{ProfileID: "p", SourceKind: "github", ExternalID: "three", Title: "three", Payload: []byte(`{}`), Lifecycle: "active", FirstSeenAt: 1, LastEventAt: 1})
	require.NoError(t, err)
	fourth, err := db.Queries().InsertInboxItem(ctx, InsertInboxItemParams{ProfileID: "p", SourceKind: "github", ExternalID: "four", Title: "four", Payload: []byte(`{}`), Lifecycle: "active", FirstSeenAt: 1, LastEventAt: 1})
	require.NoError(t, err)
	require.NoError(t, db.Queries().UpsertFeedMembershipClaim(ctx, UpsertFeedMembershipClaimParams{ProfileID: "p", FeedID: "feed-a", ItemID: third.ID, SourceID: "source-a"}))
	require.NoError(t, db.Queries().UpsertFeedMembershipClaim(ctx, UpsertFeedMembershipClaimParams{ProfileID: "p", FeedID: "feed-a", ItemID: fourth.ID, SourceID: "source-a"}))
	_, err = db.ToggleInboxItemArchived(ctx, third.ID, third.Revision, 99)
	require.NoError(t, err)
	_, err = db.ToggleInboxItemArchived(ctx, fourth.ID, fourth.Revision, 99)
	require.NoError(t, err)
	archivedByFeed, err = db.ListArchivedInboxItemsByFeed(ctx, "p", "feed-a", 10)
	require.NoError(t, err)
	assert.Equal(t, []int64{fourth.ID, third.ID, first.ID}, itemIDs(archivedByFeed), "id breaks identical archived_at ties deterministically")

	// Ignoring removes the item from its feed entirely and moves it to Trash.
	ignored, err := db.ToggleInboxItemIgnored(ctx, second.ID, second.Revision, 101)
	require.NoError(t, err)
	assert.NotNil(t, ignored.IgnoredAt)
	assert.False(t, ignored.Unread)
	byOtherFeed, err = db.ListInboxItemsByFeed(ctx, "p", "feed-b", 10)
	require.NoError(t, err)
	assert.Empty(t, byOtherFeed)
	trash, err = db.ListInboxItemsTrash(ctx, "p", 10)
	require.NoError(t, err)
	assert.Equal(t, []int64{unmatched.ID, second.ID}, itemIDs(trash), "trash holds unrouted and ignored items, newest activity first")
	counts, err = db.FeedCounts(ctx, "p")
	require.NoError(t, err)
	assert.Equal(t, []FeedInboxCount{{FeedID: "feed-a", Total: 0, Unread: 0, Archived: 3}}, counts, "an ignored item's feed drops from counts entirely")

	// Archiving an ignored item clears the ignore: the states are mutually
	// exclusive, and the item returns to its feed's archived section.
	movedToArchive, err := db.ToggleInboxItemArchived(ctx, second.ID, ignored.Revision, 102)
	require.NoError(t, err)
	assert.Nil(t, movedToArchive.IgnoredAt, "archive and ignored are mutually exclusive")
	assert.NotNil(t, movedToArchive.ArchivedAt)
	trash, err = db.ListInboxItemsTrash(ctx, "p", 10)
	require.NoError(t, err)
	assert.Equal(t, []int64{unmatched.ID}, itemIDs(trash))
	archivedOther, err := db.ListArchivedInboxItemsByFeed(ctx, "p", "feed-b", 10)
	require.NoError(t, err)
	assert.Equal(t, []int64{second.ID}, itemIDs(archivedOther))
}
