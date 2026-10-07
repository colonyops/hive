package stores

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/desktop/internal/app/data/models"
)

func requireUnixMilliNow(t *testing.T, timestamp, before, after int64) {
	t.Helper()
	assert.GreaterOrEqual(t, timestamp, before-1_000)
	assert.LessOrEqual(t, timestamp, after+1_000)
}

func TestEventLogWritesUseUnixMilliseconds(t *testing.T) {
	st, _ := openTestStores(t)
	ctx := t.Context()
	before := time.Now().UnixMilli()

	_, err := st.EventLog.Append(ctx, "source:test", "append", []byte(`{}`))
	require.NoError(t, err)
	_, err = st.EventLog.Append(ctx, "source:test", "changed", []byte(`{}`))
	require.NoError(t, err)
	require.NoError(t, st.SourceHeads.Upsert(ctx, "source:test", "changed", []byte(`{}`)))
	_, err = st.EventLog.AppendSnapshot(ctx, "source:test", "test", "scope", nil)
	require.NoError(t, err)
	after := time.Now().UnixMilli()

	msgs, _, err := st.EventLog.ReadFrom(ctx, 0, 10)
	require.NoError(t, err)
	require.Len(t, msgs, 3)
	for _, msg := range msgs {
		requireUnixMilliNow(t, msg.Ts, before, after)
	}
}

func TestCommitWritesUseUnixMilliseconds(t *testing.T) {
	st, _ := openTestStores(t)
	ctx := t.Context()
	before := time.Now().UnixMilli()

	require.NoError(t, st.EventLog.Commit(ctx, models.CommitBatch{
		Consumer:   "flow-1",
		UpToOffset: 1,
		Outputs: []models.Output{{
			Sink:          models.Sink{Kind: models.SinkKindAction, TargetID: "action-a"},
			OccurrenceKey: "item-1",
			Payload:       []byte(`{}`),
		}},
		NodeRuns: []models.NodeRun{{FlowID: "flow-1", NodeID: "node-a", OK: true}},
	}))
	after := time.Now().UnixMilli()

	command, err := st.OutputCommands.Get(ctx, 1)
	require.NoError(t, err)
	requireUnixMilliNow(t, command.CreatedAt, before, after)
	runs, err := st.NodeRuns.List(ctx, "flow-1", 1)
	require.NoError(t, err)
	require.Len(t, runs, 1)
	requireUnixMilliNow(t, runs[0].EndedAt, before, after)
}

func TestConfirmOutputCommandWritesUnixMilliseconds(t *testing.T) {
	st, _ := openTestStores(t)
	before := time.Now().UnixMilli()
	command, created, err := st.OutputCommands.Confirm(t.Context(), "action-a", "item-1", []byte(`{}`), models.ItemRef{}, "claim")
	after := time.Now().UnixMilli()
	require.NoError(t, err)
	require.True(t, created)
	requireUnixMilliNow(t, command.CreatedAt, before, after)
}
