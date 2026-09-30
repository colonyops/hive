package runtime_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/desktop/internal/app/data/models"
	"github.com/colonyops/hive/cmd/desktop/internal/app/data/queries"
	"github.com/colonyops/hive/cmd/desktop/internal/app/data/stores"
	"github.com/colonyops/hive/cmd/desktop/internal/app/flow"
	"github.com/colonyops/hive/cmd/desktop/internal/app/runtime"
	"github.com/colonyops/hive/cmd/desktop/internal/app/runtime/js"
)

const dedupScript = `
	const k = JSON.stringify([msg.SourceKind, msg.SourceScope, msg.Key])
	if (kv.has(k)) return null
	kv.set(k, true)
	return msg
`

func kvTestRunner(t *testing.T, db *queries.DB, script string) *runtime.Runner {
	t.Helper()
	registry := runtime.NewScriptRegistry()
	registry.Register(js.New(runtime.NewScriptPool(0)))
	runner, err := runtime.NewRunner(flow.Flow{
		ID:      "f",
		Enabled: true,
		Nodes: []flow.Node{
			{ID: "dedup", Type: "function", Config: &flow.FunctionConfig{OnMessage: script}},
			{ID: "n", Type: "notify", Config: &flow.NotifyConfig{Title: "Ping"}},
		},
		Wires: []flow.Wire{{From: "dedup", To: "n"}},
	}, runtime.Options{Scripts: registry, KV: stores.New(db, stores.Options{}).NodeKV})
	require.NoError(t, err)
	t.Cleanup(runner.Close)
	return runner
}

func openKVTestDB(t *testing.T) *queries.DB {
	t.Helper()
	db, err := queries.Open(t.Context(), t.TempDir(), queries.DefaultOpenOptions())
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func notifyOutputs(batch models.CommitBatch) int {
	count := 0
	for _, out := range batch.Outputs {
		if out.Sink.Kind == models.SinkKindNotify {
			count++
		}
	}
	return count
}

func kvMsg(id string) models.Msg {
	m := models.Msg{ID: id, Key: "item-1", Payload: json.RawMessage(`{"title":"hi"}`)}
	m.SourceKind = "github"
	m.SourceScope = "notifications"
	return m
}

func TestRun_DedupFunctionNotifiesExactlyOnce(t *testing.T) {
	db := openKVTestDB(t)
	ctx := t.Context()
	runner := kvTestRunner(t, db, dedupScript)

	first, err := runner.Run(ctx, []models.Msg{kvMsg("1")})
	require.NoError(t, err)
	assert.Equal(t, 1, notifyOutputs(first), "a new item notifies exactly once")
	require.NoError(t, stores.New(db, stores.Options{}).EventLog.Commit(ctx, first))

	second, err := runner.Run(ctx, []models.Msg{kvMsg("2")})
	require.NoError(t, err)
	assert.Zero(t, notifyOutputs(second), "the same item stays quiet")
	assert.Empty(t, second.KVMutations)
}

func TestRun_KVSetThenThrowPersistsNothing(t *testing.T) {
	db := openKVTestDB(t)
	ctx := t.Context()
	runner := kvTestRunner(t, db, `
		const k = JSON.stringify([msg.SourceKind, msg.SourceScope, msg.Key])
		kv.set(k, true)
		throw new Error('boom')
	`)

	batch, err := runner.Run(ctx, []models.Msg{kvMsg("1")})
	require.NoError(t, err)
	assert.Empty(t, batch.KVMutations)
	assert.Zero(t, notifyOutputs(batch))
	require.NoError(t, stores.New(db, stores.Options{}).EventLog.Commit(ctx, batch))

	var rows int
	require.NoError(t, db.Conn().QueryRowContext(ctx, `SELECT COUNT(*) FROM node_kv`).Scan(&rows))
	assert.Zero(t, rows)

	dedup := kvTestRunner(t, db, dedupScript)
	after, err := dedup.Run(ctx, []models.Msg{kvMsg("2")})
	require.NoError(t, err)
	assert.Equal(t, 1, notifyOutputs(after), "the item is still unseen — dedup was not poisoned")
}

func TestRun_DedupMemoryIsDurableAcrossRunners(t *testing.T) {
	db := openKVTestDB(t)
	ctx := t.Context()

	first, err := kvTestRunner(t, db, dedupScript).Run(ctx, []models.Msg{kvMsg("1")})
	require.NoError(t, err)
	require.NoError(t, stores.New(db, stores.Options{}).EventLog.Commit(ctx, first))

	rebuilt := kvTestRunner(t, db, dedupScript)
	second, err := rebuilt.Run(ctx, []models.Msg{kvMsg("2")})
	require.NoError(t, err)
	assert.Zero(t, notifyOutputs(second), "the seen-set survives a runner rebuild, unlike state")
}

func TestRunReplay_IsFullyInert(t *testing.T) {
	db := openKVTestDB(t)
	ctx := t.Context()

	// Every item is already marked seen; a live run would suppress them all.
	seenKey := `["github","notifications","item-1"]`
	require.NoError(t, stores.New(db, stores.Options{}).NodeKV.Set(ctx, "f", "dedup", seenKey, `true`, 0))

	registry := runtime.NewScriptRegistry()
	registry.Register(js.New(runtime.NewScriptPool(0)))
	runner, err := runtime.NewRunner(flow.Flow{
		ID:      "f",
		Enabled: true,
		Nodes: []flow.Node{
			{ID: "dedup", Type: "function", Config: &flow.FunctionConfig{OnMessage: dedupScript}},
			{ID: "inbox", Type: "feed", Config: &flow.FeedConfig{}},
		},
		Wires: []flow.Wire{{From: "dedup", To: "inbox"}},
	}, runtime.Options{Scripts: registry, KV: stores.New(db, stores.Options{}).NodeKV})
	require.NoError(t, err)
	t.Cleanup(runner.Close)

	batch, err := runner.RunReplay(ctx, []models.Msg{kvMsg("1")})
	require.NoError(t, err)

	feedClaims := 0
	for _, out := range batch.Outputs {
		if out.Sink.Kind == models.SinkKindFeed {
			feedClaims++
		}
	}
	assert.Equal(t, 1, feedClaims, "replay recomputes full membership regardless of dedup history")
	assert.Empty(t, batch.KVMutations, "replay never writes KV")

	var rows int
	require.NoError(t, db.Conn().QueryRowContext(ctx, `SELECT COUNT(*) FROM node_kv`).Scan(&rows))
	assert.Equal(t, 1, rows, "the durable seen-set is untouched")

	live, err := runner.Run(ctx, []models.Msg{kvMsg("2")})
	require.NoError(t, err)
	for _, out := range live.Outputs {
		require.NotEqual(t, models.SinkKindFeed, out.Sink.Kind, "the live run still honors the durable seen-set")
	}
}
