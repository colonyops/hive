package pipelinedb

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func requireUnixMilliNow(t *testing.T, timestamp, before, after int64) {
	t.Helper()
	assert.GreaterOrEqual(t, timestamp, before-1_000)
	assert.LessOrEqual(t, timestamp, after+1_000)
}

func TestEventLogWritesUseUnixMilliseconds(t *testing.T) {
	database := openTestDB(t)
	ctx := context.Background()
	before := time.Now().UnixMilli()

	_, err := database.Append(ctx, "source:test", "append", []byte(`{}`))
	require.NoError(t, err)
	_, appended, err := database.AppendIfChanged(ctx, "source:test", "changed", []byte(`{}`))
	require.NoError(t, err)
	require.True(t, appended)
	_, err = database.AppendSnapshot(ctx, "source:test", "test", "scope", nil)
	require.NoError(t, err)
	after := time.Now().UnixMilli()

	msgs, _, err := database.ReadFrom(ctx, 0, 10)
	require.NoError(t, err)
	require.Len(t, msgs, 3)
	for _, msg := range msgs {
		requireUnixMilliNow(t, msg.Ts, before, after)
	}
}

func TestCommitBatchWritesUseUnixMilliseconds(t *testing.T) {
	database := openTestDB(t)
	ctx := context.Background()
	before := time.Now().UnixMilli()

	require.NoError(t, database.CommitBatch(ctx, CommitBatch{
		Consumer:   "flow-1",
		UpToOffset: "1",
		Outputs: []Output{{
			Sink:          Sink{Kind: SinkKindAction, TargetID: "action-a"},
			OccurrenceKey: "item-1",
			Payload:       []byte(`{}`),
		}},
		NodeRuns: []NodeRunView{{FlowID: "flow-1", NodeID: "node-a", OK: true}},
	}))
	after := time.Now().UnixMilli()

	command, err := database.OutputCommand(ctx, 1)
	require.NoError(t, err)
	requireUnixMilliNow(t, command.CreatedAt, before, after)
	runs, err := database.NodeRuns(ctx, "flow-1", 1)
	require.NoError(t, err)
	require.Len(t, runs, 1)
	requireUnixMilliNow(t, runs[0].EndedAt, before, after)
}

func TestConfirmOutputCommandWritesUnixMilliseconds(t *testing.T) {
	database := openTestDB(t)
	before := time.Now().UnixMilli()
	command, created, err := database.ConfirmOutputCommand(t.Context(), "action-a", "item-1", []byte(`{}`))
	after := time.Now().UnixMilli()
	require.NoError(t, err)
	require.True(t, created)
	requireUnixMilliNow(t, command.CreatedAt, before, after)
}
