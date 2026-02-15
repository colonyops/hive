package notify

import (
	"context"
	"testing"

	"github.com/colonyops/hive/internal/core/notify"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// memStore is an in-memory notify.Store for testing.
type memStore struct {
	items  []notify.Notification
	nextID int64
}

func (m *memStore) Save(_ context.Context, n notify.Notification) (int64, error) {
	m.nextID++
	n.ID = m.nextID
	m.items = append(m.items, n)
	return n.ID, nil
}

func (m *memStore) List(_ context.Context) ([]notify.Notification, error) {
	// Return newest first.
	out := make([]notify.Notification, len(m.items))
	for i, n := range m.items {
		out[len(m.items)-1-i] = n
	}
	return out, nil
}

func (m *memStore) Clear(_ context.Context) error {
	m.items = nil
	return nil
}

func (m *memStore) Count(_ context.Context) (int64, error) {
	return int64(len(m.items)), nil
}

func TestBus_Publish_dispatches_to_subscribers(t *testing.T) {
	bus := NewBus(&memStore{})

	var received []notify.Notification
	bus.Subscribe(func(n notify.Notification) {
		received = append(received, n)
	})

	bus.Errorf("test error: %d", 42)
	bus.Infof("info msg")
	bus.Warnf("warn msg")

	require.Len(t, received, 3)
	assert.Equal(t, notify.LevelError, received[0].Level)
	assert.Equal(t, "test error: 42", received[0].Message)
	assert.Equal(t, notify.LevelInfo, received[1].Level)
	assert.Equal(t, notify.LevelWarning, received[2].Level)
}

func TestBus_Publish_persists_to_store(t *testing.T) {
	store := &memStore{}
	bus := NewBus(store)

	bus.Errorf("persisted error")

	assert.Len(t, store.items, 1)
	assert.Equal(t, "persisted error", store.items[0].Message)
}

func TestBus_Publish_assigns_id_from_store(t *testing.T) {
	store := &memStore{}
	bus := NewBus(store)

	var received notify.Notification
	bus.Subscribe(func(n notify.Notification) {
		received = n
	})

	bus.Infof("get id")

	assert.Equal(t, int64(1), received.ID)
}

func TestBus_History_returns_newest_first(t *testing.T) {
	store := &memStore{}
	bus := NewBus(store)

	bus.Infof("first")
	bus.Infof("second")
	bus.Infof("third")

	history, err := bus.History()
	require.NoError(t, err)
	require.Len(t, history, 3)
	assert.Equal(t, "third", history[0].Message)
	assert.Equal(t, "first", history[2].Message)
}

func TestBus_Clear(t *testing.T) {
	store := &memStore{}
	bus := NewBus(store)

	bus.Infof("to be cleared")
	require.NoError(t, bus.Clear())

	history, err := bus.History()
	require.NoError(t, err)
	assert.Empty(t, history)
}

func TestBus_nil_store(t *testing.T) {
	bus := NewBus(nil)

	var received []notify.Notification
	bus.Subscribe(func(n notify.Notification) {
		received = append(received, n)
	})

	bus.Errorf("no store")

	assert.Len(t, received, 1)
	assert.Equal(t, "no store", received[0].Message)

	history, err := bus.History()
	require.NoError(t, err)
	assert.Nil(t, history)

	assert.NoError(t, bus.Clear())
}

func TestBus_Publish_sets_created_at(t *testing.T) {
	bus := NewBus(&memStore{})

	var received notify.Notification
	bus.Subscribe(func(n notify.Notification) {
		received = n
	})

	bus.Infof("timestamp check")
	assert.False(t, received.CreatedAt.IsZero())
}
