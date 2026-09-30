package runtime_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/desktop/internal/app/data/models"
	"github.com/colonyops/hive/cmd/desktop/internal/app/flow"
	"github.com/colonyops/hive/cmd/desktop/internal/app/runtime"
	"github.com/colonyops/hive/cmd/desktop/internal/app/runtime/js"
	whsource "github.com/colonyops/hive/cmd/desktop/internal/app/sources/webhook"
)

func dryRunScripts() *runtime.ScriptRegistry {
	registry := runtime.NewScriptRegistry()
	registry.Register(js.New(runtime.NewScriptPool(0)))
	return registry
}

// fanOutFlow is the shape ADR function-node-per-entity-feed-items introduced and the shape a dry run exists to
// verify: a source feeding a function node that splits one message into one
// feed item per entity.
func fanOutFlow(onMessage string) flow.Flow {
	return flow.Flow{
		ID: "f",
		Nodes: []flow.Node{
			{ID: "src", Type: "sources.webhook", Config: flow.NewSourceConfig(whsource.Descriptor.Type, &whsource.Config{Path: "hook"})},
			{ID: "fn", Type: "function", Config: &flow.FunctionConfig{OnMessage: onMessage}},
			{ID: "inbox", Type: "feed", Config: &flow.FeedConfig{}},
		},
		Wires: []flow.Wire{{From: "src", To: "fn"}, {From: "fn", To: "inbox"}},
	}
}

func traceOf(t *testing.T, result runtime.DryRunResult, nodeID string) runtime.NodeTrace {
	t.Helper()
	for _, node := range result.Nodes {
		if node.NodeID == nodeID {
			return node
		}
	}
	t.Fatalf("no trace for node %q; got %+v", nodeID, result.Nodes)
	return runtime.NodeTrace{}
}

// The motivating case: an array return fans one message out into several, and
// the emitted messages are readable without deploying the flow.
func TestDryRunReportsWhatEachNodeEmitted(t *testing.T) {
	t.Parallel()

	runner, err := runtime.NewRunner(fanOutFlow(`
		return msg.Payload.series.map(function (s) {
			return { ...msg, Key: s.name, Payload: { title: s.name } };
		});
	`), runtime.Options{Scripts: dryRunScripts()})
	require.NoError(t, err)
	defer runner.Close()

	result, err := runner.DryRun(t.Context(), "src", []models.Msg{
		testMsg("1", `{"series":[{"name":"a"},{"name":"b"}]}`),
	})
	require.NoError(t, err)

	fn := traceOf(t, result, "fn")
	assert.True(t, fn.OK)
	assert.Equal(t, 1, fn.In)
	require.Len(t, fn.Emitted, 1)
	assert.Equal(t, 0, fn.Emitted[0].Port)
	require.Len(t, fn.Emitted[0].Messages, 2)
	assert.Equal(t, "a", fn.Emitted[0].Messages[0].Key)
	assert.Equal(t, "b", fn.Emitted[0].Messages[1].Key)

	// The terminal has no outputs of its own; what it received and what it
	// would have committed are the two halves of its answer.
	inbox := traceOf(t, result, "inbox")
	assert.Empty(t, inbox.Emitted)
	require.Len(t, inbox.Received, 2)
	require.Len(t, result.Outputs, 2)
	assert.Equal(t, models.SinkKindFeed, result.Outputs[0].Sink.Kind)
	assert.Equal(t, "f/inbox", result.Outputs[0].Sink.TargetID)
}

// An empty snapshot is the "clean poll clears the feed" case. It routes no
// items, so nothing downstream runs, but it must still declare the
// reconciliation scope that empties the feed — otherwise a dry run would report
// "nothing happened" for a poll that in fact removes every row.
func TestDryRunReportsReconciliationForAnEmptySnapshot(t *testing.T) {
	t.Parallel()

	runner, err := runtime.NewRunner(fanOutFlow("return msg;"), runtime.Options{Scripts: dryRunScripts()})
	require.NoError(t, err)
	defer runner.Close()

	result, err := runner.DryRun(t.Context(), "src", []models.Msg{{
		ID: "1", Topic: "source:f/src", Snapshot: []models.SnapshotItem{},
	}})
	require.NoError(t, err)

	assert.Empty(t, result.Outputs)
	require.Len(t, result.FeedSnapshots, 1)
	assert.Equal(t, "f/inbox", result.FeedSnapshots[0].FeedID)
	assert.Equal(t, "source:f/src", result.FeedSnapshots[0].SourceTopic)
}

// Injecting straight at a function node is what isolates it: the source in
// front of it never runs, and the captured payload reaches the script.
func TestDryRunInjectsAtAnArbitraryNode(t *testing.T) {
	t.Parallel()

	runner, err := runtime.NewRunner(fanOutFlow(`msg.Payload.seen = true; return msg;`),
		runtime.Options{Scripts: dryRunScripts()})
	require.NoError(t, err)
	defer runner.Close()

	result, err := runner.DryRun(t.Context(), "fn", []models.Msg{testMsg("1", `{"n":1}`)})
	require.NoError(t, err)

	for _, node := range result.Nodes {
		assert.NotEqual(t, "src", node.NodeID, "the source must not have run")
	}
	fn := traceOf(t, result, "fn")
	require.Len(t, fn.Emitted, 1)
	require.Len(t, fn.Emitted[0].Messages, 1)
	assert.JSONEq(t, `{"n":1,"seen":true}`, string(fn.Emitted[0].Messages[0].Payload))
}

// A port with nothing wired to it is still something the node emitted. A dry
// run that hid those would make "what does this node output" depend on the
// graph downstream, which is exactly the question it is asked instead of.
func TestDryRunReportsAnUnwiredPort(t *testing.T) {
	t.Parallel()

	runner, err := runtime.NewRunner(flow.Flow{
		ID: "f",
		Nodes: []flow.Node{
			{ID: "fn", Type: "function", Config: &flow.FunctionConfig{
				OnMessage: `return [null, { ...msg, Key: "rejected" }];`, OutputsN: 2,
			}},
			{ID: "inbox", Type: "feed", Config: &flow.FeedConfig{}},
		},
		Wires: []flow.Wire{{From: "fn", Out: 0, To: "inbox"}},
	}, runtime.Options{Scripts: dryRunScripts()})
	require.NoError(t, err)
	defer runner.Close()

	result, err := runner.DryRun(t.Context(), "fn", []models.Msg{testMsg("1", `{}`)})
	require.NoError(t, err)

	fn := traceOf(t, result, "fn")
	require.Len(t, fn.Emitted, 1)
	assert.Equal(t, 1, fn.Emitted[0].Port)
	require.Len(t, fn.Emitted[0].Messages, 1)
	assert.Equal(t, "rejected", fn.Emitted[0].Messages[0].Key)
	assert.Equal(t, 1, fn.Dropped, "the message ends here: port 1 has no wire")
	assert.Empty(t, result.Outputs)
}

// A returned null is a drop the run reports rather than swallows.
func TestDryRunCountsDropsAndErrors(t *testing.T) {
	t.Parallel()

	t.Run("a null return", func(t *testing.T) {
		t.Parallel()
		runner, err := runtime.NewRunner(fanOutFlow("return null;"), runtime.Options{Scripts: dryRunScripts()})
		require.NoError(t, err)
		defer runner.Close()

		result, err := runner.DryRun(t.Context(), "fn", []models.Msg{testMsg("1", `{}`)})
		require.NoError(t, err)

		fn := traceOf(t, result, "fn")
		assert.True(t, fn.OK, "a drop is not a failure")
		assert.Equal(t, 1, fn.Dropped)
		assert.Empty(t, fn.Emitted)
		assert.Nil(t, fn.Error)
	})

	t.Run("a thrown error carries its position", func(t *testing.T) {
		t.Parallel()
		runner, err := runtime.NewRunner(fanOutFlow("\nthrow new Error('boom');"), runtime.Options{Scripts: dryRunScripts()})
		require.NoError(t, err)
		defer runner.Close()

		result, err := runner.DryRun(t.Context(), "fn", []models.Msg{testMsg("1", `{}`)})
		require.NoError(t, err)

		fn := traceOf(t, result, "fn")
		assert.False(t, fn.OK)
		require.NotNil(t, fn.Error)
		assert.Equal(t, runtime.ScriptErrorRuntime, fn.Error.Kind)
		assert.Contains(t, fn.Error.Message, "boom")
		assert.Equal(t, 2, fn.Error.Line, "the author's own line, not the wrapper's")
	})
}

// console output is a dry run's only window into a script's own reasoning, and
// it must not leak into a live run — which is what the nil sink guarantees.
func TestDryRunCollectsConsoleOutput(t *testing.T) {
	t.Parallel()

	runner, err := runtime.NewRunner(fanOutFlow(`
		console.log("saw", msg.Key);
		console.warn({ n: 1 });
		return msg;
	`), runtime.Options{Scripts: dryRunScripts()})
	require.NoError(t, err)
	defer runner.Close()

	result, err := runner.DryRun(t.Context(), "fn", []models.Msg{testMsg("1", `{}`)})
	require.NoError(t, err)

	fn := traceOf(t, result, "fn")
	require.Len(t, fn.Console, 2)
	assert.Equal(t, runtime.ConsoleLine{Level: "log", Text: "saw k1"}, fn.Console[0])
	assert.Equal(t, runtime.ConsoleLine{Level: "warn", Text: `{"n":1}`}, fn.Console[1])
	assert.False(t, result.ConsoleTruncated)
}

// A dry run's kv is the seed and nothing else: a live run's durable store is
// never read, and what the script writes comes back as a mutation instead of
// being applied.
func TestDryRunSandboxesKV(t *testing.T) {
	t.Parallel()

	runner, err := runtime.NewRunner(fanOutFlow(`
		if (kv.has("seen:" + msg.Key)) return null;
		kv.set("seen:" + msg.Key, { at: 1 });
		return msg;
	`), runtime.Options{
		Scripts: dryRunScripts(),
		KV:      runtime.MemoryKV{"fn": {"seen:k2": `{"at":0}`}},
	})
	require.NoError(t, err)
	defer runner.Close()

	result, err := runner.DryRun(t.Context(), "fn", []models.Msg{testMsg("1", `{}`), testMsg("2", `{}`)})
	require.NoError(t, err)

	fn := traceOf(t, result, "fn")
	require.Len(t, fn.Emitted, 1)
	require.Len(t, fn.Emitted[0].Messages, 1)
	assert.Equal(t, "k1", fn.Emitted[0].Messages[0].Key, "k2 was seeded as already seen")

	require.Len(t, result.KVMutations, 1)
	assert.Equal(t, "fn", result.KVMutations[0].NodeID)
	assert.Equal(t, "seen:k1", result.KVMutations[0].Key)
	assert.JSONEq(t, `{"at":1}`, result.KVMutations[0].Value)
}

// Two runs of the same input over the same runner would not be deterministic if
// node state carried between them, which is why a dry run gets its own runner.
// Within one run, state is expected to carry — that is what makes a two-message
// batch behave like a live tick.
func TestDryRunIsRepeatableOnAFreshRunner(t *testing.T) {
	t.Parallel()

	const script = `state.n = (state.n || 0) + 1; return { ...msg, Payload: { n: state.n } };`

	run := func() json.RawMessage {
		runner, err := runtime.NewRunner(fanOutFlow(script), runtime.Options{Scripts: dryRunScripts()})
		require.NoError(t, err)
		defer runner.Close()

		result, err := runner.DryRun(t.Context(), "fn", []models.Msg{testMsg("1", `{}`)})
		require.NoError(t, err)
		return traceOf(t, result, "fn").Emitted[0].Messages[0].Payload
	}

	assert.JSONEq(t, `{"n":1}`, string(run()))
	assert.JSONEq(t, `{"n":1}`, string(run()))
}

func TestDryRunRejectsAnUnknownEntryNode(t *testing.T) {
	t.Parallel()

	runner, err := runtime.NewRunner(fanOutFlow("return msg;"), runtime.Options{Scripts: dryRunScripts()})
	require.NoError(t, err)
	defer runner.Close()

	_, err = runner.DryRun(t.Context(), "nope", []models.Msg{testMsg("1", `{}`)})
	require.ErrorContains(t, err, `no node "nope"`)
}
