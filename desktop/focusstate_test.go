package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFocusStateTracksTransitionsOnly(t *testing.T) {
	state := newFocusState()

	assert.False(t, state.get(), "a hidden window is not focused")
	assert.False(t, state.set(false), "unchanged state is a no-op")
	assert.False(t, state.get())

	assert.True(t, state.set(true))
	assert.True(t, state.get())
	assert.False(t, state.set(true), "unchanged state is a no-op")

	assert.True(t, state.set(false))
	assert.False(t, state.get())
}

func TestWindowServiceFocused(t *testing.T) {
	state := newFocusState()
	service := NewWindowService(state)

	assert.False(t, service.Focused(), "a hidden window is not focused")
	state.set(true)
	assert.True(t, service.Focused())
	state.set(false)
	assert.False(t, service.Focused())
}
