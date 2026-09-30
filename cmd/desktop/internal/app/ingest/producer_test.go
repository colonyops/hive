package ingest

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/desktop/internal/app/activity"
	"github.com/colonyops/hive/cmd/desktop/internal/app/data/models"
	"github.com/colonyops/hive/cmd/desktop/internal/app/data/queries"
	"github.com/colonyops/hive/cmd/desktop/internal/app/data/stores"
	"github.com/colonyops/hive/cmd/desktop/internal/app/sources/connector"
)

// fakeSource drives Producer.Tick with canned batches, one per call to
// Produce, so tests can assert append behavior without a network fetch.
type fakeSource struct {
	mu      sync.Mutex
	batches [][]Msg
	calls   int
	err     error
}

func (f *fakeSource) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

func (f *fakeSource) Produce(_ context.Context, emit func(Msg) error) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return f.err
	}
	var batch []Msg
	if f.calls < len(f.batches) {
		batch = f.batches[f.calls]
	}
	f.calls++
	for _, msg := range batch {
		if err := emit(msg); err != nil {
			return err
		}
	}
	return nil
}

// stubSources is the Sources seam: a fixed set of instances, plus an
// optional prefetch hook for the tests that assert the batched pre-pass runs.
type stubSources struct {
	instances []connector.Instance
	prefetch  func(context.Context, []connector.Instance) error
}

func (s stubSources) PullInstances(context.Context) []connector.Instance { return s.instances }

func (s stubSources) Prefetch(ctx context.Context, instances []connector.Instance) error {
	if s.prefetch == nil {
		return nil
	}
	return s.prefetch(ctx, instances)
}

// pullInstance is one pull-mode instance addressed "<flowID>/<nodeID>", with
// no declared capabilities — the producer must then classify it generically.
func pullInstance(flowID, nodeID string, pull connector.PullSource) connector.Instance {
	return connector.Instance{
		Type: "sources.test",
		Node: connector.Node{FlowID: flowID, NodeID: nodeID},
		Metadata: connector.Metadata{
			ProfileID:  flowID + "/" + nodeID,
			SourceKind: "generic",
			Policy:     models.ResurfacePolicyStateChanges,
		},
		Pull: pull,
	}
}

// sourcesOf builds the seam from a set of "<flowID>/<nodeID>" ids. The ids
// are split rather than passed as pairs so the tests read the way the topics
// they assert on do.
func sourcesOf(byID map[string]connector.PullSource) stubSources {
	out := stubSources{}
	ids := make([]string, 0, len(byID))
	for id := range byID {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		flowID, nodeID, found := strings.Cut(id, "/")
		if !found {
			flowID, nodeID = "", id
		}
		out.instances = append(out.instances, pullInstance(flowID, nodeID, byID[id]))
	}
	return out
}

// fakeAppender records IngestObservation calls without touching disk, for tests
// that only care whether Producer invokes its database dependency.
type fakeAppender struct {
	mu        sync.Mutex
	nextOff   int64
	calls     []models.Msg
	snapshots int
}

func (a *fakeAppender) IngestObservation(_ context.Context, _ models.Classifier, p stores.IngestObservationParams) (stores.IngestResult, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.nextOff++
	a.calls = append(a.calls, models.Msg{Topic: p.Topic, Key: p.Current.ExternalID, Payload: p.Current.Payload})
	return stores.IngestResult{Wrote: true, Offset: a.nextOff}, nil
}

func (a *fakeAppender) ListActiveKeys(context.Context, stores.SourceIdentity) ([]string, error) {
	return nil, nil
}

func (a *fakeAppender) Payload(context.Context, string, string) ([]byte, error) {
	return nil, nil
}
func (a *fakeAppender) Delete(context.Context, string, string) error { return nil }

func (a *fakeAppender) AppendSnapshot(_ context.Context, _, _, _ string, _ []models.SnapshotItem) (int64, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.nextOff++
	a.snapshots++
	return a.nextOff, nil
}

func (a *fakeAppender) callCount() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return len(a.calls)
}

type activityRecorder struct {
	mu     sync.Mutex
	events []activity.Event
}

func (r *activityRecorder) Record(_ context.Context, event activity.Event) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, event)
}

func openTestPipelineDB(t *testing.T) *queries.DB {
	t.Helper()
	db, err := queries.Open(t.Context(), t.TempDir(), queries.DefaultOpenOptions())
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	return db
}

type notifierFunc func(offset int64)

func (f notifierFunc) PublishLogAppended(offset int64) { f(offset) }

func newTestProducer(db *queries.DB, sources Sources, interval time.Duration, onAppended func(int64), logger zerolog.Logger) *Producer {
	st := stores.New(db, stores.Options{})
	var notifier LogAppendNotifier
	if onAppended != nil {
		notifier = notifierFunc(onAppended)
	}
	return NewProducer(ProducerDeps{
		Ingester:  st.InboxItems,
		Snapshots: st.EventLog,
		Heads:     st.SourceHeads,
		Sources:   sources,
		Interval:  interval,
		Notifier:  notifier,
		Logger:    logger,
	})
}

func readFrom(db *queries.DB, ctx context.Context, offset int64, limit int) ([]models.Msg, int64, error) {
	return stores.New(db, stores.Options{}).EventLog.ReadFrom(ctx, offset, limit)
}

func TestProducer_Tick_AppendsMonotonicOffsets(t *testing.T) {
	t.Parallel()

	db := openTestPipelineDB(t)
	src := &fakeSource{batches: [][]Msg{
		{
			{Topic: "source:flow/s1", Key: "a", Payload: []byte(`{"v":1}`)},
			{Topic: "source:flow/s1", Key: "b", Payload: []byte(`{"v":2}`)},
		},
	}}

	var appendedOffsets []int64
	producer := newTestProducer(db, sourcesOf(map[string]connector.PullSource{"flow/s1": src}), time.Hour, func(offset int64) {
		appendedOffsets = append(appendedOffsets, offset)
	}, zerolog.Nop())

	producer.Tick(t.Context())

	require.Len(t, appendedOffsets, 1, "one wake-up per tick that appended something")

	msgs, next, err := readFrom(db, t.Context(), 0, 10)
	require.NoError(t, err)
	require.Len(t, msgs, 3)
	assert.Equal(t, "a", msgs[0].Key)
	assert.Equal(t, "b", msgs[1].Key)
	assert.Len(t, msgs[2].Snapshot, 2)
	assert.Equal(t, "source:flow/s1", msgs[0].Topic)
	assert.Equal(t, next, appendedOffsets[0], "onAppended reports the last offset appended this tick")

	// Offsets are strictly increasing.
	var lastOffset int64
	for i, msg := range msgs {
		var offset int64
		_, err := fmt.Sscanf(msg.ID, "%d", &offset)
		require.NoError(t, err)
		if i > 0 {
			assert.Greater(t, offset, lastOffset)
		}
		lastOffset = offset
	}
}

func TestProducer_Tick_SummaryReportsSourcesAppendedFailed(t *testing.T) {
	t.Parallel()

	db := openTestPipelineDB(t)
	ok := &fakeSource{batches: [][]Msg{{{Topic: "source:flow/s1", Key: "a", Payload: []byte(`{"v":1}`)}}}}
	bad := &fakeSource{err: fmt.Errorf("fetch failed")}

	producer := newTestProducer(db, sourcesOf(map[string]connector.PullSource{
		"flow/s1": ok, "flow/s2": bad,
	}), time.Hour, func(int64) {}, zerolog.Nop())

	summary := producer.Tick(t.Context())
	assert.Equal(t, 2, summary.Sources)
	assert.Equal(t, 1, summary.Failed, "a failing source is counted, not fatal")
	assert.Positive(t, summary.Appended, "the healthy source still appended")
}

func TestProducer_Tick_EmptySnapshot_WakesConsumer(t *testing.T) {
	t.Parallel()

	db := openTestPipelineDB(t)
	src := &fakeSource{} // no batches configured: Produce emits nothing

	woke := false
	producer := newTestProducer(db, sourcesOf(map[string]connector.PullSource{"flow/s1": src}), time.Hour, func(int64) {
		woke = true
	}, zerolog.Nop())

	producer.Tick(t.Context())
	assert.True(t, woke, "an empty successful snapshot must wake the frontend for reconciliation")

	msgs, _, err := readFrom(db, t.Context(), 0, 10)
	require.NoError(t, err)
	require.Len(t, msgs, 1)
	assert.Empty(t, msgs[0].Snapshot)
}

func TestProducer_Tick_SourceErrorDoesNotBlockOthers(t *testing.T) {
	t.Parallel()

	db := openTestPipelineDB(t)
	failing := &fakeSource{err: fmt.Errorf("boom")}
	ok := &fakeSource{batches: [][]Msg{{{Topic: "source:flow/ok", Key: "x", Payload: []byte(`{}`)}}}}

	var appendedOffsets []int64
	recorder := &activityRecorder{}
	producer := newTestProducer(db, sourcesOf(map[string]connector.PullSource{
		"flow/failing": failing,
		"flow/ok":      ok,
	}), time.Hour, func(offset int64) {
		appendedOffsets = append(appendedOffsets, offset)
	}, zerolog.Nop())
	producer.SetRecorder(recorder)

	producer.Tick(t.Context())

	require.Len(t, appendedOffsets, 1, "the healthy source's append still wakes the frontend")
	msgs, _, err := readFrom(db, t.Context(), 0, 10)
	require.NoError(t, err)
	require.Len(t, msgs, 2)
	assert.Equal(t, "x", msgs[0].Key)
	assert.Len(t, msgs[1].Snapshot, 1)
	require.Len(t, recorder.events, 1, "successful refreshes must not crowd the activity log")
	assert.Equal(t, activity.CategoryRefresh, recorder.events[0].Category)
	assert.Equal(t, activity.SeverityError, recorder.events[0].Severity)
	assert.Equal(t, "Refresh failed for flow/failing", recorder.events[0].Title)
}

type gatedSource struct {
	inFlight *atomic.Int32
	gate     <-chan struct{}
}

func (g gatedSource) Produce(ctx context.Context, _ func(Msg) error) error {
	g.inFlight.Add(1)
	defer g.inFlight.Add(-1)
	select {
	case <-g.gate:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func TestProducer_Tick_DrainsSourcesConcurrentlyUpToTheCap(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		db := openTestPipelineDB(t)
		var inFlight atomic.Int32
		gate := make(chan struct{})
		byID := map[string]connector.PullSource{}
		for i := range drainConcurrency + 2 {
			byID[fmt.Sprintf("flow/s%d", i)] = gatedSource{inFlight: &inFlight, gate: gate}
		}
		var woke atomic.Int64
		producer := newTestProducer(db, sourcesOf(byID), time.Hour, func(offset int64) { woke.Store(offset) }, zerolog.Nop())

		done := make(chan TickSummary, 1)
		go func() { done <- producer.Tick(t.Context()) }()

		synctest.Wait()
		assert.EqualValues(t, drainConcurrency, inFlight.Load())

		close(gate)
		summary := <-done
		assert.Equal(t, drainConcurrency+2, summary.Sources)
		assert.Zero(t, summary.Failed)

		_, next, err := readFrom(db, t.Context(), 0, 100)
		require.NoError(t, err)
		assert.Equal(t, next, woke.Load(), "the wake-up names the highest offset appended, whichever source finished last")
	})
}

type gatedWrites struct {
	*fakeAppender
	inFlight atomic.Int32
	release  chan struct{}
}

func (g *gatedWrites) IngestObservation(ctx context.Context, c models.Classifier, p stores.IngestObservationParams) (stores.IngestResult, error) {
	g.inFlight.Add(1)
	defer g.inFlight.Add(-1)
	<-g.release
	return g.fakeAppender.IngestObservation(ctx, c, p)
}

func (g *gatedWrites) AppendSnapshot(ctx context.Context, topic, kind, scope string, items []models.SnapshotItem) (int64, error) {
	g.inFlight.Add(1)
	defer g.inFlight.Add(-1)
	<-g.release
	return g.fakeAppender.AppendSnapshot(ctx, topic, kind, scope, items)
}

func TestProducer_Tick_ConcurrentDrainsWriteOneAtATime(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		byID := map[string]connector.PullSource{}
		for i := range drainConcurrency {
			id := fmt.Sprintf("flow/s%d", i)
			byID[id] = &fakeSource{batches: [][]Msg{{{Topic: "source:" + id, Key: "a", Payload: []byte(`{}`)}}}}
		}
		store := &gatedWrites{fakeAppender: &fakeAppender{}, release: make(chan struct{})}
		producer := NewProducer(ProducerDeps{
			Ingester:  store,
			Snapshots: store,
			Heads:     store,
			Sources:   sourcesOf(byID),
			Interval:  time.Hour,
			Logger:    zerolog.Nop(),
		})

		done := make(chan TickSummary, 1)
		go func() { done <- producer.Tick(t.Context()) }()

		// One item and one snapshot per source.
		for range 2 * drainConcurrency {
			synctest.Wait()
			assert.EqualValues(t, 1, store.inFlight.Load())
			store.release <- struct{}{}
		}

		summary := <-done
		assert.Zero(t, summary.Failed)
	})
}

// TestProducer_DedupesUnchangedPayload verifies durable deduplication: an
// unchanged payload for the same topic/key is not re-appended on the next
// tick, but a changed payload is.
func TestProducer_DedupesUnchangedPayload(t *testing.T) {
	t.Parallel()

	db := openTestPipelineDB(t)
	src := &fakeSource{batches: [][]Msg{
		{{Topic: "source:flow/s1", Key: "a", Payload: []byte(`{"v":1}`)}}, // tick 1: new
		{{Topic: "source:flow/s1", Key: "a", Payload: []byte(`{"v":1}`)}}, // tick 2: unchanged
		{{Topic: "source:flow/s1", Key: "a", Payload: []byte(`{"v":2}`)}}, // tick 3: changed
	}}

	var wakeCount int
	producer := newTestProducer(db, sourcesOf(map[string]connector.PullSource{"flow/s1": src}), time.Hour, func(int64) {
		wakeCount++
	}, zerolog.Nop())

	producer.Tick(t.Context())
	producer.Tick(t.Context())
	producer.Tick(t.Context())

	msgs, _, err := readFrom(db, t.Context(), 0, 10)
	require.NoError(t, err)
	require.Len(t, msgs, 5, "every successful source tick appends its authoritative snapshot")
	assert.Equal(t, []byte(`{"v":1}`), []byte(msgs[0].Payload))
	assert.Equal(t, []byte(`{"v":2}`), []byte(msgs[3].Payload))
	assert.Len(t, msgs[1].Snapshot, 1)
	assert.Len(t, msgs[2].Snapshot, 1)
	assert.Len(t, msgs[4].Snapshot, 1)
	assert.Equal(t, 3, wakeCount)
}

// TestProducer_EmptyKeyNeverDeduped verifies that messages without a stable
// item identity are not stored in source_head, so repeated empty-key emits
// still append distinct events.
func TestProducer_EmptyKeyNeverDeduped(t *testing.T) {
	t.Parallel()

	db := openTestPipelineDB(t)
	src := &fakeSource{batches: [][]Msg{
		{{Topic: "source:flow/s1", Key: "", Payload: []byte(`{"v":1}`)}},
		{{Topic: "source:flow/s1", Key: "", Payload: []byte(`{"v":1}`)}},
	}}

	producer := newTestProducer(db, sourcesOf(map[string]connector.PullSource{"flow/s1": src}), time.Hour, nil, zerolog.Nop())

	producer.Tick(t.Context())
	producer.Tick(t.Context())
	msgs, _, err := readFrom(db, t.Context(), 0, 10)
	require.NoError(t, err)
	assert.Len(t, msgs, 2, "empty-key messages have no inbox identity; snapshots remain authoritative")
}

func TestProducer_DeduplicationSurvivesRestart(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	firstDB, err := queries.Open(t.Context(), dir, queries.DefaultOpenOptions())
	require.NoError(t, err)

	first := newTestProducer(firstDB, sourcesOf(map[string]connector.PullSource{
		"flow/s1": &fakeSource{batches: [][]Msg{{{Topic: "source:flow/s1", Key: "a", Payload: []byte(`{"v":1}`)}}}},
	}), time.Hour, nil, zerolog.Nop())
	first.Tick(t.Context())
	require.NoError(t, firstDB.Close())

	secondDB, err := queries.Open(t.Context(), dir, queries.DefaultOpenOptions())
	require.NoError(t, err)
	t.Cleanup(func() { _ = secondDB.Close() })
	second := newTestProducer(secondDB, sourcesOf(map[string]connector.PullSource{
		"flow/s1": &fakeSource{batches: [][]Msg{{{Topic: "source:flow/s1", Key: "a", Payload: []byte(`{"v":1}`)}}}},
	}), time.Hour, nil, zerolog.Nop())
	second.Tick(t.Context())

	msgs, _, err := readFrom(secondDB, t.Context(), 0, 10)
	require.NoError(t, err)
	assert.Len(t, msgs, 3, "a restarted producer retains source heads while still appending snapshots")
}

// Resolving no sources is not an error any more — a source whose connector
// is unavailable is skipped by the resolver, so the producer only ever sees
// the instances that built. A tick over none of them must still append
// nothing and, crucially, not wake the engine: a wake-up with no new rows
// makes every consumer re-read the log for nothing.
func TestProducer_NoSourcesAppendsNothingAndDoesNotWake(t *testing.T) {
	t.Parallel()

	appender := &fakeAppender{}
	woke := false
	producer := NewProducer(ProducerDeps{
		Ingester:  appender,
		Snapshots: appender,
		Heads:     appender,
		Sources:   stubSources{},
		Interval:  time.Hour,
		Notifier:  notifierFunc(func(int64) { woke = true }),
		Logger:    zerolog.Nop(),
	})

	producer.Tick(t.Context())
	assert.Equal(t, 0, appender.callCount())
	assert.False(t, woke)
}

// A prefetch failure is advisory: the batched pre-pass is an optimisation, so
// a connector whose batch request fails must still have its sources drained
// individually rather than losing the tick.
func TestProducer_PrefetchFailureStillDrains(t *testing.T) {
	t.Parallel()

	db := openTestPipelineDB(t)
	src := &fakeSource{batches: [][]Msg{{{Topic: "source:flow/s1", Key: "a", Payload: []byte(`{"v":1}`)}}}}
	sources := sourcesOf(map[string]connector.PullSource{"flow/s1": src})
	sources.prefetch = func(context.Context, []connector.Instance) error {
		return fmt.Errorf("batch request failed")
	}
	producer := newTestProducer(db, sources, time.Hour, nil, zerolog.Nop())

	producer.Tick(t.Context())

	msgs, _, err := readFrom(db, t.Context(), 0, 10)
	require.NoError(t, err)
	require.Len(t, msgs, 2, "the item and its snapshot should still be appended")
	assert.Equal(t, "a", msgs[0].Key)
}

func TestProducer_StartStop(t *testing.T) {
	// The poll loop is real-time timed (a ticker), so a wall-clock version of
	// this — sleep 50ms and hope the 10ms ticker fired — flakes under CI
	// scheduling load (wakeCount can still be 0). synctest runs the loop on a
	// fake clock, so advancing time is deterministic: the ticker fires exactly
	// as scheduled and the lifecycle assertion never races the scheduler.
	synctest.Test(t, func(t *testing.T) {
		db := openTestPipelineDB(t)
		src := &fakeSource{batches: [][]Msg{
			{{Topic: "source:flow/s1", Key: "a", Payload: []byte(`{"v":1}`)}},
		}}
		var wakeCount int
		var mu sync.Mutex
		producer := newTestProducer(db, sourcesOf(map[string]connector.PullSource{"flow/s1": src}), 10*time.Millisecond, func(int64) {
			mu.Lock()
			wakeCount++
			mu.Unlock()
		}, zerolog.Nop())

		producer.Start(t.Context())
		time.Sleep(50 * time.Millisecond) // fake time: the ticker fires deterministically
		synctest.Wait()                   // let the in-flight tick's append settle
		producer.Stop()
		producer.Stop() // idempotent

		mu.Lock()
		got := wakeCount
		mu.Unlock()
		assert.GreaterOrEqual(t, got, 1, "at least one tick should have run and appended")
	})
}

// pullInstanceEvery is one instance that asks not to be drained more often than
// interval — the cadence a command source declares when hourly is enough.
func pullInstanceEvery(flowID, nodeID string, interval time.Duration, pull connector.PullSource) connector.Instance {
	instance := pullInstance(flowID, nodeID, pull)
	instance.MinInterval = interval
	return instance
}

func TestProducer_MinInterval_SkipsUntilDue(t *testing.T) {
	t.Parallel()

	db := openTestPipelineDB(t)
	hourly := &fakeSource{}
	everyTick := &fakeSource{}
	sources := stubSources{instances: []connector.Instance{
		pullInstanceEvery("flow", "hourly", time.Hour, hourly),
		pullInstance("flow", "every-tick", everyTick),
	}}

	now := time.Date(2026, 8, 5, 9, 0, 0, 0, time.UTC)
	producer := newTestProducer(db, sources, time.Minute, nil, zerolog.Nop())
	producer.now = func() time.Time { return now }

	producer.Tick(t.Context())
	now = now.Add(5 * time.Minute)
	producer.Tick(t.Context())
	now = now.Add(56 * time.Minute)
	producer.Tick(t.Context())

	assert.Equal(t, 2, hourly.callCount(), "the hourly source runs on the first tick and again once its floor expires")
	assert.Equal(t, 3, everyTick.callCount(), "a source with no floor is unaffected")
}

// A skipped source is not a drained-empty one: nothing is produced, so no
// snapshot is appended and nothing it owns is reconciled away.
func TestProducer_MinInterval_SkippingAppendsNoSnapshot(t *testing.T) {
	t.Parallel()

	db := openTestPipelineDB(t)
	src := &fakeSource{batches: [][]Msg{{{Topic: "source:flow/s1", Key: "a", Payload: []byte(`{"v":1}`)}}}}
	sources := stubSources{instances: []connector.Instance{pullInstanceEvery("flow", "s1", time.Hour, src)}}

	now := time.Date(2026, 8, 5, 9, 0, 0, 0, time.UTC)
	producer := newTestProducer(db, sources, time.Minute, nil, zerolog.Nop())
	producer.now = func() time.Time { return now }

	producer.Tick(t.Context())
	before, _, err := readFrom(db, t.Context(), 0, 10)
	require.NoError(t, err)

	now = now.Add(time.Minute)
	producer.Tick(t.Context())

	after, _, err := readFrom(db, t.Context(), 0, 10)
	require.NoError(t, err)
	assert.Len(t, after, len(before), "a skipped tick writes nothing at all")
}

func TestProducer_Refresh_IgnoresTheCadenceFloor(t *testing.T) {
	t.Parallel()

	db := openTestPipelineDB(t)
	src := &fakeSource{}
	sources := stubSources{instances: []connector.Instance{pullInstanceEvery("flow", "hourly", time.Hour, src)}}

	now := time.Date(2026, 8, 5, 9, 0, 0, 0, time.UTC)
	producer := newTestProducer(db, sources, time.Minute, nil, zerolog.Nop())
	producer.now = func() time.Time { return now }

	producer.Tick(t.Context())
	now = now.Add(time.Minute)
	producer.Tick(t.Context())
	producer.Refresh(t.Context())

	assert.Equal(t, 2, src.callCount(), "a user who asked for a refresh gets one")
}

// A broken command fails every tick. Recording each one would bury a day of
// real events under hundreds of copies of the same line.
func TestProducer_RepeatedFailureIsAnnouncedOnceAnHour(t *testing.T) {
	t.Parallel()

	db := openTestPipelineDB(t)
	broken := &fakeSource{err: fmt.Errorf("exit status 1: gcx: not logged in")}

	now := time.Date(2026, 8, 5, 9, 0, 0, 0, time.UTC)
	recorder := &activityRecorder{}
	producer := newTestProducer(db, sourcesOf(map[string]connector.PullSource{"flow/s1": broken}), time.Minute, nil, zerolog.Nop())
	producer.now = func() time.Time { return now }
	producer.SetRecorder(recorder)

	for range 10 {
		producer.Tick(t.Context())
		now = now.Add(5 * time.Minute)
	}

	require.Len(t, recorder.events, 1, "the first failure is news; the next nine are the same condition")

	now = now.Add(time.Hour)
	producer.Tick(t.Context())
	assert.Len(t, recorder.events, 2, "a failure that outlives the interval is announced again")
}

// A source that recovers and breaks again is two conditions, so the second
// failure is announced immediately rather than waiting out the interval.
func TestProducer_FailureAfterRecoveryIsAnnouncedImmediately(t *testing.T) {
	t.Parallel()

	db := openTestPipelineDB(t)
	flaky := &failingOnceSource{}

	now := time.Date(2026, 8, 5, 9, 0, 0, 0, time.UTC)
	recorder := &activityRecorder{}
	producer := newTestProducer(db, sourcesOf(map[string]connector.PullSource{"flow/s1": flaky}), time.Minute, nil, zerolog.Nop())
	producer.now = func() time.Time { return now }
	producer.SetRecorder(recorder)

	producer.Tick(t.Context()) // fails: announced
	now = now.Add(time.Minute)
	producer.Tick(t.Context()) // succeeds: re-arms
	now = now.Add(time.Minute)
	producer.Tick(t.Context()) // fails again

	assert.Len(t, recorder.events, 2)
}

// failingOnceSource fails, succeeds, then fails again — the shape of a command
// whose backing service flapped.
type failingOnceSource struct {
	mu    sync.Mutex
	calls int
}

func (f *failingOnceSource) Produce(context.Context, func(Msg) error) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	if f.calls == 2 {
		return nil
	}
	return fmt.Errorf("boom")
}

// Both schedule maps are keyed by a node id the user renames freely, so an
// edited flow must not leak an entry per rename.
func TestProducer_ForgetsSourcesThatAreNoLongerConfigured(t *testing.T) {
	t.Parallel()

	db := openTestPipelineDB(t)
	broken := &fakeSource{err: fmt.Errorf("boom")}
	sources := stubSources{instances: []connector.Instance{pullInstanceEvery("flow", "s1", time.Hour, broken)}}

	producer := newTestProducer(db, sources, time.Minute, nil, zerolog.Nop())
	producer.SetRecorder(&activityRecorder{})
	producer.Tick(t.Context())

	producer.sources = stubSources{}
	producer.Tick(t.Context())

	producer.scheduleMu.Lock()
	defer producer.scheduleMu.Unlock()
	assert.Empty(t, producer.lastRun)
	assert.Empty(t, producer.lastFailure)
}
