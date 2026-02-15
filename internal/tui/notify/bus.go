package notify

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/colonyops/hive/internal/core/notify"
	"github.com/rs/zerolog/log"
)

// Subscriber is a callback invoked when a notification is published.
type Subscriber func(notify.Notification)

// Bus is a synchronous in-process notification bus. It dispatches notifications
// to subscribers inline and persists them to a Store. The Bus is safe for use
// from the Bubble Tea Update loop (single-threaded).
type Bus struct {
	store       notify.Store
	subscribers []Subscriber
	mu          sync.Mutex
}

// NewBus creates a notification bus backed by the given store.
// If store is nil, notifications are dispatched to subscribers but not persisted.
func NewBus(store notify.Store) *Bus {
	return &Bus{
		store: store,
	}
}

// Subscribe registers a callback that will be invoked on every Publish.
func (b *Bus) Subscribe(fn Subscriber) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.subscribers = append(b.subscribers, fn)
}

// Publish dispatches a notification to all subscribers and persists it to the store.
func (b *Bus) Publish(n notify.Notification) {
	if n.CreatedAt.IsZero() {
		n.CreatedAt = time.Now()
	}

	// Persist first so the notification has an ID for subscribers.
	if b.store != nil {
		id, err := b.store.Save(context.Background(), n)
		if err != nil {
			log.Error().Err(err).Str("message", n.Message).Msg("failed to persist notification")
		} else {
			n.ID = id
		}
	}

	b.mu.Lock()
	subs := make([]Subscriber, len(b.subscribers))
	copy(subs, b.subscribers)
	b.mu.Unlock()

	for _, fn := range subs {
		fn(n)
	}
}

// Errorf publishes an error-level notification.
func (b *Bus) Errorf(format string, args ...any) {
	b.Publish(notify.Notification{
		Level:   notify.LevelError,
		Message: fmt.Sprintf(format, args...),
	})
}

// Warnf publishes a warning-level notification.
func (b *Bus) Warnf(format string, args ...any) {
	b.Publish(notify.Notification{
		Level:   notify.LevelWarning,
		Message: fmt.Sprintf(format, args...),
	})
}

// Infof publishes an info-level notification.
func (b *Bus) Infof(format string, args ...any) {
	b.Publish(notify.Notification{
		Level:   notify.LevelInfo,
		Message: fmt.Sprintf(format, args...),
	})
}

// History returns all persisted notifications (newest first).
// Returns nil if no store is configured.
func (b *Bus) History() ([]notify.Notification, error) {
	if b.store == nil {
		return nil, nil
	}
	return b.store.List(context.Background())
}

// Clear deletes all persisted notifications.
func (b *Bus) Clear() error {
	if b.store == nil {
		return nil
	}
	return b.store.Clear(context.Background())
}
