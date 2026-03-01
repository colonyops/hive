package tui

import (
	"sync"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/colonyops/hive/internal/core/notify"
)

// NotificationBuffer buffers notifications and emits coalesced drain signals.
type NotificationBuffer struct {
	mu            sync.Mutex
	notifications []notify.Notification
	signal        chan struct{}
}

// NewNotificationBuffer constructs a buffer for async notification delivery.
func NewNotificationBuffer() *NotificationBuffer {
	return &NotificationBuffer{
		notifications: make([]notify.Notification, 0),
		signal:        make(chan struct{}, 1),
	}
}

// Push appends a notification and emits a non-blocking drain signal.
func (b *NotificationBuffer) Push(n notify.Notification) {
	if n.CreatedAt.IsZero() {
		n.CreatedAt = time.Now()
	}

	b.mu.Lock()
	b.notifications = append(b.notifications, n)
	b.mu.Unlock()

	select {
	case b.signal <- struct{}{}:
	default:
	}
}

// Drain returns all buffered notifications and clears the buffer.
func (b *NotificationBuffer) Drain() []notify.Notification {
	b.mu.Lock()
	defer b.mu.Unlock()

	if len(b.notifications) == 0 {
		return nil
	}

	out := make([]notify.Notification, len(b.notifications))
	copy(out, b.notifications)
	b.notifications = b.notifications[:0]
	return out
}

// WaitForSignal blocks until there are notifications ready to drain.
func (b *NotificationBuffer) WaitForSignal() tea.Cmd {
	return func() tea.Msg {
		<-b.signal
		return drainNotificationsMsg{}
	}
}
