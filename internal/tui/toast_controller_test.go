package tui

import (
	"testing"
	"time"

	"github.com/hay-kot/hive/internal/core/notify"
	"github.com/stretchr/testify/assert"
)

func newTestController() (*ToastController, *time.Time) {
	now := time.Now()
	c := NewToastController()
	c.now = func() time.Time { return now }
	return c, &now
}

func TestToastController_Push(t *testing.T) {
	c, now := newTestController()

	c.Push(notify.Notification{Level: notify.LevelInfo, Message: "hello"})

	assert.True(t, c.HasToasts())
	assert.Len(t, c.Toasts(), 1)
	assert.Equal(t, "hello", c.Toasts()[0].notification.Message)
	assert.Equal(t, now.Add(defaultToastTTL), c.Toasts()[0].expiresAt)
}

func TestToastController_Push_evicts_oldest_at_max(t *testing.T) {
	c, _ := newTestController()

	for i := range defaultMaxToasts + 2 {
		c.Push(notify.Notification{
			Level:   notify.LevelInfo,
			Message: time.Duration(i).String(),
		})
	}

	assert.Len(t, c.Toasts(), defaultMaxToasts)
	// Oldest two should have been evicted; first remaining is "2".
	assert.Equal(t, "2ns", c.Toasts()[0].notification.Message)
}

func TestToastController_Tick_removes_expired(t *testing.T) {
	c, now := newTestController()
	c.Push(notify.Notification{Level: notify.LevelInfo, Message: "expires"})
	c.Push(notify.Notification{Level: notify.LevelInfo, Message: "survives"})

	// Expire the first one by setting it to expire soon.
	c.toasts[0].expiresAt = now.Add(50 * time.Millisecond)

	// Advance clock past the first toast's expiration.
	*now = now.Add(100 * time.Millisecond)
	c.Tick()

	assert.Len(t, c.Toasts(), 1)
	assert.Equal(t, "survives", c.Toasts()[0].notification.Message)
}

func TestToastController_Tick_keeps_unexpired(t *testing.T) {
	c, now := newTestController()
	c.Push(notify.Notification{Level: notify.LevelInfo, Message: "stays"})

	// Advance clock by 1 second — well within the 5s TTL.
	*now = now.Add(1 * time.Second)
	c.Tick()

	assert.Len(t, c.Toasts(), 1)
}

func TestToastController_Tick_expires_at_ttl(t *testing.T) {
	c, now := newTestController()
	c.Push(notify.Notification{Level: notify.LevelInfo, Message: "gone"})

	// Advance clock past the full TTL.
	*now = now.Add(defaultToastTTL + time.Millisecond)
	c.Tick()

	assert.False(t, c.HasToasts())
}

func TestToastController_Dismiss(t *testing.T) {
	c, _ := newTestController()
	c.Push(notify.Notification{Level: notify.LevelInfo, Message: "first"})
	c.Push(notify.Notification{Level: notify.LevelInfo, Message: "second"})

	c.Dismiss()

	assert.Len(t, c.Toasts(), 1)
	assert.Equal(t, "first", c.Toasts()[0].notification.Message)
}

func TestToastController_Dismiss_empty(t *testing.T) {
	c := NewToastController()
	c.Dismiss() // should not panic
	assert.False(t, c.HasToasts())
}

func TestToastController_DismissAll(t *testing.T) {
	c, _ := newTestController()
	c.Push(notify.Notification{Level: notify.LevelInfo, Message: "a"})
	c.Push(notify.Notification{Level: notify.LevelInfo, Message: "b"})

	c.DismissAll()

	assert.False(t, c.HasToasts())
	assert.Empty(t, c.Toasts())
}
