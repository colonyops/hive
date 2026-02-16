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
// subscribes to all event types for recording. The bus is stopped
// when the test completes.
func New(t *testing.T) *Bus {
	t.Helper()

	bus := eventbus.New(64)
	ctx, cancel := context.WithCancel(context.Background())

	tb := &Bus{
		EventBus: bus,
		cancel:   cancel,
	}

	// Subscribe to all event types for recording.
	bus.SubscribeSessionCreated(func(p eventbus.SessionCreatedPayload) {
		tb.record(eventbus.EventSessionCreated, p)
	})
	bus.SubscribeSessionRecycled(func(p eventbus.SessionRecycledPayload) {
		tb.record(eventbus.EventSessionRecycled, p)
	})
	bus.SubscribeSessionDeleted(func(p eventbus.SessionDeletedPayload) {
		tb.record(eventbus.EventSessionDeleted, p)
	})
	bus.SubscribeSessionRenamed(func(p eventbus.SessionRenamedPayload) {
		tb.record(eventbus.EventSessionRenamed, p)
	})
	bus.SubscribeSessionCorrupted(func(p eventbus.SessionCorruptedPayload) {
		tb.record(eventbus.EventSessionCorrupted, p)
	})
	bus.SubscribeAgentStatusChanged(func(p eventbus.AgentStatusChangedPayload) {
		tb.record(eventbus.EventAgentStatusChanged, p)
	})
	bus.SubscribeMessageReceived(func(p eventbus.MessageReceivedPayload) {
		tb.record(eventbus.EventMessageReceived, p)
	})
	bus.SubscribeTuiStarted(func(p eventbus.TUIStartedPayload) {
		tb.record(eventbus.EventTuiStarted, p)
	})
	bus.SubscribeTuiStopped(func(p eventbus.TUIStoppedPayload) {
		tb.record(eventbus.EventTuiStopped, p)
	})
	bus.SubscribeConfigReloaded(func(p eventbus.ConfigReloadedPayload) {
		tb.record(eventbus.EventConfigReloaded, p)
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
