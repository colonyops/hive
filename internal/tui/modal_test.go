package tui

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestModal_NewDefaults(t *testing.T) {
	m := NewModal("Title", "Are you sure?")
	assert.True(t, m.Visible())
	assert.True(t, m.ConfirmSelected()) // defaults to confirm
	assert.Equal(t, "Title", m.title)
	assert.Equal(t, "Are you sure?", m.message)
}

func TestModal_Overlay_NotVisible(t *testing.T) {
	m := Modal{} // visible defaults to false
	bg := "background content"
	assert.Equal(t, bg, m.Overlay(bg, 80, 24))
}

func TestModal_ToggleSelection(t *testing.T) {
	m := NewModal("", "")
	assert.True(t, m.ConfirmSelected())

	m.ToggleSelection()
	assert.False(t, m.ConfirmSelected())

	m.ToggleSelection()
	assert.True(t, m.ConfirmSelected())
}
