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
