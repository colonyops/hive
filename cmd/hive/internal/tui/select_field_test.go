package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
)

func TestSelectField(t *testing.T) {
	items := []SelectItem{
		{label: "alpha", value: 0},
		{label: "beta", value: 1},
		{label: "gamma", value: 2},
	}

	t.Run("SelectedIndex returns correct value", func(t *testing.T) {
		sf := NewSelectField("Test", items, 1)
		assert.Equal(t, 1, sf.SelectedIndex())
	})

	t.Run("SelectedIndex returns 0 for first item by default", func(t *testing.T) {
		sf := NewSelectField("Test", items, -1)
		// When no valid preselection, list defaults to first item
		assert.Equal(t, 0, sf.SelectedIndex())
	})

	t.Run("SelectedIndex returns -1 for empty list", func(t *testing.T) {
		sf := NewSelectField("Test", []SelectItem{}, -1)
		assert.Equal(t, -1, sf.SelectedIndex())
	})

	t.Run("out of bounds preselection defaults to first", func(t *testing.T) {
		sf := NewSelectField("Test", items, 99)
		// Out of bounds preselection is ignored, defaults to first
		assert.Equal(t, 0, sf.SelectedIndex())
	})

	t.Run("Focus and Blur change state", func(t *testing.T) {
		sf := NewSelectField("Test", items, 0)
		assert.False(t, sf.Focused())

		sf.Focus()
		assert.True(t, sf.Focused())

		sf.Blur()
		assert.False(t, sf.Focused())
	})

	t.Run("Update ignores messages when not focused", func(t *testing.T) {
		sf := NewSelectField("Test", items, 0)
		// Don't focus - updates should be ignored
		initialIdx := sf.SelectedIndex()

		// Try to move down
		sf, _ = sf.Update(tea.KeyPressMsg(tea.Key{Code: 'j'}))
		assert.Equal(t, initialIdx, sf.SelectedIndex())
	})

	t.Run("Update processes messages when focused", func(t *testing.T) {
		sf := NewSelectField("Test", items, 0)
		sf.Focus()

		// Move down with 'j'
		sf, _ = sf.Update(tea.KeyPressMsg(tea.Key{Code: 'j'}))
		assert.Equal(t, 1, sf.SelectedIndex())
	})

	t.Run("IsFiltering returns false initially", func(t *testing.T) {
		sf := NewSelectField("Test", items, 0)
		assert.False(t, sf.IsFiltering())
	})

	t.Run("View renders without panic", func(t *testing.T) {
		sf := NewSelectField("Test", items, 0)
		view := sf.View()
		assert.Contains(t, view, "Test") // Title should be visible
	})

	t.Run("View renders empty list without panic", func(t *testing.T) {
		sf := NewSelectField("Empty", []SelectItem{}, -1)
		view := sf.View()
		assert.Contains(t, view, "Empty")
	})
}
