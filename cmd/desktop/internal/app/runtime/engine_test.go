package runtime_test

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/desktop/internal/app/activity"
	"github.com/colonyops/hive/cmd/desktop/internal/app/data/models"
	"github.com/colonyops/hive/cmd/desktop/internal/app/data/queries"
	"github.com/colonyops/hive/cmd/desktop/internal/app/data/stores"
	"github.com/colonyops/hive/cmd/desktop/internal/app/events"
	"github.com/colonyops/hive/cmd/desktop/internal/app/flow"
	"github.com/colonyops/hive/cmd/desktop/internal/app/runtime"
	ghsource "github.com/colonyops/hive/cmd/desktop/internal/app/sources/github"
)

// The engine is tested against a real SQLite store, like everything else that
// touches the commit protocol. Its whole job is what happens between a read
// and a commit, and a fake store would only assert that the calls were made.

func openTestStore(t *testing.T) *queries.DB {
	t.Helper()
	db, err := queries.Open(t.Context(), t.TempDir(), queries.DefaultOpenOptions())
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func testStores(db *queries.DB) *stores.Stores {
	return stores.New(db, stores.Options{})
}

// flowSet is a mutable stand-in for the flow store, so a test can change what
// the engine sees between reloads.
type flowSet struct {
	mu    sync.Mutex
	flows []flow.Flow
}

func (f *flowSet) List() []flow.Flow {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]flow.Flow(nil), f.flows...)
}

func (f *flowSet) set(flows ...flow.Flow) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.flows = flows
}

// triageFlow is a sources.github into one feed, the smallest flow that commits
// membership.
func triageFlow(id string, enabled bool) flow.Flow {
	return flow.Flow{
		ID:      id,
		Name:    id,
		Enabled: enabled,
		Nodes: []flow.Node{
			{ID: "src", Type: ghsource.Descriptor.Type, Config: flow.NewSourceConfig(ghsource.Descriptor.Type, &ghsource.Config{Credential: "github/octocat", Kind: "search", Query: "is:open"})},
			{ID: "inbox", Type: "feed", Config: &flow.FeedConfig{}},
		},
		Wires: []flow.Wire{{From: "src", To: "inbox"}},
	}
}

type observation struct {
	externalID string
	title      string
}

// ingest writes an item through the production source boundary and returns the
// resulting log offset, exactly as the producer does.
func ingest(t *testing.T, db *queries.DB, flowID string, obs ...observation) int64 {
	t.Helper()
	var last int64
	for _, o := range obs {
		payload, err := json.Marshal(map[string]string{"title": o.title, "repo": "acme/app"})
		require.NoError(t, err)
		result, err := testStores(db).InboxItems.IngestObservation(t.Context(), passthroughClassifier{}, stores.IngestObservationParams{
			ProfileID: flowID,
			Topic:     "source:" + flowID + "/src",
			Current: models.Observation{
				ExternalID: o.externalID, Title: o.title, URL: "https://example.invalid/" + o.externalID,
				SourceKind: "github", SourceScope: "search",
				ObservedAt: time.Now().UnixMilli(), Payload: payload,
			},
		})
		require.NoError(t, err)
		if result.Wrote {
			last = result.Offset
		}
	}
	return last
}

// snapshotSource appends the authoritative "this is everything the source
// currently has" event a producer tick ends with. Replay recomputes membership
// from it, so a flow with no snapshot in its log has nothing to replay.
func snapshotSource(t *testing.T, db *queries.DB, flowID string, obs ...observation) {
	t.Helper()
	items := make([]models.SnapshotItem, 0, len(obs))
	for _, o := range obs {
		payload, err := json.Marshal(map[string]string{"title": o.title, "repo": "acme/app"})
		require.NoError(t, err)
		items = append(items, models.SnapshotItem{Key: o.externalID, Payload: payload})
	}
	_, err := testStores(db).EventLog.AppendSnapshot(t.Context(), "source:"+flowID+"/src", "github", "search", items)
	require.NoError(t, err)
}

// splitFlow is a source → function → feed, where the function splits one
// source message into per-entity feed items under keys it mints. It is the
// shape that turns a single grafana metrics result into one durable item per
// series (issue hay-kot/hive-desktop#117).
func splitFlow(id, script string) flow.Flow {
	return flow.Flow{
		ID:      id,
		Name:    id,
		Enabled: true,
		Nodes: []flow.Node{
			{ID: "src", Type: ghsource.Descriptor.Type, Config: flow.NewSourceConfig(ghsource.Descriptor.Type, &ghsource.Config{Credential: "github/octocat", Kind: "search", Query: "is:open"})},
			{ID: "fn", Type: "function", Config: &flow.FunctionConfig{OnMessage: script}},
			{ID: "inbox", Type: "feed", Config: &flow.FeedConfig{}},
		},
		Wires: []flow.Wire{{From: "src", To: "fn"}, {From: "fn", To: "inbox"}},
	}
}

// snapshotOne appends a single-item snapshot — the shape a grafana metrics
// source emits, where one node maps to one message whose payload carries the
// whole query result for a downstream function node to split.
func snapshotOne(t *testing.T, db *queries.DB, flowID, key, payload string) {
	t.Helper()
	_, err := testStores(db).EventLog.AppendSnapshot(t.Context(), "source:"+flowID+"/src", "github", "search",
		[]models.SnapshotItem{{Key: key, Payload: json.RawMessage(payload)}})
	require.NoError(t, err)
}

type passthroughClassifier struct{}

func (passthroughClassifier) Classify(previous *models.Observation, current models.Observation) models.Classification {
	kind := "observed"
	if previous != nil {
		kind = "updated"
	}
	return models.Classification{
		Kind: kind, Transition: models.TransitionNone, Attention: models.AttentionActivity,
		Lifecycle: models.LifecycleActive, Summary: current.Title,
	}
}

// committed is a latch a test waits on instead of sleeping, so a slow machine
// makes the test slower rather than flaky.
type committed struct {
	ch chan struct{}
}

func newCommitted() *committed { return &committed{ch: make(chan struct{}, 64)} }

func (c *committed) signal() {
	select {
	case c.ch <- struct{}{}:
	default:
	}
}

func (c *committed) wait(t *testing.T) {
	t.Helper()
	select {
	case <-c.ch:
	case <-time.After(5 * time.Second):
		t.Fatal("the engine did not report a commit within 5s")
	}
}

// Guard records because the engine writes on its loop goroutine while tests
// read through require.Eventually.
type fakeFlowRecorder struct {
	mu    sync.Mutex
	flows []string
}

func (r *fakeFlowRecorder) Record(_ context.Context, e activity.Event) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.flows = append(r.flows, e.Source)
}

func (r *fakeFlowRecorder) sources() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.flows...)
}

func startEngine(t *testing.T, db *queries.DB, flows *flowSet, onCommit func()) *runtime.Engine {
	t.Helper()
	st := testStores(db)
	bus := events.New(zerolog.Nop())
	t.Cleanup(bus.Close)
	if onCommit != nil {
		cancel := events.Subscribe(t.Context(), bus, "test", events.Coalesce(), func(context.Context, events.InboxUpdated) {
			onCommit()
		})
		t.Cleanup(cancel)
	}
	engine := runtime.NewEngine(runtime.EngineOptions{
		Log:     st.EventLog,
		Items:   st.InboxItems,
		Commits: st.EventLog,
		KV:      st.NodeKV,
		Flows:   flows,
		Scripts: testScripts(),
		Logger:  zerolog.Nop(),
		Events:  bus,
	})
	require.NoError(t, engine.Start(t.Context()))
	t.Cleanup(engine.Stop)
	return engine
}

// Start installs flows synchronously, so a wake-up that arrives immediately
// afterwards always finds a runner. That is what removed the readiness latch
// the browser runtime needed a test hook for.
func TestEngineRoutesAnAppendIntoAFeed(t *testing.T) {
	t.Parallel()

	db := openTestStore(t)
	flows := &flowSet{}
	flows.set(triageFlow("triage", true))

	done := newCommitted()
	engine := startEngine(t, db, flows, done.signal)

	ingest(t, db, "triage", observation{"pr-1", "First"}, observation{"pr-2", "Second"})
	engine.Wake()
	done.wait(t)

	items, err := testStores(db).InboxItems.ListByFeed(t.Context(), "triage", "triage/inbox", 10)
	require.NoError(t, err)
	require.Len(t, items, 2)

	runs, err := testStores(db).NodeRuns.List(t.Context(), "triage", 10)
	require.NoError(t, err)
	require.NotEmpty(t, runs)
}

// A wake-up that lands while a pass is already running must not be lost. It is
// a latch rather than a queue, so the pass in flight is followed by exactly
// one more — enough to see the append, without piling up.
func TestEngineWakeIsLevelTriggered(t *testing.T) {
	t.Parallel()

	db := openTestStore(t)
	flows := &flowSet{}
	flows.set(triageFlow("triage", true))

	done := newCommitted()
	engine := startEngine(t, db, flows, done.signal)

	for i := range 25 {
		ingest(t, db, "triage", observation{externalID: "pr-" + string(rune('a'+i)), title: "Item"})
		engine.Wake()
	}

	require.Eventually(t, func() bool {
		items, err := testStores(db).InboxItems.ListByFeed(t.Context(), "triage", "triage/inbox", 100)
		return err == nil && len(items) == 25
	}, 10*time.Second, 20*time.Millisecond, "every append must eventually be routed")
}

// A disabled flow is not merely skipped: its runner is dropped, so nothing it
// used to hold survives a toggle.
func TestEngineDropsARunnerWhenItsFlowIsDisabled(t *testing.T) {
	t.Parallel()

	db := openTestStore(t)
	flows := &flowSet{}
	flows.set(triageFlow("triage", true))

	done := newCommitted()
	engine := startEngine(t, db, flows, done.signal)

	ingest(t, db, "triage", observation{"pr-1", "First"})
	engine.Wake()
	done.wait(t)

	flows.set(triageFlow("triage", false))
	engine.Reload()

	ingest(t, db, "triage", observation{"pr-2", "Second"})
	engine.Wake()

	require.Never(t, func() bool {
		items, err := testStores(db).InboxItems.ListByFeed(t.Context(), "triage", "triage/inbox", 10)
		return err == nil && len(items) > 1
	}, time.Second, 50*time.Millisecond, "a disabled flow must stop routing")
}

// The replay protocol is what makes a flow edit change what a feed contains
// without re-running the actions already run for those items. Removing the
// feed node must therefore take its claims with it.
func TestEngineReplayReconcilesMembershipOnReload(t *testing.T) {
	t.Parallel()

	db := openTestStore(t)
	flows := &flowSet{}
	flows.set(triageFlow("triage", true))

	done := newCommitted()
	engine := startEngine(t, db, flows, done.signal)

	ingest(t, db, "triage", observation{"pr-1", "First"})
	snapshotSource(t, db, "triage", observation{"pr-1", "First"})
	engine.Wake()
	done.wait(t)

	require.Eventually(t, func() bool {
		items, err := testStores(db).InboxItems.ListByFeed(t.Context(), "triage", "triage/inbox", 10)
		return err == nil && len(items) == 1
	}, 5*time.Second, 20*time.Millisecond)

	// Same source, different feed node id. The old feed's claims belong to
	// structure that no longer exists.
	replacement := triageFlow("triage", true)
	replacement.Nodes[1].ID = "other"
	replacement.Wires = []flow.Wire{{From: "src", To: "other"}}
	flows.set(replacement)
	engine.Reload()

	require.Eventually(t, func() bool {
		stale, err := testStores(db).InboxItems.ListByFeed(t.Context(), "triage", "triage/inbox", 10)
		if err != nil || len(stale) != 0 {
			return false
		}
		moved, err := testStores(db).InboxItems.ListByFeed(t.Context(), "triage", "triage/other", 10)
		return err == nil && len(moved) == 1
	}, 5*time.Second, 20*time.Millisecond, "replay recomputes membership from the source's latest snapshot")
}

// A flow that cannot be built keeps its predecessor in service. Silently
// taking a working flow offline for an authoring mistake would lose messages
// the last-known-good version handles fine.
func TestEngineKeepsTheLastGoodRunnerWhenAReloadFails(t *testing.T) {
	t.Parallel()

	db := openTestStore(t)
	flows := &flowSet{}
	flows.set(triageFlow("triage", true))

	done := newCommitted()
	rec := &fakeFlowRecorder{}
	st := testStores(db)
	bus := events.New(zerolog.Nop())
	t.Cleanup(bus.Close)
	cancel := events.Subscribe(t.Context(), bus, "test", events.Coalesce(), func(context.Context, events.InboxUpdated) {
		done.signal()
	})
	t.Cleanup(cancel)
	engine := runtime.NewEngine(runtime.EngineOptions{
		Log:      st.EventLog,
		Items:    st.InboxItems,
		Commits:  st.EventLog,
		KV:       st.NodeKV,
		Flows:    flows,
		Scripts:  testScripts(),
		Logger:   zerolog.Nop(),
		Events:   bus,
		Recorder: rec,
	})
	require.NoError(t, engine.Start(t.Context()))
	t.Cleanup(engine.Stop)

	ingest(t, db, "triage", observation{"pr-1", "First"})
	engine.Wake()
	done.wait(t)

	broken := triageFlow("triage", true)
	broken.Wires = append(broken.Wires, flow.Wire{From: "inbox", To: "src"}) // a cycle
	flows.set(broken)
	engine.Reload()

	ingest(t, db, "triage", observation{"pr-2", "Second"})
	engine.Wake()

	require.Eventually(t, func() bool {
		items, err := testStores(db).InboxItems.ListByFeed(t.Context(), "triage", "triage/inbox", 10)
		return err == nil && len(items) == 2
	}, 5*time.Second, 20*time.Millisecond, "the previous runner must keep routing")
	require.Contains(t, rec.sources(), "triage", "and the failure must be reported, or the app silently runs an older graph")
}

// A function node splitting one source message into per-entity feed items is
// the first-party answer to issue hay-kot/hive-desktop#117: the metrics source stays one message
// per node, and the split — the keys and payloads — lives in author JavaScript.
// Each series becomes its own durable item, and a series leaving the query
// drops that item alone, because the split messages inherit the source
// snapshot's reconciliation scope.
func TestEngineSplitsOneMessageIntoPerEntityFeedItemsWithLifecycle(t *testing.T) {
	t.Parallel()

	db := openTestStore(t)
	flows := &flowSet{}
	// The source payload carries a list of "series"; the function mints one item
	// per series, keyed by name, preserving Topic so the feed reconciles them
	// under the source's snapshot.
	flows.set(splitFlow("ignores", `
return msg.Payload.result.map(function (s) {
  return { ...msg, Key: s.name, Payload: { title: s.name + " ignored", cluster: s.cluster } };
});
`))

	engine := startEngine(t, db, flows, nil)

	snapshotOne(t, db, "ignores", "metrics-node",
		`{"result":[{"name":"apps","cluster":"prod"},{"name":"infra","cluster":"prod"}]}`)
	engine.Wake()

	require.Eventually(t, func() bool {
		items, err := testStores(db).InboxItems.ListByFeed(t.Context(), "ignores", "ignores/inbox", 10)
		return err == nil && len(items) == 2
	}, 5*time.Second, 20*time.Millisecond, "each series becomes its own feed item")

	// One series leaves the query. The next snapshot restates the current set, so
	// the departed item's membership claim is reconciled away — presence-based
	// lifecycle, without an absence confirmer.
	snapshotOne(t, db, "ignores", "metrics-node", `{"result":[{"name":"apps","cluster":"prod"}]}`)
	engine.Wake()

	require.Eventually(t, func() bool {
		items, err := testStores(db).InboxItems.ListByFeed(t.Context(), "ignores", "ignores/inbox", 10)
		if err != nil || len(items) != 1 {
			return false
		}
		return items[0].ExternalID == "apps"
	}, 5*time.Second, 20*time.Millisecond, "the departed series drops from the feed; the surviving one stays")
}

// Stop is the single teardown path and has to survive being called on an
// engine that was never started — App.Close runs whether Start did or not.
func TestEngineStopWithoutStart(t *testing.T) {
	t.Parallel()

	db := openTestStore(t)
	st := testStores(db)
	engine := runtime.NewEngine(runtime.EngineOptions{
		Log: st.EventLog, Items: st.InboxItems, Commits: st.EventLog, KV: st.NodeKV,
		Flows: &flowSet{}, Scripts: testScripts(), Logger: zerolog.Nop(),
	})
	require.NotPanics(t, engine.Stop)
	require.NotPanics(t, engine.Stop)
}

// Installing a flow reconciles node KV inside the activation transaction:
// ids still present as function nodes keep their rows, everything else under
// the flow is cleared.
func TestEngineInstallReconcilesNodeKV(t *testing.T) {
	db := openTestStore(t)
	ctx := t.Context()

	require.NoError(t, testStores(db).NodeKV.Set(ctx, "triage", "fn", "seen", `1`, 0))
	require.NoError(t, testStores(db).NodeKV.Set(ctx, "triage", "ghost", "seen", `1`, 0))

	f := triageFlow("triage", true)
	f.Nodes = append(f.Nodes, flow.Node{ID: "fn", Type: "function", Config: &flow.FunctionConfig{OnMessage: "return msg"}})
	f.Wires = append(f.Wires, flow.Wire{From: "src", To: "fn"})

	flows := &flowSet{}
	flows.set(f)
	startEngine(t, db, flows, nil)

	_, found, err := testStores(db).NodeKV.Get(ctx, "triage", "fn", "seen", 1)
	require.NoError(t, err)
	require.True(t, found, "a live function node's KV survives install")
	_, found, err = testStores(db).NodeKV.Get(ctx, "triage", "ghost", "seen", 1)
	require.NoError(t, err)
	require.False(t, found, "a node id no longer in the flow loses its KV on install")
}
