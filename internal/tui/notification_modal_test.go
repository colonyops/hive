package tui

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/hay-kot/hive/internal/core/notify"
	"github.com/hay-kot/hive/internal/core/styles"
	tuinotify "github.com/hay-kot/hive/internal/tui/notify"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubStore is a minimal notify.Store for testing that can optionally return errors.
type stubStore struct {
	items    []notify.Notification
	nextID   int64
	listErr  error
	clearErr error
}

func (s *stubStore) Save(_ context.Context, n notify.Notification) (int64, error) {
	s.nextID++
	n.ID = s.nextID
	s.items = append(s.items, n)
	return n.ID, nil
}

func (s *stubStore) List(_ context.Context) ([]notify.Notification, error) {
	if s.listErr != nil {
		return nil, s.listErr
	}
	out := make([]notify.Notification, len(s.items))
	for i, n := range s.items {
		out[len(s.items)-1-i] = n
	}
	return out, nil
}

func (s *stubStore) Clear(_ context.Context) error {
	if s.clearErr != nil {
		return s.clearErr
	}
	s.items = nil
	return nil
}

func (s *stubStore) Count(_ context.Context) (int64, error) {
	return int64(len(s.items)), nil
}

func newTestBus(store notify.Store) *tuinotify.Bus {
	return tuinotify.NewBus(store)
}

func TestNotificationModal_empty_history(t *testing.T) {
	bus := newTestBus(&stubStore{})
	m := NewNotificationModal(bus, 100, 40)

	content := m.viewport.View()
	assert.Contains(t, content, "No notifications")
}

func TestNotificationModal_populated_history(t *testing.T) {
	store := &stubStore{}
	bus := newTestBus(store)

	bus.Infof("first message")
	bus.Errorf("second message")
	bus.Warnf("third message")

	m := NewNotificationModal(bus, 100, 40)
	content := m.viewport.View()

	assert.Contains(t, content, "first message")
	assert.Contains(t, content, "second message")
	assert.Contains(t, content, "third message")
}

func TestNotificationModal_history_error(t *testing.T) {
	store := &stubStore{
		listErr: errors.New("db connection failed"),
	}
	bus := newTestBus(store)

	m := NewNotificationModal(bus, 100, 40)
	content := m.viewport.View()

	assert.Contains(t, content, "failed to load notifications")
	assert.Contains(t, content, "db connection failed")
}

func TestNotificationModal_Clear_removes_notifications(t *testing.T) {
	store := &stubStore{}
	bus := newTestBus(store)

	bus.Infof("will be cleared")

	m := NewNotificationModal(bus, 100, 40)
	require.Contains(t, m.viewport.View(), "will be cleared")

	err := m.Clear()
	require.NoError(t, err)

	assert.Contains(t, m.viewport.View(), "No notifications")
}

func TestNotificationModal_Clear_returns_store_error(t *testing.T) {
	store := &stubStore{
		clearErr: errors.New("clear failed"),
	}
	bus := newTestBus(store)

	m := NewNotificationModal(bus, 100, 40)
	err := m.Clear()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "clear failed")
}

func TestNotificationModal_formatNotification_levels(t *testing.T) {
	now := time.Date(2026, 1, 15, 14, 30, 45, 0, time.UTC)

	tests := []struct {
		level notify.Level
		icon  string
	}{
		{notify.LevelInfo, styles.IconNotifyInfo},
		{notify.LevelWarning, styles.IconNotifyWarning},
		{notify.LevelError, styles.IconNotifyError},
	}

	for _, tt := range tests {
		t.Run(string(tt.level), func(t *testing.T) {
			n := notify.Notification{
				Level:     tt.level,
				Message:   "test",
				CreatedAt: now,
			}
			out := formatNotification(n)
			assert.Contains(t, out, tt.icon)
			assert.Contains(t, out, "14:30:45")
			assert.Contains(t, out, "test")
		})
	}
}

