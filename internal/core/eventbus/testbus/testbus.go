// Package testbus provides test utilities for the event bus.
// It wraps a real EventBus with event recording and assertion helpers.
package testbus

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/colonyops/hive/internal/core/eventbus"
)

// RecordedEvent holds a captured event name and payload.
type RecordedEvent struct {
	Event   eventbus.Event
	Payload any
}

// Bus wraps a real EventBus with event recording for tests.
type Bus struct {
	*eventbus.EventBus
	cancel context.CancelFunc

	mu     sync.Mutex
	events []RecordedEvent
}

// New creates a test bus, starts it in a background goroutine, and
// records all published events via the OnPublish hook. The bus is
// stopped when the test completes.
func New(t *testing.T) *Bus {
	t.Helper()

	bus := eventbus.New(64)
	ctx, cancel := context.WithCancel(context.Background())

	tb := &Bus{
		EventBus: bus,
		cancel:   cancel,
	}

	bus.OnPublish(func(event eventbus.Event, payload any) {
		tb.record(event, payload)
	})

	go bus.Start(ctx)

	t.Cleanup(func() {
		cancel()
	})

	return tb
}

func (tb *Bus) record(event eventbus.Event, payload any) {
	tb.mu.Lock()
	defer tb.mu.Unlock()
	tb.events = append(tb.events, RecordedEvent{Event: event, Payload: payload})
}

// Events returns a copy of all recorded events.
func (tb *Bus) Events() []RecordedEvent {
	tb.mu.Lock()
	defer tb.mu.Unlock()
	out := make([]RecordedEvent, len(tb.events))
	copy(out, tb.events)
	return out
}

// Reset clears all recorded events.
func (tb *Bus) Reset() {
	tb.mu.Lock()
	defer tb.mu.Unlock()
	tb.events = nil
}

// WaitFor blocks until an event of the given type is recorded or the timeout expires.
// Returns true if the event was found.
func (tb *Bus) WaitFor(event eventbus.Event, timeout time.Duration) bool {
	deadline := time.After(timeout)
	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-deadline:
			return false
		case <-ticker.C:
			if tb.has(event) {
				return true
			}
		}
	}
}

func (tb *Bus) has(event eventbus.Event) bool {
	tb.mu.Lock()
	defer tb.mu.Unlock()
	for _, e := range tb.events {
		if e.Event == event {
			return true
		}
	}
	return false
}

// FindPayload waits for the given event type to be published, then returns
// the first matching payload type-asserted to T. It fails the test if the
// event is not found within 500ms or the payload type doesn't match.
func FindPayload[T any](tb *Bus, t *testing.T, event eventbus.Event) T {
	t.Helper()
	if !tb.WaitFor(event, 500*time.Millisecond) {
		t.Fatalf("expected event %q to be published, but it was not", event)
	}
	for _, e := range tb.Events() {
		if e.Event == event {
			p, ok := e.Payload.(T)
			if !ok {
				t.Fatalf("event %q payload: got %T, want %T", event, e.Payload, p)
			}
			return p
		}
	}
	// unreachable after WaitFor succeeds, but the compiler needs it
	var zero T
	return zero
}

// AssertPublished asserts that an event of the given type was recorded.
func (tb *Bus) AssertPublished(t *testing.T, event eventbus.Event) {
	t.Helper()
	if !tb.WaitFor(event, 500*time.Millisecond) {
		t.Errorf("expected event %q to be published, but it was not", event)
	}
}

// AssertNotPublished asserts that an event of the given type was NOT recorded
// within the given wait period.
func (tb *Bus) AssertNotPublished(t *testing.T, event eventbus.Event, wait time.Duration) {
	t.Helper()
	time.Sleep(wait)
	if tb.has(event) {
		t.Errorf("expected event %q to NOT be published, but it was", event)
	}
}
