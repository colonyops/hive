package pipelinedb

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInboxViewsTriageAndFeedCounts(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	empty, err := db.InboxCounts(ctx, "empty")
	require.NoError(t, err)
	assert.Equal(t, InboxCounts{}, empty)

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
	require.Len(t, byFeed, 1)
	assert.Equal(t, first.ID, byFeed[0].ID)
	byOtherFeed, err := db.ListInboxItemsByFeed(ctx, "p", "feed-b", 10)
	require.NoError(t, err)
	require.Len(t, byOtherFeed, 1)
	assert.Equal(t, second.ID, byOtherFeed[0].ID)
	counts, err := db.FeedCounts(ctx, "p")
	require.NoError(t, err)
	assert.Equal(t, []FeedInboxCount{{FeedID: "feed-a", Total: 1, Unread: 1}, {FeedID: "feed-b", Total: 1, Unread: 0}}, counts)
	inbox, err := db.ListInboxItems(ctx, "p", "inbox", 10)
	require.NoError(t, err)
	assert.Len(t, inbox, 2)
	assert.NotContains(t, []int64{inbox[0].ID, inbox[1].ID}, unmatched.ID, "items that never reach a feed stay out of workspace views")

	updated, err := db.SetInboxItemUnread(ctx, first.ID, first.Revision, false)
	require.NoError(t, err)
	assert.Equal(t, first.Revision+1, updated.Revision)
	_, err = db.SetInboxItemUnread(ctx, first.ID, first.Revision, true)
	require.ErrorIs(t, err, ErrStaleInboxItem)
	archived, err := db.ToggleInboxItemArchived(ctx, first.ID, updated.Revision, 99)
	require.NoError(t, err)
	assert.Equal(t, updated.Revision+1, archived.Revision)
	_, err = db.ToggleInboxItemArchived(ctx, first.ID, updated.Revision, 99)
	require.ErrorIs(t, err, ErrStaleInboxItem)
	byFeed, err = db.ListInboxItemsByFeed(ctx, "p", "feed-a", 10)
	require.NoError(t, err)
	assert.Empty(t, byFeed)
	counts, err = db.FeedCounts(ctx, "p")
	require.NoError(t, err)
	assert.Equal(t, []FeedInboxCount{{FeedID: "feed-b", Total: 1, Unread: 0}}, counts)

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

	archive, err := db.ListInboxItems(ctx, "p", "archive", 10)
	require.NoError(t, err)
	assert.Equal(t, []int64{fourth.ID, third.ID, first.ID}, []int64{archive[0].ID, archive[1].ID, archive[2].ID}, "id breaks identical archived_at ties deterministically")
	all, err := db.ListInboxItems(ctx, "p", "all", 10)
	require.NoError(t, err)
	assert.Equal(t, []int64{second.ID, first.ID, fourth.ID, third.ID}, []int64{all[0].ID, all[1].ID, all[2].ID, all[3].ID}, "id breaks same last-event ties")

	ignored, err := db.ToggleInboxItemIgnored(ctx, second.ID, second.Revision, 101)
	require.NoError(t, err)
	assert.NotNil(t, ignored.IgnoredAt)
	assert.False(t, ignored.Unread)
	ignoredItems, err := db.ListInboxItems(ctx, "p", "ignored", 10)
	require.NoError(t, err)
	require.Len(t, ignoredItems, 1)
	assert.Equal(t, second.ID, ignoredItems[0].ID)
	all, err = db.ListInboxItems(ctx, "p", "all", 10)
	require.NoError(t, err)
	assert.NotContains(t, []int64{all[0].ID, all[1].ID, all[2].ID}, second.ID)

	movedToArchive, err := db.ToggleInboxItemArchived(ctx, second.ID, ignored.Revision, 102)
	require.NoError(t, err)
	assert.Nil(t, movedToArchive.IgnoredAt, "archive and ignored are mutually exclusive")
	assert.NotNil(t, movedToArchive.ArchivedAt)
	ignoredItems, err = db.ListInboxItems(ctx, "p", "ignored", 10)
	require.NoError(t, err)
	assert.Empty(t, ignoredItems)
}
