package form

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
)

func TestTextAreaField(t *testing.T) {
	t.Run("creation with defaults", func(t *testing.T) {
		f := NewTextAreaField("Description", "enter text", "")
		assert.Equal(t, "Description", f.Label())
		assert.Empty(t, f.Value())
		assert.False(t, f.Focused())
	})

	t.Run("creation with default value", func(t *testing.T) {
		f := NewTextAreaField("Description", "", "hello world")
		assert.Equal(t, "hello world", f.Value())
	})

	t.Run("focus and blur", func(t *testing.T) {
		f := NewTextAreaField("Description", "", "")
		assert.False(t, f.Focused())

		f.Focus()
		assert.True(t, f.Focused())

		f.Blur()
		assert.False(t, f.Focused())
	})

	t.Run("update ignored when not focused", func(t *testing.T) {
		f := NewTextAreaField("Description", "", "")
		field, cmd := f.Update(tea.KeyPressMsg(tea.Key{Code: 'a'}))
		assert.Nil(t, cmd)
		assert.Empty(t, field.Value())
	})

	t.Run("view renders without panic", func(t *testing.T) {
		f := NewTextAreaField("Description", "placeholder", "")
		view := f.View()
		assert.Contains(t, view, "Description")
	})

	t.Run("view changes with focus", func(t *testing.T) {
		f := NewTextAreaField("Description", "", "")
		unfocused := f.View()

		f.Focus()
		focused := f.View()

		assert.NotEqual(t, unfocused, focused)
	})
}
