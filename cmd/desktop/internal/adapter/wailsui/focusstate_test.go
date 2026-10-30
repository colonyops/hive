package wailsui

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestFocusStateReadsTheWindowsAfterTheDelay(t *testing.T) {
	var windowFocused atomic.Bool
	var notified atomic.Int32
	state := NewFocusState(time.Millisecond, windowFocused.Load, func(bool) { notified.Add(1) })

	assert.False(t, state.Get(), "no window has reported focus yet")

	windowFocused.Store(true)
	state.Changed()
	assert.Eventually(t, state.Get, time.Second, time.Millisecond)
	assert.EqualValues(t, 1, notified.Load())

	windowFocused.Store(false)
	state.Changed()
	assert.Eventually(t, func() bool { return !state.Get() }, time.Second, time.Millisecond)
	assert.EqualValues(t, 2, notified.Load())
}

func TestFocusStateStaysFocusedWhileMovingBetweenWindows(t *testing.T) {
	var notified atomic.Int32
	state := NewFocusState(time.Millisecond, func() bool { return true }, func(bool) { notified.Add(1) })

	state.Changed()
	assert.Eventually(t, state.Get, time.Second, time.Millisecond)

	// The resign and the become-key of a move between windows.
	state.Changed()
	state.Changed()
	time.Sleep(20 * time.Millisecond)

	assert.True(t, state.Get())
	assert.EqualValues(t, 1, notified.Load(), "the move is not a transition")
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
