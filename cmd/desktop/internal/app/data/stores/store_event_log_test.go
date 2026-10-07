package stores

import (
	"encoding/json"
	"fmt"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/desktop/internal/app/data/models"
)

func TestAppend_ReadFrom_Monotonic(t *testing.T) {
	st, _ := openTestStores(t)
	ctx := t.Context()

	var offsets []int64
	for i := range 3 {
		offset, err := st.EventLog.Append(ctx, "source:test", fmt.Sprintf("key-%d", i), fmt.Appendf(nil, `{"n":%d}`, i))
		require.NoError(t, err)
		offsets = append(offsets, offset)
	}

	for i := 1; i < len(offsets); i++ {
		assert.Greater(t, offsets[i], offsets[i-1])
	}

	msgs, next, err := st.EventLog.ReadFrom(ctx, 0, 10)
	require.NoError(t, err)
	require.Len(t, msgs, 3)
	assert.Equal(t, offsets[2], next)

	for i, msg := range msgs {
		assert.Equal(t, "source:test", msg.Topic)
		assert.Equal(t, fmt.Sprintf("key-%d", i), msg.Key)
		assert.Equal(t, fmt.Sprintf(`{"n":%d}`, i), string(msg.Payload))
		assert.Positive(t, msg.Ts)
		assert.Equal(t, fmt.Sprintf("%d", offsets[i]), msg.ID)
	}

	msgs, next, err = st.EventLog.ReadFrom(ctx, offsets[2], 10)
	require.NoError(t, err)
	assert.Empty(t, msgs)
	assert.Equal(t, offsets[2], next)

	msgs, next, err = st.EventLog.ReadFrom(ctx, offsets[0], 1)
	require.NoError(t, err)
	require.Len(t, msgs, 1)
	assert.Equal(t, "key-1", msgs[0].Key)
	assert.Equal(t, offsets[1], next)
}

func TestReadForConsumer_ResumesFromPersistedOffset(t *testing.T) {
	st, _ := openTestStores(t)
	ctx := t.Context()

	for i := range 3 {
		_, err := st.EventLog.Append(ctx, "source:test", fmt.Sprintf("key-%d", i), []byte(`{}`))
		require.NoError(t, err)
	}
	require.NoError(t, st.EventLog.Commit(ctx, models.CommitBatch{Consumer: "flow-1", UpToOffset: 2}))

	msgs, err := st.EventLog.ReadForConsumer(ctx, "flow-1", 500, models.RoutedTopics{All: true})
	require.NoError(t, err)
	require.Len(t, msgs, 1)
	assert.Equal(t, "3", msgs[0].ID)
}

func TestReadForConsumer_LoadsPayloadsOnlyForRoutedTopics(t *testing.T) {
	st, _ := openTestStores(t)
	ctx := t.Context()

	_, err := st.EventLog.AppendSnapshot(ctx, "source:mine/a", "github", "", []models.SnapshotItem{{Key: "k", Payload: []byte(`{"n":1}`)}})
	require.NoError(t, err)
	_, err = st.EventLog.AppendSnapshot(ctx, "source:other/a", "github", "", []models.SnapshotItem{{Key: "k", Payload: []byte(`{"n":2}`)}})
	require.NoError(t, err)
	_, err = st.EventLog.Append(ctx, "source:other/a", "k", []byte(`{"n":3}`))
	require.NoError(t, err)

	msgs, err := st.EventLog.ReadForConsumer(ctx, "mine", 10, models.RoutedTopics{Topics: []string{"source:mine/a"}})
	require.NoError(t, err)
	require.Len(t, msgs, 3, "unrouted messages still come back so the consumer can move past them")

	assert.Equal(t, []models.SnapshotItem{{Key: "k", Payload: []byte(`{"n":1}`)}}, msgs[0].Snapshot)
	for _, msg := range msgs[1:] {
		assert.Equal(t, "source:other/a", msg.Topic)
		assert.Nil(t, msg.Payload)
		assert.Nil(t, msg.Snapshot)
	}

	msgs, err = st.EventLog.ReadForConsumer(ctx, "mine", 10, models.RoutedTopics{Topics: []string{}})
	require.NoError(t, err)
	require.Len(t, msgs, 3)
	for _, msg := range msgs {
		assert.Nil(t, msg.Payload)
	}
}

func TestListLatestSnapshots_ReturnsLatestSnapshotPerOwnedSource(t *testing.T) {
	st, _ := openTestStores(t)
	ctx := t.Context()

	_, err := st.EventLog.AppendSnapshot(ctx, "source:flow/a", "github", "scope-a", []models.SnapshotItem{{Key: "old", Payload: []byte(`{"version":1}`)}})
	require.NoError(t, err)
	latestA, err := st.EventLog.AppendSnapshot(ctx, "source:flow/a", "github", "scope-a", []models.SnapshotItem{{Key: "new", Payload: []byte(`{"version":2}`)}})
	require.NoError(t, err)
	latestB, err := st.EventLog.AppendSnapshot(ctx, "source:flow/b", "github", "scope-b", []models.SnapshotItem{})
	require.NoError(t, err)
	_, err = st.EventLog.AppendSnapshot(ctx, "source:flow-other/a", "github", "other", []models.SnapshotItem{{Key: "wrong-profile", Payload: []byte(`{}`)}})
	require.NoError(t, err)

	tail, err := st.EventLog.TailOffset(ctx)
	require.NoError(t, err)
	messages, err := st.EventLog.ListLatestSnapshots(ctx, "flow", tail)
	require.NoError(t, err)
	require.Len(t, messages, 2)
	assert.Equal(t, models.Msg{ID: strconv.FormatInt(latestA, 10), Topic: "source:flow/a", Ts: messages[0].Ts, Payload: json.RawMessage(`[{"key":"new","payload":{"version":2}}]`), Snapshot: []models.SnapshotItem{{Key: "new", Payload: json.RawMessage(`{"version":2}`)}}, SourceKind: "github", SourceScope: "scope-a"}, messages[0])
	assert.Equal(t, models.Msg{ID: strconv.FormatInt(latestB, 10), Topic: "source:flow/b", Ts: messages[1].Ts, Payload: json.RawMessage(`[]`), Snapshot: []models.SnapshotItem{}, SourceKind: "github", SourceScope: "scope-b"}, messages[1])
}

func TestListLatestSnapshots_HonorsCapturedTail(t *testing.T) {
	st, _ := openTestStores(t)
	ctx := t.Context()

	capturedTail, err := st.EventLog.AppendSnapshot(ctx, "source:flow/a", "github", "scope", []models.SnapshotItem{{Key: "old", Payload: []byte(`{}`)}})
	require.NoError(t, err)
	_, err = st.EventLog.AppendSnapshot(ctx, "source:flow/a", "github", "scope", []models.SnapshotItem{{Key: "new", Payload: []byte(`{}`)}})
	require.NoError(t, err)

	messages, err := st.EventLog.ListLatestSnapshots(ctx, "flow", capturedTail)
	require.NoError(t, err)
	require.Len(t, messages, 1)
	assert.Equal(t, "old", messages[0].Snapshot[0].Key)
	assert.Equal(t, "github", messages[0].SourceKind)
	assert.Equal(t, "scope", messages[0].SourceScope)
}

func TestReadFrom_EmptySnapshotSurvivesJSON(t *testing.T) {
	st, _ := openTestStores(t)
	ctx := t.Context()

	_, err := st.EventLog.AppendSnapshot(ctx, "source:test", "github", "", []models.SnapshotItem{})
	require.NoError(t, err)

	msgs, _, err := st.EventLog.ReadFrom(ctx, 0, 10)
	require.NoError(t, err)
	require.Len(t, msgs, 1)
	require.NotNil(t, msgs[0].Snapshot, "an empty snapshot must decode as a non-nil slice")

	// The frontend engine routes on `msg.Snapshot != null`; an empty snapshot
	// must therefore cross the binding as [] rather than being omitted, or the
	// boundary row (key "") is treated as an ordinary item and models.CommitBatch
	// wedges the consumer on resolving inbox item "<kind>//".
	wire, err := json.Marshal(msgs[0])
	require.NoError(t, err)
	assert.Contains(t, string(wire), `"Snapshot":[]`)
}
