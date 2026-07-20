package pipelinedb

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCommitBatch_ReconcilesSnapshotAtomicallyAndPreservesUnread(t *testing.T) {
	database := openTestDB(t)
	ctx := context.Background()
	const (
		consumer = "flow-1"
		feedID   = "flow-1/feed"
		topic    = "source:flow-1/source"
	)

	initial := CommitBatch{
		Consumer:   consumer,
		UpToOffset: "1",
		Outputs: []Output{
			{Sink: Sink{Kind: SinkKindFeed, TargetID: feedID}, Key: "gone", Payload: []byte(`{"v":1}`), Unread: true, SourceTopic: topic, SnapshotID: "1", PreserveUnread: true},
			{Sink: Sink{Kind: SinkKindFeed, TargetID: feedID}, Key: "kept", Payload: []byte(`{"v":2}`), Unread: true, SourceTopic: topic, SnapshotID: "1", PreserveUnread: true},
		},
		FeedSnapshots: []FeedSnapshot{{FeedID: feedID, SourceTopic: topic, SnapshotID: "1"}},
	}
	require.NoError(t, database.CommitBatch(ctx, initial))
	require.NoError(t, database.MarkFeedItemRead(ctx, feedID, "kept"))

	// Reconciliation is part of the same transaction as offset advancement.
	// A failed delete must leave both the prior rows and prior offset intact.
	_, err := database.Conn().ExecContext(ctx, `
		CREATE TRIGGER fail_snapshot_delete
		BEFORE DELETE ON feed_item
		BEGIN SELECT RAISE(FAIL, 'delete blocked'); END;
	`)
	require.NoError(t, err)
	failed := CommitBatch{
		Consumer:      consumer,
		UpToOffset:    "2",
		FeedSnapshots: []FeedSnapshot{{FeedID: feedID, SourceTopic: topic, SnapshotID: "2"}},
	}
	require.Error(t, database.CommitBatch(ctx, failed))
	offset, err := database.ConsumerOffset(ctx, consumer)
	require.NoError(t, err)
	assert.Equal(t, int64(1), offset)
	require.NoError(t, database.MarkFeedItemRead(ctx, feedID, "kept"))
	_, err = database.Conn().ExecContext(ctx, `DROP TRIGGER fail_snapshot_delete`)
	require.NoError(t, err)

	// The next source snapshot no longer contains "gone". The surviving item
	// is routed again because snapshots are complete, but preserving unread
	// avoids turning a read, unchanged row back into unread.
	reconcile := CommitBatch{
		Consumer:   consumer,
		UpToOffset: "2",
		Outputs: []Output{
			{Sink: Sink{Kind: SinkKindFeed, TargetID: feedID}, Key: "kept", Payload: []byte(`{"v":2}`), Unread: true, SourceTopic: topic, SnapshotID: "2", PreserveUnread: true},
		},
		FeedSnapshots: []FeedSnapshot{{FeedID: feedID, SourceTopic: topic, SnapshotID: "2"}},
	}
	require.NoError(t, database.CommitBatch(ctx, reconcile))

	items, err := database.FeedItems(ctx, feedID)
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, "kept", items[0].ItemID)
	assert.False(t, items[0].Unread, "an unchanged surviving snapshot item remains read")

	offset, err = database.ConsumerOffset(ctx, consumer)
	require.NoError(t, err)
	assert.Equal(t, int64(2), offset, "reconciliation and offset advancement commit together")
}
