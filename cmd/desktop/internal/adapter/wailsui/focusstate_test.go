package wailsui

import (
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
)

const testSettleDelay = 100 * time.Millisecond

func settleFocus() {
	time.Sleep(testSettleDelay)
	synctest.Wait()
}

func TestFocusStateReadsTheWindowsAfterTheDelay(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var windowFocused atomic.Bool
		var notified atomic.Int32
		state := NewFocusState(testSettleDelay, windowFocused.Load, func(bool) { notified.Add(1) })

		assert.False(t, state.Get(), "no window has reported focus yet")

		windowFocused.Store(true)
		state.Changed()
		time.Sleep(testSettleDelay / 2)
		synctest.Wait()
		assert.False(t, state.Get(), "the windows are not read before the delay")

		settleFocus()
		assert.True(t, state.Get())
		assert.EqualValues(t, 1, notified.Load())

		windowFocused.Store(false)
		state.Changed()
		settleFocus()
		assert.False(t, state.Get())
		assert.EqualValues(t, 2, notified.Load())
	})
}

func TestFocusStateStaysFocusedThroughTheGapOfAMoveBetweenWindows(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var windowFocused atomic.Bool
		var queries, notified atomic.Int32
		query := func() bool {
			queries.Add(1)
			return windowFocused.Load()
		}
		state := NewFocusState(testSettleDelay, query, func(bool) { notified.Add(1) })

		windowFocused.Store(true)
		state.Changed()
		settleFocus()
		queries.Store(0)
		notified.Store(0)

		// The resign event, while no window has focus, then the become-key.
		windowFocused.Store(false)
		state.Changed()
		time.Sleep(testSettleDelay / 2)
		windowFocused.Store(true)
		state.Changed()
		settleFocus()

		assert.True(t, state.Get())
		assert.EqualValues(t, 1, queries.Load(), "the two events collapse into one read")
		assert.Zero(t, notified.Load(), "the move is not a transition")
	})
}

func TestWindowServiceFocused(t *testing.T) {
	var windowFocused atomic.Bool
	state := NewFocusState(time.Millisecond, windowFocused.Load, func(bool) {})
	service := NewWindowService(state)

	assert.False(t, service.Focused(), "a hidden window is not focused")
	windowFocused.Store(true)
	state.Changed()
	assert.Eventually(t, service.Focused, time.Second, time.Millisecond)
}
