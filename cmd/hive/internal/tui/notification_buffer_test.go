package tui

import (
	"sync"
	"testing"
	"time"

	"github.com/colonyops/hive/internal/core/notify"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNotificationBuffer_Drain_empty_returnsNil(t *testing.T) {
	b := NewNotificationBuffer()
	assert.Nil(t, b.Drain())
}

func TestNotificationBuffer_PushDrain_orderAndClear(t *testing.T) {
	b := NewNotificationBuffer()
	b.Push(notify.Notification{Level: notify.LevelInfo, Message: "first"})
	b.Push(notify.Notification{Level: notify.LevelWarning, Message: "second"})

	items := b.Drain()
	require.Len(t, items, 2)
	assert.Equal(t, "first", items[0].Message)
	assert.Equal(t, "second", items[1].Message)
	assert.Nil(t, b.Drain())
}

func TestNotificationBuffer_Push_setsCreatedAtWhenZero(t *testing.T) {
	b := NewNotificationBuffer()
	b.Push(notify.Notification{Level: notify.LevelInfo, Message: "stamp me"})

	items := b.Drain()
	require.Len(t, items, 1)
	assert.False(t, items[0].CreatedAt.IsZero())
}

func TestNotificationBuffer_WaitForSignal_bufferedSignal(t *testing.T) {
	b := NewNotificationBuffer()
	b.Push(notify.Notification{Level: notify.LevelInfo, Message: "queued"})

	msg := b.WaitForSignal()()
	_, ok := msg.(drainNotificationsMsg)
	require.True(t, ok)
}

func TestNotificationBuffer_WaitForSignal_singleSignalDrainsAll(t *testing.T) {
	b := NewNotificationBuffer()
	b.Push(notify.Notification{Level: notify.LevelInfo, Message: "one"})
	b.Push(notify.Notification{Level: notify.LevelInfo, Message: "two"})

	msg := b.WaitForSignal()()
	_, ok := msg.(drainNotificationsMsg)
	require.True(t, ok)

	items := b.Drain()
	require.Len(t, items, 2)
	assert.Equal(t, "one", items[0].Message)
	assert.Equal(t, "two", items[1].Message)
}

func TestNotificationBuffer_ConcurrentPush_noLoss(t *testing.T) {
	b := NewNotificationBuffer()
	const count = 200

	var wg sync.WaitGroup
	for i := range count {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			b.Push(notify.Notification{Level: notify.LevelInfo, Message: time.Duration(i).String()})
		}(i)
	}
	wg.Wait()

	items := b.Drain()
	assert.Len(t, items, count)
}
