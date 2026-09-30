package wailsui

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFocusStateTracksTransitionsOnly(t *testing.T) {
	state := NewFocusState()

	assert.False(t, state.Get(), "a hidden window is not focused")
	assert.False(t, state.Set(false), "unchanged state is a no-op")
	assert.False(t, state.Get())

	assert.True(t, state.Set(true))
	assert.True(t, state.Get())
	assert.False(t, state.Set(true), "unchanged state is a no-op")

	assert.True(t, state.Set(false))
	assert.False(t, state.Get())
}

func TestWindowServiceFocused(t *testing.T) {
	state := NewFocusState()
	service := NewWindowService(state)

	assert.False(t, service.Focused(), "a hidden window is not focused")
	state.Set(true)
	assert.True(t, service.Focused())
	state.Set(false)
	assert.False(t, service.Focused())
}
