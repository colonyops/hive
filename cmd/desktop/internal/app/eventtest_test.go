package app

import (
	"context"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"github.com/colonyops/hive/cmd/desktop/internal/app/events"
)

// eventWait bounds how long a test waits for an event a service is expected
// to publish. Delivery runs on the subscriber's own goroutine (events.Bus
// hands off asynchronously even for Buffer), so a synchronous read right
// after the call that publishes would race it.
const eventWait = 2 * time.Second

func newTestBus(t *testing.T) *events.Bus {
	t.Helper()
	bus := events.New(zerolog.Nop())
	t.Cleanup(bus.Close)
	return bus
}

func subscribeEvents[E events.Event](t *testing.T, bus *events.Bus) <-chan E {
	t.Helper()
	ch := make(chan E, 64)
	cancel := events.Subscribe(t.Context(), bus, "test", events.Buffer(64), func(_ context.Context, e E) {
		ch <- e
	})
	t.Cleanup(cancel)
	return ch
}

// requireEvents waits for exactly want events of type E, in delivery order,
// and fails if a further one shows up shortly after.
func requireEvents[E any](t *testing.T, ch <-chan E, want int) []E {
	t.Helper()
	got := make([]E, 0, want)
	timeout := time.After(eventWait)
	for len(got) < want {
		select {
		case e := <-ch:
			got = append(got, e)
		case <-timeout:
			t.Fatalf("expected %d events, got %d: %+v", want, len(got), got)
		}
	}
	requireNoMoreEvents(t, ch)
	return got
}

func requireNoMoreEvents[E any](t *testing.T, ch <-chan E) {
	t.Helper()
	select {
	case e := <-ch:
		t.Fatalf("unexpected extra event: %+v", e)
	case <-time.After(100 * time.Millisecond):
	}
}
