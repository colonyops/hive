package runtime_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/desktop/internal/app/data/models"
	"github.com/colonyops/hive/cmd/desktop/internal/app/flow"
	"github.com/colonyops/hive/cmd/desktop/internal/app/runtime"
	whsource "github.com/colonyops/hive/cmd/desktop/internal/app/sources/webhook"
)

func testMsg(id, payload string) models.Msg {
	return models.Msg{
		ID: id, Key: "k" + id, Topic: "source:f/src", Ts: 1,
		Payload: json.RawMessage(payload), SourceKind: "webhook", SourceScope: "hook",
	}
}

// sourceFlow is a sources.webhook wired to one terminal, the smallest shape
// that exercises routing.
func sourceFlow(terminal flow.Node, wires ...flow.Wire) flow.Flow {
	nodes := []flow.Node{
		{ID: "src", Type: "sources.webhook", Config: flow.NewSourceConfig(whsource.Descriptor.Type, &whsource.Config{Path: "hook"})},
		terminal,
	}
	if len(wires) == 0 {
		wires = []flow.Wire{{From: "src", To: terminal.ID}}
	}
	return flow.Flow{ID: "f", Nodes: nodes, Wires: wires}
}

func TestRunRejectsAFlowItCannotExecute(t *testing.T) {
	t.Parallel()

	t.Run("a cycle", func(t *testing.T) {
		t.Parallel()
		_, err := runtime.NewRunner(flow.Flow{
			ID: "loop",
			Nodes: []flow.Node{
				{ID: "a", Type: "function", Config: &flow.FunctionConfig{OnMessage: "return msg"}},
				{ID: "b", Type: "function", Config: &flow.FunctionConfig{OnMessage: "return msg"}},
			},
			Wires: []flow.Wire{{From: "a", To: "b"}, {From: "b", To: "a"}},
		}, runtime.Options{})
		require.ErrorContains(t, err, "not a DAG")
	})

	// A function node with no runtime registered is a flow that would fail on
	// its first message instead of at deploy. Reporting it here is what makes
	// the failure land where a user can see it.
	t.Run("a function node with no script runtime", func(t *testing.T) {
		t.Parallel()
		_, err := runtime.NewRunner(flow.Flow{
			ID:    "f",
			Nodes: []flow.Node{{ID: "fn", Type: "function", Config: &flow.FunctionConfig{OnMessage: "return msg"}}},
		}, runtime.Options{})
		require.ErrorContains(t, err, "script runtime")
	})

	t.Run("a node type the engine has no behavior for", func(t *testing.T) {
		t.Parallel()
		_, err := runtime.NewRunner(flow.Flow{
			ID:    "f",
			Nodes: []flow.Node{{ID: "x", Type: "not-a-node-type", Config: &flow.FeedConfig{}}},
		}, runtime.Options{})
		require.ErrorContains(t, err, "cannot execute type")
	})
}

// The engine returns what should be committed and does not commit it. That is
// what lets a dry-run and a live tick be the same code path.
func TestRunProducesACommitForTheWholeBatch(t *testing.T) {
	t.Parallel()

	runner, err := runtime.NewRunner(sourceFlow(flow.Node{ID: "inbox", Type: "feed", Config: &flow.FeedConfig{}}), runtime.Options{})
	require.NoError(t, err)
	defer runner.Close()

	batch := []models.Msg{testMsg("4", `{}`), testMsg("9", `{}`), testMsg("6", `{}`)}
	got, err := runner.Run(t.Context(), batch)
	require.NoError(t, err)

	require.Equal(t, "f", got.Consumer)
	require.Equal(t, int64(9), got.UpToOffset, "the highest offset in the batch, not the last one seen")
	require.Len(t, got.Outputs, 3)
	require.Empty(t, got.Discards)
}

// Nothing may fall out of the accounting: every input message ends as an
// output, a discard, or an errored discard. This is what the commit
// protocol's ability to advance the offset rests on.
func TestEveryMessageIsAccountedFor(t *testing.T) {
	t.Parallel()

	runner, err := runtime.NewRunner(flow.Flow{
		ID: "f",
		Nodes: []flow.Node{
			{ID: "src", Type: "sources.webhook", Config: flow.NewSourceConfig(whsource.Descriptor.Type, &whsource.Config{Path: "hook"})},
			{ID: "keep", Type: "github-filter", Config: &flow.GithubFilterConfig{Repos: []string{"acme/*"}}},
			{ID: "inbox", Type: "feed", Config: &flow.FeedConfig{}},
		},
		Wires: []flow.Wire{{From: "src", To: "keep"}, {From: "keep", Out: 0, To: "inbox"}},
	}, runtime.Options{})
	require.NoError(t, err)
	defer runner.Close()

	batch := []models.Msg{
		testMsg("1", `{"repo":"acme/app"}`),
		testMsg("2", `{"repo":"other/app"}`),
	}
	batch = append(batch, models.Msg{ID: "3", Topic: "source:elsewhere/src"})

	got, err := runner.Run(t.Context(), batch)
	require.NoError(t, err)
	require.Len(t, got.Outputs, 1, "only the matching item reaches the feed")
	require.Len(t, got.Discards, 2, "the filtered item and the foreign topic are both recorded")
	// Routing runs before any node does, so a message that matched no entry is
	// recorded first.
	require.Equal(t, models.Discard{MsgID: "3", NodeID: runtime.UnroutedNodeID}, got.Discards[0])
	require.Equal(t, models.Discard{MsgID: "2", NodeID: "keep"}, got.Discards[1])
}

// A node that outlives its timeout is respawned rather than reused: whatever
// state it was interrupted in cannot be trusted, so the next message starts
// from a clean one.
func TestATimedOutFunctionNodeIsRespawnedWithFreshState(t *testing.T) {
	t.Parallel()

	src := `
state.n = (state.n ?? 0) + 1;
if (state.n === 2) { while (true) {} }
msg.Payload = { n: state.n };
return msg;
`
	runner, err := runtime.NewRunner(flow.Flow{
		ID: "f",
		Nodes: []flow.Node{
			{ID: "src", Type: "sources.webhook", Config: flow.NewSourceConfig(whsource.Descriptor.Type, &whsource.Config{Path: "hook"})},
			{ID: "fn", Type: "function", Config: &flow.FunctionConfig{OnMessage: src, Timeout: flow.Duration(200 * time.Millisecond)}},
			{ID: "out", Type: "notify", Config: &flow.NotifyConfig{Title: "t"}},
		},
		Wires: []flow.Wire{{From: "src", To: "fn"}, {From: "fn", To: "out"}},
	}, runtime.Options{Scripts: testScripts()})
	require.NoError(t, err)
	defer runner.Close()

	got, err := runner.Run(t.Context(), []models.Msg{
		testMsg("1", `{}`), testMsg("2", `{}`), testMsg("3", `{}`),
	})
	require.NoError(t, err)

	require.Len(t, got.Outputs, 2, "the timed-out message is discarded; the other two get through")
	require.JSONEq(t, `{"n":1}`, string(got.Outputs[0].Payload))
	require.JSONEq(t, `{"n":1}`, string(got.Outputs[1].Payload), "the respawned instance counts from one again")

	fnRun := nodeRun(t, got, "fn")
	require.False(t, fnRun.OK)
	require.Equal(t, 3, fnRun.InCount)
	require.Equal(t, 1, fnRun.DropCount)
	require.NotEmpty(t, fnRun.Err)
}

// State is the reason a Runner is reused across batches rather than rebuilt:
// a flow pumped page by page has to behave as one continuous run.
func TestFunctionStateSurvivesAcrossBatches(t *testing.T) {
	t.Parallel()

	runner, err := runtime.NewRunner(flow.Flow{
		ID: "f",
		Nodes: []flow.Node{
			{ID: "src", Type: "sources.webhook", Config: flow.NewSourceConfig(whsource.Descriptor.Type, &whsource.Config{Path: "hook"})},
			{ID: "fn", Type: "function", Config: &flow.FunctionConfig{OnMessage: "state.n = (state.n ?? 0) + 1; msg.Payload = {n: state.n}; return msg"}},
			{ID: "out", Type: "notify", Config: &flow.NotifyConfig{Title: "t"}},
		},
		Wires: []flow.Wire{{From: "src", To: "fn"}, {From: "fn", To: "out"}},
	}, runtime.Options{Scripts: testScripts()})
	require.NoError(t, err)
	defer runner.Close()

	first, err := runner.Run(t.Context(), []models.Msg{testMsg("1", `{}`)})
	require.NoError(t, err)
	second, err := runner.Run(t.Context(), []models.Msg{testMsg("2", `{}`)})
	require.NoError(t, err)

	require.JSONEq(t, `{"n":1}`, string(first.Outputs[0].Payload))
	require.JSONEq(t, `{"n":2}`, string(second.Outputs[0].Payload), "a second page continues the same instance")
}

// A function node may mint new Keys to split one message into many feed items,
// but every split message must stay inside the source snapshot's
// reconciliation scope: its Topic and SnapshotID have to match the declared
// FeedSnapshot, or the commit cannot reconcile it and a departed item never
// leaves the feed. This pins that contract — and, by extension, why an author
// must not rewrite Topic.
func TestFunctionNodeSplitInheritsSnapshotScope(t *testing.T) {
	t.Parallel()

	split := `return msg.Payload.result.map(function (s) {
  return { ...msg, Key: s.name, Payload: { title: s.name } };
});`
	runner, err := runtime.NewRunner(flow.Flow{
		ID: "f",
		Nodes: []flow.Node{
			{ID: "src", Type: "sources.webhook", Config: flow.NewSourceConfig(whsource.Descriptor.Type, &whsource.Config{Path: "hook"})},
			{ID: "fn", Type: "function", Config: &flow.FunctionConfig{OnMessage: split}},
			{ID: "inbox", Type: "feed", Config: &flow.FeedConfig{}},
		},
		Wires: []flow.Wire{{From: "src", To: "fn"}, {From: "fn", To: "inbox"}},
	}, runtime.Options{Scripts: testScripts()})
	require.NoError(t, err)
	defer runner.Close()

	got, err := runner.Run(t.Context(), []models.Msg{{
		ID: "5", Topic: "source:f/src", Ts: 1, SourceKind: "grafana", SourceScope: "grafana/prod",
		Snapshot: []models.SnapshotItem{{Key: "node", Payload: json.RawMessage(`{"result":[{"name":"a"},{"name":"b"}]}`)}},
	}})
	require.NoError(t, err)

	require.Len(t, got.FeedSnapshots, 1, "the feed reconciles under one snapshot scope")
	scope := got.FeedSnapshots[0]
	require.Len(t, got.Outputs, 2, "one feed output per series")

	keys := make([]string, 0, len(got.Outputs))
	for _, out := range got.Outputs {
		require.Equal(t, models.SinkKindFeed, out.Sink.Kind)
		require.Equal(t, scope.SourceTopic, out.SourceTopic, "Topic is preserved, so the split output reconciles under the source scope")
		require.Equal(t, scope.SnapshotID, out.SnapshotID, "each split output carries the snapshot id it was expanded from")
		require.Equal(t, "grafana", out.SourceKind)
		keys = append(keys, out.Key)
	}
	require.ElementsMatch(t, []string{"a", "b"}, keys, "each series is keyed by the value the script minted")
}

func nodeRun(t *testing.T, batch models.CommitBatch, nodeID string) models.NodeRun {
	t.Helper()
	for _, run := range batch.NodeRuns {
		if run.NodeID == nodeID {
			return run
		}
	}
	t.Fatalf("no node run recorded for %q", nodeID)
	return models.NodeRun{}
}
