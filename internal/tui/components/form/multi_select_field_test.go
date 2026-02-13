package form

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
)

func TestMultiSelectField(t *testing.T) {
	options := []string{"alpha", "beta", "gamma"}

	t.Run("creation defaults", func(t *testing.T) {
		f := NewMultiSelectFormField("Select", options)
		assert.Equal(t, "Select", f.Label())
		assert.False(t, f.Focused())
		// Nothing selected initially
		val, ok := f.Value().([]string)
		assert.True(t, ok)
		assert.Empty(t, val)
		assert.Empty(t, f.SelectedIndices())
	})

	t.Run("empty options", func(t *testing.T) {
		f := NewMultiSelectFormField("Select", []string{})
		val, ok := f.Value().([]string)
		assert.True(t, ok)
		assert.Empty(t, val)
	})

	t.Run("toggle selection with space", func(t *testing.T) {
		f := NewMultiSelectFormField("Select", options)
		f.Focus()

		// Toggle first item (cursor starts at index 0)
		f.Update(tea.KeyPressMsg(tea.Key{Code: ' '}))
		val, ok := f.Value().([]string)
		assert.True(t, ok)
		assert.Equal(t, []string{"alpha"}, val)
		assert.Equal(t, []int{0}, f.SelectedIndices())

		// Move down and toggle second item
		f.Update(tea.KeyPressMsg(tea.Key{Code: 'j'}))
		f.Update(tea.KeyPressMsg(tea.Key{Code: ' '}))
		val, ok = f.Value().([]string)
		assert.True(t, ok)
		assert.Equal(t, []string{"alpha", "beta"}, val)
		assert.Equal(t, []int{0, 1}, f.SelectedIndices())
	})

	t.Run("untoggle selection", func(t *testing.T) {
		f := NewMultiSelectFormField("Select", options)
		f.Focus()

		// Toggle on
		f.Update(tea.KeyPressMsg(tea.Key{Code: ' '}))
		assert.Equal(t, []int{0}, f.SelectedIndices())

		// Toggle off
		f.Update(tea.KeyPressMsg(tea.Key{Code: ' '}))
		assert.Empty(t, f.SelectedIndices())
	})

	t.Run("focus and blur", func(t *testing.T) {
		f := NewMultiSelectFormField("Select", options)
		assert.False(t, f.Focused())

		f.Focus()
		assert.True(t, f.Focused())

		f.Blur()
		assert.False(t, f.Focused())
	})

	t.Run("update ignored when not focused", func(t *testing.T) {
		f := NewMultiSelectFormField("Select", options)
		// Try to toggle without focus
		f.Update(tea.KeyPressMsg(tea.Key{Code: ' '}))
		assert.Empty(t, f.SelectedIndices())
	})

	t.Run("is not filtering initially", func(t *testing.T) {
		f := NewMultiSelectFormField("Select", options)
		assert.False(t, f.IsFiltering())
	})

	t.Run("view renders without panic", func(t *testing.T) {
		f := NewMultiSelectFormField("Select", options)
		view := f.View()
		assert.Contains(t, view, "Select")
	})

	t.Run("view renders empty list without panic", func(t *testing.T) {
		f := NewMultiSelectFormField("Empty", []string{})
		view := f.View()
		assert.Contains(t, view, "Empty")
	})
}
