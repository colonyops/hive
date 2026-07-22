package pipelinedb

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCommitBatch_FeedOutput_IsAcceptedWithoutPersistence(t *testing.T) {
	database := openTestDB(t)
	ctx := context.Background()

	require.NoError(t, database.CommitBatch(ctx, CommitBatch{
		Consumer:   "flow-1",
		UpToOffset: "1",
		Outputs: []Output{{
			Sink:    Sink{Kind: SinkKindFeed, TargetID: "feed-a"},
			Key:     "item-1",
			Payload: []byte(`{"v":1}`),
		}},
	}))

	offset, err := database.ConsumerOffset(ctx, "flow-1")
	require.NoError(t, err)
	assert.Equal(t, int64(1), offset)
}

func TestCommitBatch_ActionOutput_EnqueuesOnce(t *testing.T) {
	database := openTestDB(t)
	ctx := context.Background()

	countPending := func(t *testing.T) int {
		t.Helper()
		var count int
		require.NoError(t, database.Conn().QueryRowContext(
			ctx,
			`SELECT COUNT(*) FROM output_command WHERE action_id = ? AND key = ?`,
			"action-a", "item-1",
		).Scan(&count))
		return count
	}

	batch := CommitBatch{
		Consumer:   "flow-1",
		UpToOffset: "1",
		Outputs: []Output{
			{
				Sink:    Sink{Kind: SinkKindAction, TargetID: "action-a"},
				Key:     "item-1",
				Payload: []byte(`{"cmd":"do-it"}`),
			},
		},
	}
	require.NoError(t, database.CommitBatch(ctx, batch))
	assert.Equal(t, 1, countPending(t))

	// A duplicate (action_id, key) from a later batch is a no-op: the
	// action fires at most once.
	batch2 := CommitBatch{
		Consumer:   "flow-1",
		UpToOffset: "2",
		Outputs: []Output{
			{
				Sink:    Sink{Kind: SinkKindAction, TargetID: "action-a"},
				Key:     "item-1",
				Payload: []byte(`{"cmd":"do-it-again"}`),
			},
		},
	}
	require.NoError(t, database.CommitBatch(ctx, batch2))
	assert.Equal(t, 1, countPending(t), "duplicate (action_id, key) must not enqueue a second command")
}

func TestCommitBatch_InsertsNodeRuns(t *testing.T) {
	database := openTestDB(t)
	ctx := context.Background()

	batch := CommitBatch{
		Consumer:   "flow-1",
		UpToOffset: "1",
		NodeRuns: []NodeRunView{
			{
				FlowID:    "flow-1",
				NodeID:    "node-a",
				OK:        true,
				InCount:   3,
				OutCount:  2,
				DropCount: 1,
				DurMs:     42,
			},
			{
				FlowID:   "flow-1",
				NodeID:   "node-b",
				OK:       false,
				Err:      "boom",
				InCount:  1,
				OutCount: 0,
			},
		},
	}
	require.NoError(t, database.CommitBatch(ctx, batch))

	rows, err := database.Conn().QueryContext(ctx,
		`SELECT node_id, ok, in_count, out_count, drop_count, err, dur_ms FROM node_run ORDER BY node_id`)
	require.NoError(t, err)
	defer func() { require.NoError(t, rows.Close()) }()

	type row struct {
		nodeID    string
		ok        int64
		inCount   int64
		outCount  int64
		dropCount int64
		err       *string
		durMs     int64
	}
	var got []row
	for rows.Next() {
		var r row
		require.NoError(t, rows.Scan(&r.nodeID, &r.ok, &r.inCount, &r.outCount, &r.dropCount, &r.err, &r.durMs))
		got = append(got, r)
	}
	require.NoError(t, rows.Err())
	require.Len(t, got, 2)

	assert.Equal(t, "node-a", got[0].nodeID)
	assert.Equal(t, int64(1), got[0].ok)
	assert.Equal(t, int64(3), got[0].inCount)
	assert.Equal(t, int64(2), got[0].outCount)
	assert.Equal(t, int64(1), got[0].dropCount)
	assert.Nil(t, got[0].err)
	assert.Equal(t, int64(42), got[0].durMs)

	assert.Equal(t, "node-b", got[1].nodeID)
	assert.Equal(t, int64(0), got[1].ok)
	require.NotNil(t, got[1].err)
	assert.Equal(t, "boom", *got[1].err)
}

func TestCommitBatch_AdvancesOffset_AndIsIdempotentOnReplay(t *testing.T) {
	database := openTestDB(t)
	ctx := context.Background()

	batch := CommitBatch{
		Consumer:   "flow-1",
		UpToOffset: "5",
		NodeRuns: []NodeRunView{
			{FlowID: "flow-1", NodeID: "node-a", OK: true},
		},
	}
	require.NoError(t, database.CommitBatch(ctx, batch))

	offset, err := database.ConsumerOffset(ctx, "flow-1")
	require.NoError(t, err)
	assert.Equal(t, int64(5), offset)

	countRows := func(t *testing.T, table string) int {
		t.Helper()
		var count int
		require.NoError(t, database.Conn().QueryRowContext(ctx, "SELECT COUNT(*) FROM "+table).Scan(&count))
		return count
	}
	nodeRunsBefore := countRows(t, "node_run")

	// Replaying the exact same batch (UpToOffset <= current) must be a
	// full no-op: no new node_run rows, offset unchanged.
	require.NoError(t, database.CommitBatch(ctx, batch))

	offset, err = database.ConsumerOffset(ctx, "flow-1")
	require.NoError(t, err)
	assert.Equal(t, int64(5), offset, "replaying an already-applied batch must not change the offset")
	assert.Equal(t, nodeRunsBefore, countRows(t, "node_run"), "replay must not insert duplicate node_run rows")

	// A batch with UpToOffset below the current committed offset is also a
	// no-op, even with different/new outputs.
	staleBatch := CommitBatch{
		Consumer:   "flow-1",
		UpToOffset: "3",
		Outputs: []Output{{
			Sink:    Sink{Kind: SinkKindAction, TargetID: "action-a"},
			Key:     "item-new",
			Payload: []byte(`{"v":"new"}`),
		}},
	}
	require.NoError(t, database.CommitBatch(ctx, staleBatch))

	offset, err = database.ConsumerOffset(ctx, "flow-1")
	require.NoError(t, err)
	assert.Equal(t, int64(5), offset, "a stale UpToOffset must not regress the committed offset")

	assert.Zero(t, countRows(t, "output_command"), "a no-op stale batch must not apply its outputs")
}

func TestCommitBatch_UnknownSinkKind_Errors(t *testing.T) {
	database := openTestDB(t)
	ctx := context.Background()

	batch := CommitBatch{
		Consumer:   "flow-1",
		UpToOffset: "1",
		Outputs: []Output{
			{Sink: Sink{Kind: "bogus", TargetID: "x"}, Key: "k", Payload: []byte(`{}`)},
		},
	}
	err := database.CommitBatch(ctx, batch)
	require.Error(t, err)

	// The whole batch must roll back: the offset must not advance either.
	offset, offsetErr := database.ConsumerOffset(ctx, "flow-1")
	require.NoError(t, offsetErr)
	assert.Equal(t, int64(0), offset)
}
