package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/hay-kot/hive/internal/core/notify"
	tuinotify "github.com/hay-kot/hive/internal/tui/notify"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestToastUpdateLoop_tick_chain_expires_at_TTL simulates the Bubbletea update
// loop by sending toastTickMsg messages and verifying the toast is removed after
// the expected number of ticks.
func TestToastUpdateLoop_tick_chain_expires_at_TTL(t *testing.T) {
	ctrl, now := newTestController()

	ctrl.Push(notify.Notification{Level: notify.LevelInfo, Message: "test"})

	m := Model{toastController: ctrl}

	tickCount := 0
	for {
		*now = now.Add(toastTickInterval)

		result, cmd := m.Update(toastTickMsg(*now))
		m = result.(Model)
		tickCount++

		if cmd == nil {
			break
		}
		if tickCount > 100 {
			t.Fatal("tick chain ran for >100 ticks without expiring")
		}
	}

	expectedTicks := int(defaultToastTTL / toastTickInterval) // 5s / 100ms = 50
	assert.Equal(t, expectedTicks, tickCount)
	assert.False(t, ctrl.HasToasts())
}

// TestToastUpdateLoop_notificationMsg_starts_tick starts from a notificationMsg
// and verifies the full chain through Update.
func TestToastUpdateLoop_notificationMsg_starts_tick(t *testing.T) {
	ctrl := NewToastController()
	bus := tuinotify.NewBus(nil)
	bus.Subscribe(func(n notify.Notification) {
		ctrl.Push(n)
	})

	m := Model{
		toastController: ctrl,
		notifyBus:       bus,
	}

	_, cmd := m.Update(notificationMsg{
		notification: notify.Notification{
			Level:   notify.LevelError,
			Message: "something broke",
		},
	})

	require.True(t, ctrl.HasToasts(), "toast should be pushed")
	assert.Equal(t, "something broke", ctrl.Toasts()[0].notification.Message)
	assert.NotNil(t, cmd, "should return scheduleToastTick cmd")
}

// TestToastUpdateLoop_full_lifecycle runs notification → tick chain → expiry
// end-to-end through the Model Update loop.
func TestToastUpdateLoop_full_lifecycle(t *testing.T) {
	ctrl, now := newTestController()
	bus := tuinotify.NewBus(nil)
	bus.Subscribe(func(n notify.Notification) {
		ctrl.Push(n)
	})

	m := Model{
		toastController: ctrl,
		notifyBus:       bus,
	}

	// Step 1: Push notification
	result, cmd := m.Update(notificationMsg{
		notification: notify.Notification{
			Level:   notify.LevelInfo,
			Message: "hello",
		},
	})
	m = result.(Model)
	require.NotNil(t, cmd, "notification should start tick chain")
	require.True(t, ctrl.HasToasts())

	// Step 2: Simulate tick chain until expiry
	tickCount := 0
	for cmd != nil {
		*now = now.Add(toastTickInterval)
		result, cmd = m.Update(toastTickMsg(*now))
		m = result.(Model)
		tickCount++

		if tickCount > 100 {
			t.Fatalf("tick chain exceeded 100 ticks; toasts remaining: %d", len(ctrl.Toasts()))
		}
	}

	assert.False(t, ctrl.HasToasts(), "toast should be expired")

	expectedTicks := int(defaultToastTTL / toastTickInterval)
	assert.Equal(t, expectedTicks, tickCount)
}

// TestToastUpdateLoop_second_notification_during_chain verifies that a new
// notification pushed while the tick chain is running gets its own tick cmd,
// ensuring the chain continues even if the original chain's cmd is in-flight.
func TestToastUpdateLoop_second_notification_during_chain(t *testing.T) {
	ctrl, now := newTestController()
	bus := tuinotify.NewBus(nil)
	bus.Subscribe(func(n notify.Notification) {
		ctrl.Push(n)
	})

	m := Model{
		toastController: ctrl,
		notifyBus:       bus,
	}

	// Push first notification
	result, _ := m.Update(notificationMsg{
		notification: notify.Notification{Level: notify.LevelInfo, Message: "first"},
	})
	m = result.(Model)

	// Run 25 ticks (2.5s into the first toast's 5s TTL)
	var cmd tea.Cmd
	for range 25 {
		*now = now.Add(toastTickInterval)
		result, _ = m.Update(toastTickMsg(*now))
		m = result.(Model)
	}

	// Push second notification (TTL starts now, expires at T+7.5s)
	result, cmd = m.Update(notificationMsg{
		notification: notify.Notification{Level: notify.LevelInfo, Message: "second"},
	})
	m = result.(Model)
	require.Len(t, ctrl.Toasts(), 2, "should have 2 toasts")

	// notificationMsg always returns a tick cmd when toasts exist.
	// This ensures the chain continues even if the previous tick's cmd
	// hasn't fired yet.
	require.NotNil(t, cmd, "notificationMsg should always return tick cmd when toasts exist")

	// Run ticks until all expire
	tickCount := 25 // already did 25
	for cmd != nil {
		*now = now.Add(toastTickInterval)
		result, cmd = m.Update(toastTickMsg(*now))
		m = result.(Model)
		tickCount++

		if tickCount > 200 {
			t.Fatal("tick chain ran too long")
		}
	}

	assert.False(t, ctrl.HasToasts())

	// First toast: expires at T+5s. Second toast: pushed at T+2.5s, expires at T+7.5s.
	// Total ticks from start: 75 (7.5s / 100ms).
	assert.Equal(t, 75, tickCount)
}

// TestToastUpdateLoop_new_toast_after_chain_stops verifies that ensureToastTick
// restarts the chain after all toasts have expired.
func TestToastUpdateLoop_new_toast_after_chain_stops(t *testing.T) {
	ctrl, now := newTestController()
	bus := tuinotify.NewBus(nil)
	bus.Subscribe(func(n notify.Notification) {
		ctrl.Push(n)
	})

	m := Model{
		toastController: ctrl,
		notifyBus:       bus,
	}

	// Push and expire first toast
	result, cmd := m.Update(notificationMsg{
		notification: notify.Notification{Level: notify.LevelInfo, Message: "first"},
	})
	m = result.(Model)
	for cmd != nil {
		*now = now.Add(toastTickInterval)
		result, cmd = m.Update(toastTickMsg(*now))
		m = result.(Model)
	}
	require.False(t, ctrl.HasToasts())

	// Push second toast — ensureToastTick should start a new chain
	_, cmd = m.Update(notificationMsg{
		notification: notify.Notification{Level: notify.LevelInfo, Message: "second"},
	})

	assert.True(t, ctrl.HasToasts(), "second toast should exist")
	assert.NotNil(t, cmd, "should return tick cmd")
}
