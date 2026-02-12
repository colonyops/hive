package tui

import (
	"testing"
	"time"

	"github.com/hay-kot/hive/internal/core/notify"
	"github.com/stretchr/testify/assert"
)

func TestToastController_Push(t *testing.T) {
	c := NewToastController()

	c.Push(notify.Notification{Level: notify.LevelInfo, Message: "hello"})

	assert.True(t, c.HasToasts())
	assert.Len(t, c.Toasts(), 1)
	assert.Equal(t, "hello", c.Toasts()[0].notification.Message)
	assert.Equal(t, defaultToastTTL, c.Toasts()[0].remaining)
}

func TestToastController_Push_evicts_oldest_at_max(t *testing.T) {
	c := NewToastController()

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

func TestToastController_Tick_decrements_TTL(t *testing.T) {
	c := NewToastController()
	c.Push(notify.Notification{Level: notify.LevelInfo, Message: "tick"})

	c.Tick(1 * time.Second)

	assert.Equal(t, defaultToastTTL-1*time.Second, c.Toasts()[0].remaining)
}

func TestToastController_Tick_removes_expired(t *testing.T) {
	c := NewToastController()
	c.Push(notify.Notification{Level: notify.LevelInfo, Message: "expires"})
	c.Push(notify.Notification{Level: notify.LevelInfo, Message: "survives"})

	// Expire the first one by consuming most of its TTL, then add time.
	c.toasts[0].remaining = 50 * time.Millisecond
	c.Tick(100 * time.Millisecond)

	assert.Len(t, c.Toasts(), 1)
	assert.Equal(t, "survives", c.Toasts()[0].notification.Message)
}

func TestToastController_Dismiss(t *testing.T) {
	c := NewToastController()
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
	c := NewToastController()
	c.Push(notify.Notification{Level: notify.LevelInfo, Message: "a"})
	c.Push(notify.Notification{Level: notify.LevelInfo, Message: "b"})

	c.DismissAll()

	assert.False(t, c.HasToasts())
	assert.Empty(t, c.Toasts())
}

func TestToastController_Ticking(t *testing.T) {
	c := NewToastController()
	assert.False(t, c.Ticking())

	c.SetTicking(true)
	assert.True(t, c.Ticking())

	c.SetTicking(false)
	assert.False(t, c.Ticking())
}
