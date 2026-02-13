package form

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
)

func TestSelectFormField(t *testing.T) {
	options := []string{"alpha", "beta", "gamma"}

	t.Run("creation with no default", func(t *testing.T) {
		f := NewSelectFormField("Pick", options, "")
		assert.Equal(t, "Pick", f.Label())
		assert.False(t, f.Focused())
		// First item selected by default
		assert.Equal(t, "alpha", f.Value())
	})

	t.Run("creation with default value", func(t *testing.T) {
		f := NewSelectFormField("Pick", options, "beta")
		assert.Equal(t, "beta", f.Value())
	})

	t.Run("creation with invalid default falls back to first", func(t *testing.T) {
		f := NewSelectFormField("Pick", options, "nonexistent")
		assert.Equal(t, "alpha", f.Value())
	})

	t.Run("empty options", func(t *testing.T) {
		f := NewSelectFormField("Pick", []string{}, "")
		assert.Empty(t, f.Value())
	})

	t.Run("focus and blur", func(t *testing.T) {
		f := NewSelectFormField("Pick", options, "")
		assert.False(t, f.Focused())

		f.Focus()
		assert.True(t, f.Focused())

		f.Blur()
		assert.False(t, f.Focused())
	})

	t.Run("update ignored when not focused", func(t *testing.T) {
		f := NewSelectFormField("Pick", options, "")
		initial := f.Value()

		field, _ := f.Update(tea.KeyPressMsg(tea.Key{Code: 'j'}))
		assert.Equal(t, initial, field.Value())
	})

	t.Run("update processes input when focused", func(t *testing.T) {
		f := NewSelectFormField("Pick", options, "")
		f.Focus()

		// Move down with j
		field, _ := f.Update(tea.KeyPressMsg(tea.Key{Code: 'j'}))
		assert.Equal(t, "beta", field.Value())
	})

	t.Run("is not filtering initially", func(t *testing.T) {
		f := NewSelectFormField("Pick", options, "")
		assert.False(t, f.IsFiltering())
	})

	t.Run("view renders without panic", func(t *testing.T) {
		f := NewSelectFormField("Pick", options, "")
		view := f.View()
		assert.Contains(t, view, "Pick")
	})

	t.Run("view renders empty list without panic", func(t *testing.T) {
		f := NewSelectFormField("Empty", []string{}, "")
		view := f.View()
		assert.Contains(t, view, "Empty")
	})
}
