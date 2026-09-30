package stores

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/desktop/internal/app/data/models"
	"github.com/colonyops/hive/cmd/desktop/internal/app/data/queries"
)

func seedClaimItem(t *testing.T, db *queries.DB, externalID string) int64 {
	t.Helper()
	item, err := db.InsertInboxItem(t.Context(), queries.InsertInboxItemParams{
		ProfileID: "p", SourceKind: "github", SourceScope: "s", ExternalID: externalID,
		Payload: []byte(`{}`), Lifecycle: "active",
	})
	require.NoError(t, err)
	return item.ID
}

func countFeedClaims(t *testing.T, db *queries.DB) int {
	t.Helper()
	var n int
	require.NoError(t, db.Conn().QueryRowContext(t.Context(), `SELECT COUNT(*) FROM feed_membership_claim`).Scan(&n))
	return n
}

func TestFeedClaimStore_UpsertIsIdempotent(t *testing.T) {
	st, db := openTestStores(t)
	ctx := t.Context()
	itemID := seedClaimItem(t, db, "item-1")

	claim := models.FeedClaim{ProfileID: "p", FeedID: "p/feed", ItemID: itemID, SourceID: "source-a"}
	require.NoError(t, st.FeedClaims.Upsert(ctx, claim))
	require.NoError(t, st.FeedClaims.Upsert(ctx, claim))
	assert.Equal(t, 1, countFeedClaims(t, db))
}

func TestFeedClaimStore_DeleteNotInSnapshot(t *testing.T) {
	st, db := openTestStores(t)
	ctx := t.Context()
	kept := seedClaimItem(t, db, "kept")
	dropped := seedClaimItem(t, db, "dropped")

	require.NoError(t, st.FeedClaims.Upsert(ctx, models.FeedClaim{ProfileID: "p", FeedID: "p/feed", ItemID: kept, SourceID: "source-a"}))
	require.NoError(t, st.FeedClaims.Upsert(ctx, models.FeedClaim{ProfileID: "p", FeedID: "p/feed", ItemID: dropped, SourceID: "source-a"}))

	require.NoError(t, st.FeedClaims.DeleteNotInSnapshot(ctx, "p/feed", "source-a", []int64{kept}))
	assert.Equal(t, 1, countFeedClaims(t, db))
}

func TestFeedClaimStore_DeleteForSourceAll(t *testing.T) {
	st, db := openTestStores(t)
	ctx := t.Context()
	itemID := seedClaimItem(t, db, "item-1")

	require.NoError(t, st.FeedClaims.Upsert(ctx, models.FeedClaim{ProfileID: "p", FeedID: "p/feed", ItemID: itemID, SourceID: "source-a"}))
	require.NoError(t, st.FeedClaims.DeleteForSourceAll(ctx, "p/feed", "source-a"))
	assert.Equal(t, 0, countFeedClaims(t, db))
}

func TestFeedClaimStore_DeleteForFeeds(t *testing.T) {
	st, db := openTestStores(t)
	ctx := t.Context()
	itemID := seedClaimItem(t, db, "item-1")

	require.NoError(t, st.FeedClaims.Upsert(ctx, models.FeedClaim{ProfileID: "p", FeedID: "p/keep", ItemID: itemID, SourceID: "source-a"}))
	require.NoError(t, st.FeedClaims.Upsert(ctx, models.FeedClaim{ProfileID: "p", FeedID: "p/drop", ItemID: itemID, SourceID: "source-a"}))

	require.NoError(t, st.FeedClaims.DeleteForFeeds(ctx, "p", []string{"p/keep"}))
	assert.Equal(t, 1, countFeedClaims(t, db))

	require.NoError(t, st.FeedClaims.DeleteForFeeds(ctx, "p", nil))
	assert.Equal(t, 0, countFeedClaims(t, db))
}

func TestFeedClaimStore_DeleteForRemovedSources(t *testing.T) {
	st, db := openTestStores(t)
	ctx := t.Context()
	itemID := seedClaimItem(t, db, "item-1")

	require.NoError(t, st.FeedClaims.Upsert(ctx, models.FeedClaim{ProfileID: "p", FeedID: "p/feed", ItemID: itemID, SourceID: "source-keep"}))
	require.NoError(t, st.FeedClaims.Upsert(ctx, models.FeedClaim{ProfileID: "p", FeedID: "p/feed", ItemID: itemID, SourceID: "source-drop"}))

	require.NoError(t, st.FeedClaims.DeleteForRemovedSources(ctx, "p", []string{"source-keep"}))
	assert.Equal(t, 1, countFeedClaims(t, db))

	require.NoError(t, st.FeedClaims.DeleteForRemovedSources(ctx, "p", nil))
	assert.Equal(t, 0, countFeedClaims(t, db))
}

func TestFeedClaimStore_DeleteUnarchivedByProfile(t *testing.T) {
	st, db := openTestStores(t)
	ctx := t.Context()
	itemID := seedClaimItem(t, db, "item-1")
	other := seedClaimItem(t, db, "item-2")

	require.NoError(t, st.FeedClaims.Upsert(ctx, models.FeedClaim{ProfileID: "p", FeedID: "p/feed", ItemID: itemID, SourceID: "source-a"}))
	require.NoError(t, st.FeedClaims.Upsert(ctx, models.FeedClaim{ProfileID: "q", FeedID: "q/feed", ItemID: other, SourceID: "source-a"}))

	require.NoError(t, st.FeedClaims.DeleteUnarchivedByProfile(ctx, "p"))

	assert.Equal(t, 1, countFeedClaims(t, db))
}
