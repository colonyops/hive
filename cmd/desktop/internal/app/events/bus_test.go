package events

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const settle = 2 * time.Second

// TestCoalesce_SlowSubscriberDoesNotStallPublisher is the property the whole
// policy exists for: the producer goroutine must never be held up by a
// webview. A subscriber blocked mid-handler has to let Publish return.
func TestCoalesce_SlowSubscriberDoesNotStallPublisher(t *testing.T) {
	t.Parallel()

	bus := New(zerolog.Nop())
	t.Cleanup(bus.Close)

	entered := make(chan struct{}, 1)
	release := make(chan struct{})
	var seen []int64
	var mu sync.Mutex

	cancel := Subscribe(t.Context(), bus, "slow", Coalesce(), func(_ context.Context, e LogAppended) {
		select {
		case entered <- struct{}{}:
		default:
		}
		<-release
		mu.Lock()
		seen = append(seen, e.NextOffset)
		mu.Unlock()
	})
	t.Cleanup(cancel)

	bus.Publish(t.Context(), LogAppended{NextOffset: 1})
	select {
	case <-entered:
	case <-time.After(settle):
		t.Fatal("the subscriber never received the first event")
	}

	// The handler is now blocked. These must not block the publisher, and
	// only the newest may survive.
	done := make(chan struct{})
	go func() {
		for i := int64(2); i <= 50; i++ {
			bus.Publish(t.Context(), LogAppended{NextOffset: i})
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(settle):
		t.Fatal("Publish blocked on a busy coalescing subscriber")
	}

	close(release)
	require.Eventually(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(seen) == 2
	}, settle, 5*time.Millisecond, "expected the first event and one coalesced survivor")

	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, []int64{1, 50}, seen, "the survivor must be the newest, not an arbitrary one")
}

// TestBuffer_DeliversEveryEventInOrder is the other half of the contract: a
// consumer that must see each message gets each message.
func TestBuffer_DeliversEveryEventInOrder(t *testing.T) {
	t.Parallel()

	bus := New(zerolog.Nop())
	t.Cleanup(bus.Close)

	const count = 200
	got := make(chan int64, count)
	cancel := Subscribe(t.Context(), bus, "buffered", Buffer(4), func(_ context.Context, e LogAppended) {
		got <- e.NextOffset
	})
	t.Cleanup(cancel)

	for i := int64(1); i <= count; i++ {
		bus.Publish(t.Context(), LogAppended{NextOffset: i})
	}

	for i := int64(1); i <= count; i++ {
		select {
		case offset := <-got:
			require.Equal(t, i, offset, "buffered delivery must preserve order")
		case <-time.After(settle):
			t.Fatalf("only %d of %d events were delivered", i-1, count)
		}
	}
}

// TestSubscribe_IsTypeRouted proves the type parameter is the routing key: a
// subscriber only ever sees its own payload type.
func TestSubscribe_IsTypeRouted(t *testing.T) {
	t.Parallel()

	bus := New(zerolog.Nop())
	t.Cleanup(bus.Close)

	logs := make(chan int64, 4)
	jobs := make(chan int64, 4)
	t.Cleanup(Subscribe(t.Context(), bus, "logs", Buffer(4), func(_ context.Context, e LogAppended) { logs <- e.NextOffset }))
	t.Cleanup(Subscribe(t.Context(), bus, "jobs", Buffer(4), func(_ context.Context, e JobsUpdated) { jobs <- e.JobID }))

	bus.Publish(t.Context(), JobsUpdated{JobID: 7})

	select {
	case id := <-jobs:
		assert.Equal(t, int64(7), id)
	case <-time.After(settle):
		t.Fatal("the JobsUpdated subscriber never fired")
	}
	select {
	case <-logs:
		t.Fatal("a LogAppended subscriber received a JobsUpdated event")
	case <-time.After(50 * time.Millisecond):
	}
}

// TestSubscribe_PanickingHandlerDoesNotKillTheBus: a subscriber is not
// trusted. One that panics must not take the publisher or its siblings down.
func TestSubscribe_PanickingHandlerDoesNotKillTheBus(t *testing.T) {
	t.Parallel()

	bus := New(zerolog.Nop())
	t.Cleanup(bus.Close)

	survived := make(chan int64, 4)
	t.Cleanup(Subscribe(t.Context(), bus, "boom", Buffer(2), func(context.Context, LogAppended) {
		panic("subscriber exploded")
	}))
	t.Cleanup(Subscribe(t.Context(), bus, "fine", Buffer(2), func(_ context.Context, e LogAppended) { survived <- e.NextOffset }))

	require.NotPanics(t, func() { bus.Publish(t.Context(), LogAppended{NextOffset: 1}) })
	select {
	case offset := <-survived:
		assert.Equal(t, int64(1), offset)
	case <-time.After(settle):
		t.Fatal("a sibling subscriber was starved by a panicking one")
	}

	// And the panicking subscriber is still delivering after recovering.
	require.NotPanics(t, func() { bus.Publish(t.Context(), LogAppended{NextOffset: 2}) })
	select {
	case offset := <-survived:
		assert.Equal(t, int64(2), offset)
	case <-time.After(settle):
		t.Fatal("the bus stopped delivering after a panic")
	}
}

// TestClose_DrainsWithoutDeadlock covers the shutdown path App.Close depends
// on, including an in-flight handler and a second Close.
func TestClose_DrainsWithoutDeadlock(t *testing.T) {
	t.Parallel()

	bus := New(zerolog.Nop())

	entered := make(chan struct{})
	finished := make(chan struct{})
	Subscribe(t.Context(), bus, "inflight", Buffer(1), func(context.Context, LogAppended) {
		close(entered)
		time.Sleep(20 * time.Millisecond)
		close(finished)
	})

	bus.Publish(t.Context(), LogAppended{NextOffset: 1})
	<-entered

	closed := make(chan struct{})
	go func() { bus.Close(); close(closed) }()
	select {
	case <-closed:
	case <-time.After(settle):
		t.Fatal("Close deadlocked")
	}

	select {
	case <-finished:
	default:
		t.Fatal("Close returned before the in-flight handler finished")
	}

	require.NotPanics(t, bus.Close, "Close must be idempotent")
	require.NotPanics(t, func() { bus.Publish(t.Context(), LogAppended{NextOffset: 2}) }, "publishing to a closed bus is a no-op")
}

// TestCancel_StopsDelivery: a cancelled subscription stops receiving, and
// cancelling twice is safe.
func TestCancel_StopsDelivery(t *testing.T) {
	t.Parallel()

	bus := New(zerolog.Nop())
	t.Cleanup(bus.Close)

	got := make(chan int64, 4)
	cancel := Subscribe(t.Context(), bus, "temporary", Buffer(2), func(_ context.Context, e LogAppended) { got <- e.NextOffset })

	bus.Publish(t.Context(), LogAppended{NextOffset: 1})
	select {
	case <-got:
	case <-time.After(settle):
		t.Fatal("the subscriber never received the first event")
	}

	cancel()
	require.NotPanics(t, cancel, "cancel must be idempotent")

	bus.Publish(t.Context(), LogAppended{NextOffset: 2})
	select {
	case <-got:
		t.Fatal("a cancelled subscription still received an event")
	case <-time.After(50 * time.Millisecond):
	}
}

// TestSubscribe_ContextCancellationEndsTheSubscription: the subscription's
// lifetime is its context, which is how App.Close unwinds every adapter's
// subscriptions at once.
func TestSubscribe_ContextCancellationEndsTheSubscription(t *testing.T) {
	t.Parallel()

	bus := New(zerolog.Nop())
	t.Cleanup(bus.Close)

	ctx, cancelCtx := context.WithCancel(t.Context())
	handled := make(chan struct{}, 4)
	t.Cleanup(Subscribe(ctx, bus, "scoped", Buffer(2), func(context.Context, LogAppended) { handled <- struct{}{} }))

	bus.Publish(t.Context(), LogAppended{NextOffset: 1})
	select {
	case <-handled:
	case <-time.After(settle):
		t.Fatal("the subscriber never received the first event")
	}

	cancelCtx()
	require.Eventually(t, func() bool {
		bus.Publish(t.Context(), LogAppended{NextOffset: 2})
		select {
		case <-handled:
			return false
		case <-time.After(20 * time.Millisecond):
			return true
		}
	}, settle, 10*time.Millisecond, "delivery continued after the subscription context was cancelled")
}

func TestBuffer_RejectsAnUnusableSize(t *testing.T) {
	t.Parallel()

	assert.Equal(t, 1, Buffer(0).size, "an unbuffered queue would make every publish synchronous")
	assert.Equal(t, 1, Buffer(-3).size)
}

func TestSubscribe_ZeroPolicyPanics(t *testing.T) {
	t.Parallel()

	bus := New(zerolog.Nop())
	t.Cleanup(bus.Close)

	assert.Panics(t, func() {
		Subscribe(t.Context(), bus, "invalid", Policy{}, func(context.Context, LogAppended) {})
	}, "a zero Policy is a wiring bug, not a runtime condition")
}
