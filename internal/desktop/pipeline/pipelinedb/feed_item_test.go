package pipelinedb

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// upsertFeedItemAt writes a feed_item row with an explicit updated_at,
// bypassing CommitBatch (which always stamps time.Now()) so ordering tests
// can control timestamps deterministically instead of relying on wall-clock
// separation between calls — mirroring log_test.go's insertEventAt.
func upsertFeedItemAt(t *testing.T, database *DB, feedID, itemID, payload string, updatedAt int64) {
	t.Helper()
	require.NoError(t, database.queries.UpsertFeedItem(context.Background(), UpsertFeedItemParams{
		FeedID:    feedID,
		ItemID:    itemID,
		Payload:   []byte(payload),
		UpdatedAt: updatedAt,
		Unread:    0,
	}))
}

func TestFeedItems_NewestFirst(t *testing.T) {
	database := openTestDB(t)
	ctx := context.Background()

	upsertFeedItemAt(t, database, "feed-a", "item-1", `{"n":1}`, 100)
	upsertFeedItemAt(t, database, "feed-a", "item-2", `{"n":2}`, 200)
	upsertFeedItemAt(t, database, "feed-a", "item-3", `{"n":3}`, 300)
	// A different feed must not show up in feed-a's results.
	upsertFeedItemAt(t, database, "feed-b", "item-other", `{"n":"other"}`, 400)

	items, err := database.FeedItems(ctx, "feed-a")
	require.NoError(t, err)
	require.Len(t, items, 3)

	assert.Equal(t, "item-3", items[0].ItemID)
	assert.Equal(t, "item-2", items[1].ItemID)
	assert.Equal(t, "item-1", items[2].ItemID)
	for i := 1; i < len(items); i++ {
		assert.GreaterOrEqual(t, items[i-1].UpdatedAt, items[i].UpdatedAt)
	}
}

func TestFeedItems_EmptyFeed(t *testing.T) {
	database := openTestDB(t)
	ctx := context.Background()

	items, err := database.FeedItems(ctx, "no-such-feed")
	require.NoError(t, err)
	assert.Empty(t, items)
}

func TestMarkFeedItemRead_ClearsUnread(t *testing.T) {
	database := openTestDB(t)
	ctx := context.Background()

	require.NoError(t, database.CommitBatch(ctx, CommitBatch{
		Consumer:   "flow-1",
		UpToOffset: 1,
		Outputs: []Output{
			{
				Sink:    Sink{Kind: SinkKindFeed, TargetID: "feed-a"},
				Key:     "item-1",
				Payload: []byte(`{}`),
				Unread:  true,
			},
		},
	}))

	items, err := database.FeedItems(ctx, "feed-a")
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.True(t, items[0].Unread)

	require.NoError(t, database.MarkFeedItemRead(ctx, "feed-a", "item-1"))

	items, err = database.FeedItems(ctx, "feed-a")
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.False(t, items[0].Unread)
}
