package wailsui

import (
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/desktop/internal/app/events"
)

// swapEmitNotificationToast replaces the package-level emitter with a spy
// that records every call on toasts, restoring the original on cleanup. It is
// the same seam silenceEmits uses for emitUpdateAvailable/emitUpdateNone.
func swapEmitNotificationToast(t *testing.T, toasts chan<- NotificationToast) {
	t.Helper()
	orig := emitNotificationToast
	emitNotificationToast = func(toast NotificationToast) { toasts <- toast }
	t.Cleanup(func() { emitNotificationToast = orig })
}

// TestSubscribeEmitsToastForInAppNotification proves the Observer path Task B
// wires up: the core publishes NotificationRaised with InApp true, and this
// adapter's Subscribe turns that into exactly the notification:toast payload
// the frontend already expects -- the same shape FlowNotifier used to emit
// ad hoc from inside the driven port.
func TestSubscribeEmitsToastForInAppNotification(t *testing.T) {
	toasts := make(chan NotificationToast, 1)
	swapEmitNotificationToast(t, toasts)

	bus := events.New(zerolog.Nop())
	t.Cleanup(bus.Close)
	cancel := Subscribe(t.Context(), bus, nil)
	t.Cleanup(cancel)

	bus.Publish(t.Context(), events.NotificationRaised{
		ProfileID: "acct",
		ItemID:    42,
		Title:     "New review",
		Body:      "Someone requested your review",
		Severity:  "info",
		InApp:     true,
	})

	select {
	case got := <-toasts:
		require.Equal(t, NotificationToast{Title: "New review", Body: "Someone requested your review", Severity: "info"}, got)
	case <-time.After(2 * time.Second):
		t.Fatal("notification:toast was not emitted for an in-app NotificationRaised")
	}
}

// TestSubscribeSkipsToastForBannerNotification proves the adapter leaves an
// OS-banner delivery alone: it already went out through the notifier port
// itself, so re-raising it as a toast here would double-surface it.
func TestSubscribeSkipsToastForBannerNotification(t *testing.T) {
	toasts := make(chan NotificationToast, 1)
	swapEmitNotificationToast(t, toasts)

	bus := events.New(zerolog.Nop())
	t.Cleanup(bus.Close)
	cancel := Subscribe(t.Context(), bus, nil)
	t.Cleanup(cancel)

	bus.Publish(t.Context(), events.NotificationRaised{Title: "New review", Body: "body", Severity: "info", InApp: false})

	select {
	case got := <-toasts:
		t.Fatalf("notification:toast fired for a banner delivery: %+v", got)
	case <-time.After(200 * time.Millisecond):
	}
}

type emitted struct {
	name string
	data any
}

func swapEmitEvent(t *testing.T, sink chan<- emitted) {
	t.Helper()
	orig := emitEvent
	emitEvent = func(name string, data any) { sink <- emitted{name: name, data: data} }
	t.Cleanup(func() { emitEvent = orig })
}

func receiveEmitted(t *testing.T, sink <-chan emitted) emitted {
	t.Helper()
	select {
	case got := <-sink:
		return got
	case <-time.After(2 * time.Second):
		t.Fatal("no event was emitted")
		return emitted{}
	}
}

// The event names are the frontend's contract: it listens for drive:updated
// and session:tab by string, so this pins them.
func TestSubscribeEmitsDriveEventsUnderTheirFrontendNames(t *testing.T) {
	sink := make(chan emitted, 2)
	swapEmitEvent(t, sink)

	bus := events.New(zerolog.Nop())
	t.Cleanup(bus.Close)
	cancel := Subscribe(t.Context(), bus, nil)
	t.Cleanup(cancel)

	bus.Publish(t.Context(), events.DriveChanged{Space: "acme/site", Paths: []string{"plans/a.md"}, HiveSession: "abc123"})
	require.Equal(t, emitted{
		name: "drive:updated",
		data: DriveWrite{Space: "acme/site", Paths: []string{"plans/a.md"}, HiveSession: "abc123"},
	}, receiveEmitted(t, sink))

	bus.Publish(t.Context(), events.SessionTabRequested{
		Session: "7", Space: "workspaces/ws", View: "drive", Path: "canvases/plan.canvas.json", Placement: "beside", Open: true,
	})
	require.Equal(t, emitted{
		name: "session:tab",
		data: SessionTab{Session: "7", Space: "workspaces/ws", View: "drive", Path: "canvases/plan.canvas.json", Placement: "beside", Open: true},
	}, receiveEmitted(t, sink))
}

// Two asks for two views in a row are two intents, so none may coalesce away.
func TestSubscribeEmitsEverySessionTabRequestInOrder(t *testing.T) {
	sink := make(chan emitted, 2)
	swapEmitEvent(t, sink)

	bus := events.New(zerolog.Nop())
	t.Cleanup(bus.Close)
	cancel := Subscribe(t.Context(), bus, nil)
	t.Cleanup(cancel)

	bus.Publish(t.Context(), events.SessionTabRequested{HiveSession: "abc123", Space: "acme/site", View: "diff", Placement: "behind", Open: true})
	bus.Publish(t.Context(), events.SessionTabRequested{HiveSession: "abc123", Space: "acme/site", View: "tasks", Placement: "behind", Open: true})
	require.Equal(t, emitted{
		name: "session:tab",
		data: SessionTab{HiveSession: "abc123", Space: "acme/site", View: "diff", Placement: "behind", Open: true},
	}, receiveEmitted(t, sink))
	require.Equal(t, emitted{
		name: "session:tab",
		data: SessionTab{HiveSession: "abc123", Space: "acme/site", View: "tasks", Placement: "behind", Open: true},
	}, receiveEmitted(t, sink))
}

// One watcher batch publishes a DriveChanged per space back to back, and the
// frontend reloads only the named space, so none of them may be dropped.
func TestSubscribeEmitsEveryDriveChangeInOrder(t *testing.T) {
	sink := make(chan emitted, 2)
	swapEmitEvent(t, sink)

	bus := events.New(zerolog.Nop())
	t.Cleanup(bus.Close)
	cancel := Subscribe(t.Context(), bus, nil)
	t.Cleanup(cancel)

	bus.Publish(t.Context(), events.DriveChanged{Space: "acme/site", Paths: []string{"a.md"}})
	bus.Publish(t.Context(), events.DriveChanged{Space: "acme/api", Paths: []string{"b.md"}})

	require.Equal(t, emitted{name: "drive:updated", data: DriveWrite{Space: "acme/site", Paths: []string{"a.md"}}}, receiveEmitted(t, sink))
	require.Equal(t, emitted{name: "drive:updated", data: DriveWrite{Space: "acme/api", Paths: []string{"b.md"}}}, receiveEmitted(t, sink))
}
