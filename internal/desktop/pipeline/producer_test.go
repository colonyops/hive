package pipeline

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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

// fakeAppender records Append calls without touching disk, for tests that
// only care about whether/how many times Append was invoked.
type fakeAppender struct {
	mu      sync.Mutex
	nextOff int64
	calls   []pipelinedb.Msg
}

func (a *fakeAppender) Append(_ context.Context, topic, key string, payload []byte) (int64, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.nextOff++
	a.calls = append(a.calls, pipelinedb.Msg{Topic: topic, Key: key, Payload: payload})
	return a.nextOff, nil
}

func (a *fakeAppender) callCount() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return len(a.calls)
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
	require.Len(t, msgs, 2)
	assert.Equal(t, "a", msgs[0].Key)
	assert.Equal(t, "b", msgs[1].Key)
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

func TestProducer_Tick_NoAppends_NoWakeup(t *testing.T) {
	t.Parallel()

	db := openTestPipelineDB(t)
	src := &fakeSource{} // no batches configured: Produce emits nothing

	woke := false
	producer := NewProducer(db, listerOf(map[string]Source{"s1": src}), time.Hour, func(int64) {
		woke = true
	}, zerolog.Nop())

	producer.Tick(t.Context())
	assert.False(t, woke, "a tick that appends nothing must not wake the frontend")

	msgs, _, err := db.ReadFrom(t.Context(), 0, 10)
	require.NoError(t, err)
	assert.Empty(t, msgs)
}

func TestProducer_Tick_SourceErrorDoesNotBlockOthers(t *testing.T) {
	t.Parallel()

	db := openTestPipelineDB(t)
	failing := &fakeSource{err: fmt.Errorf("boom")}
	ok := &fakeSource{batches: [][]Msg{{{Topic: "source:ok", Key: "x", Payload: []byte(`{}`)}}}}

	var appendedOffsets []int64
	producer := NewProducer(db, listerOf(map[string]Source{
		"failing": failing,
		"ok":      ok,
	}), time.Hour, func(offset int64) {
		appendedOffsets = append(appendedOffsets, offset)
	}, zerolog.Nop())

	producer.Tick(t.Context())

	require.Len(t, appendedOffsets, 1, "the healthy source's append still wakes the frontend")
	msgs, _, err := db.ReadFrom(t.Context(), 0, 10)
	require.NoError(t, err)
	require.Len(t, msgs, 1)
	assert.Equal(t, "x", msgs[0].Key)
}

// TestProducer_DedupesUnchangedPayload verifies the dedup-vs-Compact choice:
// an unchanged payload for the same topic+key is not re-appended on the next
// tick, but a changed payload is.
func TestProducer_DedupesUnchangedPayload(t *testing.T) {
	t.Parallel()

	appender := &fakeAppender{}
	src := &fakeSource{batches: [][]Msg{
		{{Topic: "source:s1", Key: "a", Payload: []byte(`{"v":1}`)}}, // tick 1: new
		{{Topic: "source:s1", Key: "a", Payload: []byte(`{"v":1}`)}}, // tick 2: unchanged
		{{Topic: "source:s1", Key: "a", Payload: []byte(`{"v":2}`)}}, // tick 3: changed
	}}

	var wakeCount int
	producer := NewProducer(appender, listerOf(map[string]Source{"s1": src}), time.Hour, func(int64) {
		wakeCount++
	}, zerolog.Nop())

	producer.Tick(t.Context())
	assert.Equal(t, 1, appender.callCount(), "tick 1 appends the new item")
	assert.Equal(t, 1, wakeCount)

	producer.Tick(t.Context())
	assert.Equal(t, 1, appender.callCount(), "tick 2 is an unchanged payload: no append, no wake-up")
	assert.Equal(t, 1, wakeCount)

	producer.Tick(t.Context())
	assert.Equal(t, 2, appender.callCount(), "tick 3's changed payload is appended")
	assert.Equal(t, 2, wakeCount)
}

// TestProducer_EmptyKeyNeverDeduped mirrors pipelinedb.Compact's exemption
// of empty-key rows: Producer must not collapse repeated empty-key emits
// into a single append.
func TestProducer_EmptyKeyNeverDeduped(t *testing.T) {
	t.Parallel()

	appender := &fakeAppender{}
	src := &fakeSource{batches: [][]Msg{
		{{Topic: "source:s1", Key: "", Payload: []byte(`{"v":1}`)}},
		{{Topic: "source:s1", Key: "", Payload: []byte(`{"v":1}`)}},
	}}

	producer := NewProducer(appender, listerOf(map[string]Source{"s1": src}), time.Hour, nil, zerolog.Nop())

	producer.Tick(t.Context())
	producer.Tick(t.Context())
	assert.Equal(t, 2, appender.callCount(), "empty-key messages are always appended")
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
	t.Parallel()

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
	time.Sleep(50 * time.Millisecond)
	producer.Stop()
	producer.Stop() // idempotent

	mu.Lock()
	got := wakeCount
	mu.Unlock()
	assert.GreaterOrEqual(t, got, 1, "at least one tick should have run and appended")
}
