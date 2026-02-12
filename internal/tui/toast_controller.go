package tui

import (
	"time"

	"github.com/hay-kot/hive/internal/core/notify"
)

const (
	defaultToastTTL   = 5 * time.Second
	defaultMaxToasts  = 5
	toastTickInterval = 100 * time.Millisecond
	toastWidth        = 50
)

type toast struct {
	notification notify.Notification
	remaining    time.Duration
}

// ToastController manages the lifecycle of active toast notifications.
// It handles push, eviction, TTL countdown, and dismissal.
type ToastController struct {
	toasts  []toast
	ticking bool
}

func NewToastController() *ToastController {
	return &ToastController{}
}

// Push adds a notification to the toast stack. If the stack exceeds
// defaultMaxToasts, the oldest toast is evicted.
func (c *ToastController) Push(n notify.Notification) {
	c.toasts = append(c.toasts, toast{
		notification: n,
		remaining:    defaultToastTTL,
	})
	if len(c.toasts) > defaultMaxToasts {
		c.toasts = c.toasts[len(c.toasts)-defaultMaxToasts:]
	}
}

// Tick decrements the remaining TTL on all toasts by d and removes
// any that have expired.
func (c *ToastController) Tick(d time.Duration) {
	alive := c.toasts[:0]
	for _, t := range c.toasts {
		t.remaining -= d
		if t.remaining > 0 {
			alive = append(alive, t)
		}
	}
	c.toasts = alive
}

// Dismiss removes the newest (bottom-most) toast.
func (c *ToastController) Dismiss() {
	if len(c.toasts) > 0 {
		c.toasts = c.toasts[:len(c.toasts)-1]
	}
}

// DismissAll removes all active toasts.
func (c *ToastController) DismissAll() {
	c.toasts = c.toasts[:0]
}

// HasToasts returns true if there are any active toasts.
func (c *ToastController) HasToasts() bool {
	return len(c.toasts) > 0
}

// Toasts returns the current active toast slice.
func (c *ToastController) Toasts() []toast {
	return c.toasts
}

// Ticking returns whether the tick timer is currently running.
func (c *ToastController) Ticking() bool {
	return c.ticking
}

// SetTicking sets the tick timer state.
func (c *ToastController) SetTicking(v bool) {
	c.ticking = v
}
