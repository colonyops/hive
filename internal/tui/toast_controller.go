package tui

import (
	"time"

	"github.com/colonyops/hive/internal/core/notify"
)

const (
	defaultToastTTL   = 5 * time.Second
	defaultMaxToasts  = 5
	toastTickInterval = 100 * time.Millisecond
	toastWidth        = 50
)

type toast struct {
	notification notify.Notification
	expiresAt    time.Time
}

// ToastController manages the lifecycle of active toast notifications.
// It handles push, eviction, TTL countdown, and dismissal.
type ToastController struct {
	toasts []toast
	now    func() time.Time // injectable clock for testing
}

func NewToastController() *ToastController {
	return &ToastController{now: time.Now}
}

// Push adds a notification to the toast stack. If the stack exceeds
// defaultMaxToasts, the oldest toast is evicted.
func (c *ToastController) Push(n notify.Notification) {
	c.toasts = append(c.toasts, toast{
		notification: n,
		expiresAt:    c.now().Add(defaultToastTTL),
	})
	if len(c.toasts) > defaultMaxToasts {
		c.toasts = c.toasts[len(c.toasts)-defaultMaxToasts:]
	}
}

// Tick removes any toasts whose expiration time has passed.
func (c *ToastController) Tick() {
	now := c.now()
	alive := c.toasts[:0]
	for _, t := range c.toasts {
		if now.Before(t.expiresAt) {
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
