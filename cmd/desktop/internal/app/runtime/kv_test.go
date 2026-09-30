package runtime

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/desktop/internal/app/data/models"
	"github.com/colonyops/hive/cmd/desktop/internal/app/flow"
)

type fakeKVReader struct {
	rows map[string]string // "nodeID/key" -> value
}

func (f fakeKVReader) Get(_ context.Context, _, nodeID, key string, _ int64) (string, bool, error) {
	value, ok := f.rows[nodeID+"/"+key]
	return value, ok, nil
}

func (f fakeKVReader) Keys(_ context.Context, _, nodeID, prefix string, _ int64) ([]string, error) {
	var keys []string
	for row := range f.rows {
		id, key, _ := cutRow(row)
		if id == nodeID && hasPrefix(key, prefix) {
			keys = append(keys, key)
		}
	}
	return keys, nil
}

func cutRow(row string) (nodeID, key string, ok bool) {
	for i := range row {
		if row[i] == '/' {
			return row[:i], row[i+1:], true
		}
	}
	return "", row, false
}

func hasPrefix(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}

func TestNodeStaging_ReadYourWrites(t *testing.T) {
	buf := newKVBuffer(fakeKVReader{}, "f", 1000)
	stg := buf.node("n")
	ctx := t.Context()

	require.NoError(t, stg.Set(ctx, "k", `"v"`, 0))
	value, found, err := stg.Get(ctx, "k")
	require.NoError(t, err)
	require.True(t, found)
	assert.Equal(t, `"v"`, value)
}

func TestNodeStaging_TombstoneAndSetPrecedence(t *testing.T) {
	buf := newKVBuffer(fakeKVReader{rows: map[string]string{"n/durable": `1`}}, "f", 1000)
	stg := buf.node("n")
	ctx := t.Context()

	require.NoError(t, stg.Delete(ctx, "k"))
	require.NoError(t, stg.Set(ctx, "k", `2`, 0))
	value, found, err := stg.Get(ctx, "k")
	require.NoError(t, err)
	require.True(t, found)
	assert.Equal(t, `2`, value)

	require.NoError(t, stg.Set(ctx, "durable", `2`, 0))
	require.NoError(t, stg.Delete(ctx, "durable"))
	_, found, err = stg.Get(ctx, "durable")
	require.NoError(t, err)
	assert.False(t, found, "a delete after a set shadows the durable row too")
}

func TestNodeStaging_FallsThroughToTheReader(t *testing.T) {
	buf := newKVBuffer(fakeKVReader{rows: map[string]string{"n/k": `"durable"`}}, "f", 1000)
	stg := buf.node("n")

	value, found, err := stg.Get(t.Context(), "k")
	require.NoError(t, err)
	require.True(t, found)
	assert.Equal(t, `"durable"`, value)
}

func TestNodeStaging_OverlayHonorsExpiry(t *testing.T) {
	buf := newKVBuffer(fakeKVReader{}, "f", 1000)
	stg := buf.node("n")
	ctx := t.Context()

	require.NoError(t, stg.Set(ctx, "k", `1`, 60))
	_, found, err := stg.Get(ctx, "k")
	require.NoError(t, err)
	assert.True(t, found)

	buf.now = 1000 + 60*1000
	_, found, err = stg.Get(ctx, "k")
	require.NoError(t, err)
	assert.False(t, found, "an overlay entry at its expiry cutoff reads absent")
}

func TestNodeStaging_HonorsContextCancellation(t *testing.T) {
	buf := newKVBuffer(fakeKVReader{}, "f", 1000)
	stg := buf.node("n")
	ctx, cancel := context.WithCancel(t.Context())
	require.NoError(t, stg.Set(ctx, "k", `1`, 0))
	cancel()

	_, _, err := stg.Get(ctx, "k")
	require.ErrorIs(t, err, context.Canceled)
	require.ErrorIs(t, stg.Set(ctx, "k", `1`, 0), context.Canceled)
	_, err = stg.Keys(ctx, "")
	require.ErrorIs(t, err, context.Canceled)
}

func TestNodeStaging_KeysUnionMinusTombstones(t *testing.T) {
	buf := newKVBuffer(fakeKVReader{rows: map[string]string{"n/a": `1`, "n/b": `1`, "n/x": `1`}}, "f", 1000)
	ctx := t.Context()

	merged := buf.node("n")
	require.NoError(t, merged.Set(ctx, "c", `1`, 0))
	require.NoError(t, merged.Delete(ctx, "b"))
	merged.commit()

	stg := buf.node("n")
	require.NoError(t, stg.Set(ctx, "d", `1`, 0))
	require.NoError(t, stg.Delete(ctx, "x"))

	keys, err := stg.Keys(ctx, "")
	require.NoError(t, err)
	assert.Equal(t, []string{"a", "c", "d"}, keys)

	prefixed, err := stg.Keys(ctx, "c")
	require.NoError(t, err)
	assert.Equal(t, []string{"c"}, prefixed)
}

func TestNodeStaging_UncommittedWritesInvisibleToLaterMessages(t *testing.T) {
	buf := newKVBuffer(fakeKVReader{}, "f", 1000)
	ctx := t.Context()

	errored := buf.node("n")
	require.NoError(t, errored.Set(ctx, "k", `1`, 0))

	next := buf.node("n")
	_, found, err := next.Get(ctx, "k")
	require.NoError(t, err)
	assert.False(t, found)
	assert.Empty(t, buf.mutations())
}

func TestKVBuffer_MutationsAreDeterministicallyOrdered(t *testing.T) {
	buf := newKVBuffer(fakeKVReader{}, "f", 1000)
	ctx := t.Context()

	b := buf.node("node-b")
	require.NoError(t, b.Set(ctx, "z", `1`, 0))
	require.NoError(t, b.Set(ctx, "a", `1`, 60))
	require.NoError(t, b.Delete(ctx, "m"))
	b.commit()
	a := buf.node("node-a")
	require.NoError(t, a.Set(ctx, "k", `1`, 0))
	a.commit()

	want := []models.KVMutation{
		{NodeID: "node-a", Key: "k", Value: `1`},
		{NodeID: "node-b", Key: "a", Value: `1`, ExpiresAt: 1000 + 60*1000},
		{NodeID: "node-b", Key: "m", Delete: true},
		{NodeID: "node-b", Key: "z", Value: `1`},
	}
	assert.Equal(t, want, buf.mutations())
	assert.Equal(t, want, buf.mutations(), "repeated drains yield the same order")
}

func TestInertKVBuffer_DisabledInBothDirections(t *testing.T) {
	buf := newInertKVBuffer("f", 1000)
	stg := buf.node("n")
	ctx := t.Context()

	require.NoError(t, stg.Set(ctx, "k", `1`, 0))
	_, found, err := stg.Get(ctx, "k")
	require.NoError(t, err)
	assert.False(t, found, "even a same-message staged write reads back absent")

	keys, err := stg.Keys(ctx, "")
	require.NoError(t, err)
	assert.Empty(t, keys)

	stg.commit()
	assert.Empty(t, buf.mutations())
}

// fakeScriptRuntime registers under the default language so a function node
// resolves to it; each OnMessage call is scripted by the test.
type fakeScriptRuntime struct {
	onMessage func(ctx context.Context, msg models.Msg, kv NodeKV) ([][]models.Msg, error)
}

func (f *fakeScriptRuntime) Name() string       { return DefaultScriptLanguage }
func (f *fakeScriptRuntime) Check(string) error { return nil }
func (f *fakeScriptRuntime) New(string, int) (ScriptInstance, error) {
	return &fakeScriptInstance{onMessage: f.onMessage}, nil
}

type fakeScriptInstance struct {
	onMessage func(ctx context.Context, msg models.Msg, kv NodeKV) ([][]models.Msg, error)
}

func (f *fakeScriptInstance) OnMessage(ctx context.Context, msg models.Msg, _ any, kv NodeKV, _ ConsoleSink) ([][]models.Msg, error) {
	return f.onMessage(ctx, msg, kv)
}
func (f *fakeScriptInstance) Close() {}

// A message that errors after staging a write persists nothing, and a later
// message in the same batch does not see the discarded write; a succeeding
// message's write reaches the batch.
func TestRun_StagedWritesFollowTheMessageOutcome(t *testing.T) {
	rt := &fakeScriptRuntime{onMessage: func(ctx context.Context, msg models.Msg, kv NodeKV) ([][]models.Msg, error) {
		require.NoError(t, kv.Set(ctx, "seen:"+msg.Key, `true`, 0))
		if msg.Key == "poison" {
			return nil, &ScriptError{Kind: ScriptErrorRuntime, Message: "boom"}
		}
		_, foundPoison, err := kv.Get(ctx, "seen:poison")
		require.NoError(t, err)
		assert.False(t, foundPoison, "the errored message's staged write must be invisible")
		return [][]models.Msg{{msg}}, nil
	}}

	registry := NewScriptRegistry()
	registry.Register(rt)
	runner, err := NewRunner(flow.Flow{
		ID:      "f",
		Enabled: true,
		Nodes: []flow.Node{
			{ID: "fn", Type: "function", Config: &flow.FunctionConfig{OnMessage: "unused"}},
		},
	}, Options{Scripts: registry})
	require.NoError(t, err)
	defer runner.Close()

	batch, err := runner.Run(t.Context(), []models.Msg{
		{ID: "1", Key: "poison", Payload: json.RawMessage(`{}`)},
		{ID: "2", Key: "ok", Payload: json.RawMessage(`{}`)},
	})
	require.NoError(t, err)

	assert.Equal(t, []models.KVMutation{{NodeID: "fn", Key: "seen:ok", Value: `true`}}, batch.KVMutations)
}

// A script that writes KV and then intentionally drops the message (returns
// nothing) still merges: only an error discards staging.
func TestRun_IntentionalDropStillCommitsStaging(t *testing.T) {
	rt := &fakeScriptRuntime{onMessage: func(ctx context.Context, msg models.Msg, kv NodeKV) ([][]models.Msg, error) {
		require.NoError(t, kv.Set(ctx, "seen", `true`, 0))
		return nil, nil
	}}

	registry := NewScriptRegistry()
	registry.Register(rt)
	runner, err := NewRunner(flow.Flow{
		ID:      "f",
		Enabled: true,
		Nodes:   []flow.Node{{ID: "fn", Type: "function", Config: &flow.FunctionConfig{OnMessage: "unused"}}},
	}, Options{Scripts: registry})
	require.NoError(t, err)
	defer runner.Close()

	batch, err := runner.Run(t.Context(), []models.Msg{{ID: "1", Key: "k", Payload: json.RawMessage(`{}`)}})
	require.NoError(t, err)
	assert.Equal(t, []models.KVMutation{{NodeID: "fn", Key: "seen", Value: `true`}}, batch.KVMutations)
}

// Within one Run, message 2 sees message 1's write once message 1 succeeded.
func TestRun_LaterMessageSeesEarlierCommittedWrite(t *testing.T) {
	secondSawWrite := false
	rt := &fakeScriptRuntime{onMessage: func(ctx context.Context, msg models.Msg, kv NodeKV) ([][]models.Msg, error) {
		found, err := kv.Has(ctx, "seen")
		require.NoError(t, err)
		if msg.Key == "b" {
			secondSawWrite = found
		}
		require.NoError(t, kv.Set(ctx, "seen", `true`, 0))
		return nil, nil
	}}

	registry := NewScriptRegistry()
	registry.Register(rt)
	runner, err := NewRunner(flow.Flow{
		ID:      "f",
		Enabled: true,
		Nodes:   []flow.Node{{ID: "fn", Type: "function", Config: &flow.FunctionConfig{OnMessage: "unused"}}},
	}, Options{Scripts: registry})
	require.NoError(t, err)
	defer runner.Close()

	batch, err := runner.Run(t.Context(), []models.Msg{
		{ID: "1", Key: "a", Payload: json.RawMessage(`{}`)},
		{ID: "2", Key: "b", Payload: json.RawMessage(`{}`)},
	})
	require.NoError(t, err)

	assert.True(t, secondSawWrite, "message 2 must read message 1's merged write")
	assert.Len(t, batch.KVMutations, 1)
}

func TestResetProcessorsDropsInstances(t *testing.T) {
	instances := 0
	rt := &fakeScriptRuntime{onMessage: func(context.Context, models.Msg, NodeKV) ([][]models.Msg, error) {
		return nil, nil
	}}
	counting := &countingScriptRuntime{fakeScriptRuntime: rt, news: &instances}

	registry := NewScriptRegistry()
	registry.Register(counting)
	runner, err := NewRunner(flow.Flow{
		ID:      "f",
		Enabled: true,
		Nodes:   []flow.Node{{ID: "fn", Type: "function", Config: &flow.FunctionConfig{OnMessage: "unused"}}},
	}, Options{Scripts: registry})
	require.NoError(t, err)
	defer runner.Close()

	_, err = runner.Run(t.Context(), []models.Msg{{ID: "1", Key: "a", Payload: json.RawMessage(`{}`)}})
	require.NoError(t, err)
	assert.Equal(t, 1, instances)

	runner.resetProcessors()

	_, err = runner.Run(t.Context(), []models.Msg{{ID: "2", Key: "b", Payload: json.RawMessage(`{}`)}})
	require.NoError(t, err)
	assert.Equal(t, 2, instances, "a reset drops the instance and its state; the next message builds fresh")
}

type countingScriptRuntime struct {
	*fakeScriptRuntime
	news *int
}

func (c *countingScriptRuntime) New(src string, outputs int) (ScriptInstance, error) {
	*c.news++
	return c.fakeScriptRuntime.New(src, outputs)
}
