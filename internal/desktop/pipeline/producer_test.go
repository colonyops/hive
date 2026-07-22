package pipeline

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/internal/desktop/activity"
	"github.com/colonyops/hive/internal/desktop/pipeline/pipelinedb"
)

// fakeSource drives Producer.Tick with canned batches, one per call to
// Produce, so tests can assert append behavior without a network fetch.
type fakeSource struct {
	mu      sync.Mutex
	batches [][]Msg
	calls   int
	err     error
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

func listerOf(sources map[string]Source) SourceLister {
	return func(context.Context) (map[string]Source, error) {
		return sources, nil
	}
}

// fakeAppender records AppendIfChanged calls without touching disk, for tests
// that only care whether Producer invokes its database dependency.
type fakeAppender struct {
	mu        sync.Mutex
	nextOff   int64
	calls     []pipelinedb.Msg
	snapshots int
}

func (a *fakeAppender) AppendIfChanged(_ context.Context, topic, key string, payload []byte) (int64, bool, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.nextOff++
	a.calls = append(a.calls, pipelinedb.Msg{Topic: topic, Key: key, Payload: payload})
	return a.nextOff, true, nil
}

func (a *fakeAppender) AppendSnapshot(_ context.Context, _ string, _ []pipelinedb.SnapshotItem) (int64, error) {
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
	events []activity.Event
}

func (r *activityRecorder) Record(_ context.Context, event activity.Event) {
	r.events = append(r.events, event)
}

func openTestPipelineDB(t *testing.T) *pipelinedb.DB {
	t.Helper()
	db, err := pipelinedb.Open(t.TempDir(), pipelinedb.DefaultOpenOptions())
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestProducer_Tick_AppendsMonotonicOffsets(t *testing.T) {
	t.Parallel()

	db := openTestPipelineDB(t)
	src := &fakeSource{batches: [][]Msg{
		{
			{Topic: "source:s1", Key: "a", Payload: []byte(`{"v":1}`)},
			{Topic: "source:s1", Key: "b", Payload: []byte(`{"v":2}`)},
		},
	}}

	var appendedOffsets []int64
	producer := NewProducer(db, listerOf(map[string]Source{"s1": src}), time.Hour, func(offset int64) {
		appendedOffsets = append(appendedOffsets, offset)
	}, zerolog.Nop())

	producer.Tick(t.Context())

	require.Len(t, appendedOffsets, 1, "one wake-up per tick that appended something")

	msgs, next, err := db.ReadFrom(t.Context(), 0, 10)
	require.NoError(t, err)
	require.Len(t, msgs, 3)
	assert.Equal(t, "a", msgs[0].Key)
	assert.Equal(t, "b", msgs[1].Key)
	assert.Len(t, msgs[2].Snapshot, 2)
	assert.Equal(t, "source:s1", msgs[0].Topic)
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

func TestProducer_Tick_EmptySnapshot_WakesConsumer(t *testing.T) {
	t.Parallel()

	db := openTestPipelineDB(t)
	src := &fakeSource{} // no batches configured: Produce emits nothing

	woke := false
	producer := NewProducer(db, listerOf(map[string]Source{"s1": src}), time.Hour, func(int64) {
		woke = true
	}, zerolog.Nop())

	producer.Tick(t.Context())
	assert.True(t, woke, "an empty successful snapshot must wake the frontend for reconciliation")

	msgs, _, err := db.ReadFrom(t.Context(), 0, 10)
	require.NoError(t, err)
	require.Len(t, msgs, 1)
	assert.Empty(t, msgs[0].Snapshot)
}

func TestProducer_Tick_SourceErrorDoesNotBlockOthers(t *testing.T) {
	t.Parallel()

	db := openTestPipelineDB(t)
	failing := &fakeSource{err: fmt.Errorf("boom")}
	ok := &fakeSource{batches: [][]Msg{{{Topic: "source:ok", Key: "x", Payload: []byte(`{}`)}}}}

	var appendedOffsets []int64
	recorder := &activityRecorder{}
	producer := NewProducer(db, listerOf(map[string]Source{
		"failing": failing,
		"ok":      ok,
	}), time.Hour, func(offset int64) {
		appendedOffsets = append(appendedOffsets, offset)
	}, zerolog.Nop())
	producer.SetRecorder(recorder)

	producer.Tick(t.Context())

	require.Len(t, appendedOffsets, 1, "the healthy source's append still wakes the frontend")
	msgs, _, err := db.ReadFrom(t.Context(), 0, 10)
	require.NoError(t, err)
	require.Len(t, msgs, 2)
	assert.Equal(t, "x", msgs[0].Key)
	assert.Len(t, msgs[1].Snapshot, 1)
	require.Len(t, recorder.events, 1, "successful refreshes must not crowd the activity log")
	assert.Equal(t, activity.CategoryRefresh, recorder.events[0].Category)
	assert.Equal(t, activity.SeverityError, recorder.events[0].Severity)
	assert.Equal(t, "Refresh failed for failing", recorder.events[0].Title)
}

// TestProducer_DedupesUnchangedPayload verifies durable deduplication: an
// unchanged payload for the same topic/key is not re-appended on the next
// tick, but a changed payload is.
func TestProducer_DedupesUnchangedPayload(t *testing.T) {
	t.Parallel()

	db := openTestPipelineDB(t)
	src := &fakeSource{batches: [][]Msg{
		{{Topic: "source:s1", Key: "a", Payload: []byte(`{"v":1}`)}}, // tick 1: new
		{{Topic: "source:s1", Key: "a", Payload: []byte(`{"v":1}`)}}, // tick 2: unchanged
		{{Topic: "source:s1", Key: "a", Payload: []byte(`{"v":2}`)}}, // tick 3: changed
	}}

	var wakeCount int
	producer := NewProducer(db, listerOf(map[string]Source{"s1": src}), time.Hour, func(int64) {
		wakeCount++
	}, zerolog.Nop())

	producer.Tick(t.Context())
	producer.Tick(t.Context())
	producer.Tick(t.Context())

	msgs, _, err := db.ReadFrom(t.Context(), 0, 10)
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
		{{Topic: "source:s1", Key: "", Payload: []byte(`{"v":1}`)}},
		{{Topic: "source:s1", Key: "", Payload: []byte(`{"v":1}`)}},
	}}

	producer := NewProducer(db, listerOf(map[string]Source{"s1": src}), time.Hour, nil, zerolog.Nop())

	producer.Tick(t.Context())
	producer.Tick(t.Context())
	msgs, _, err := db.ReadFrom(t.Context(), 0, 10)
	require.NoError(t, err)
	assert.Len(t, msgs, 4, "empty-key messages and source snapshots are always appended")
}

func TestProducer_DeduplicationSurvivesRestart(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	firstDB, err := pipelinedb.Open(dir, pipelinedb.DefaultOpenOptions())
	require.NoError(t, err)

	first := NewProducer(firstDB, listerOf(map[string]Source{
		"s1": &fakeSource{batches: [][]Msg{{{Topic: "source:s1", Key: "a", Payload: []byte(`{"v":1}`)}}}},
	}), time.Hour, nil, zerolog.Nop())
	first.Tick(t.Context())
	require.NoError(t, firstDB.Close())

	secondDB, err := pipelinedb.Open(dir, pipelinedb.DefaultOpenOptions())
	require.NoError(t, err)
	t.Cleanup(func() { _ = secondDB.Close() })
	second := NewProducer(secondDB, listerOf(map[string]Source{
		"s1": &fakeSource{batches: [][]Msg{{{Topic: "source:s1", Key: "a", Payload: []byte(`{"v":1}`)}}}},
	}), time.Hour, nil, zerolog.Nop())
	second.Tick(t.Context())

	msgs, _, err := secondDB.ReadFrom(t.Context(), 0, 10)
	require.NoError(t, err)
	assert.Len(t, msgs, 3, "a restarted producer retains source heads while still appending snapshots")
}

func TestProducer_ResolvingSourcesErrorSkipsTick(t *testing.T) {
	t.Parallel()

	appender := &fakeAppender{}
	lister := func(context.Context) (map[string]Source, error) {
		return nil, fmt.Errorf("config broken")
	}
	woke := false
	producer := NewProducer(appender, lister, time.Hour, func(int64) { woke = true }, zerolog.Nop())

	producer.Tick(t.Context())
	assert.Equal(t, 0, appender.callCount())
	assert.False(t, woke)
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
			{{Topic: "source:s1", Key: "a", Payload: []byte(`{"v":1}`)}},
		}}
		var wakeCount int
		var mu sync.Mutex
		producer := NewProducer(db, listerOf(map[string]Source{"s1": src}), 10*time.Millisecond, func(int64) {
			mu.Lock()
			wakeCount++
			mu.Unlock()
		}, zerolog.Nop())

		producer.Start()
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
