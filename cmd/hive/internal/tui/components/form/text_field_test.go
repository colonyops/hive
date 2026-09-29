package form

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
)

func TestTextField(t *testing.T) {
	t.Run("creation with defaults", func(t *testing.T) {
		f := NewTextField("Name", "enter name", "")
		assert.Equal(t, "Name", f.Label())
		assert.Empty(t, f.Value())
		assert.False(t, f.Focused())
	})

	t.Run("creation with default value", func(t *testing.T) {
		f := NewTextField("Name", "enter name", "hello")
		assert.Equal(t, "hello", f.Value())
	})

	t.Run("focus and blur", func(t *testing.T) {
		f := NewTextField("Name", "", "")
		assert.False(t, f.Focused())

		f.Focus()
		assert.True(t, f.Focused())

		f.Blur()
		assert.False(t, f.Focused())
	})

	t.Run("focus returns a cmd", func(t *testing.T) {
		f := NewTextField("Name", "", "")
		cmd := f.Focus()
		assert.NotNil(t, cmd)
	})

	t.Run("update ignored when not focused", func(t *testing.T) {
		f := NewTextField("Name", "", "")
		field, cmd := f.Update(tea.KeyPressMsg(tea.Key{Code: 'a'}))
		assert.Nil(t, cmd)
		assert.Empty(t, field.Value())
	})

	t.Run("value extraction after set", func(t *testing.T) {
		f := NewTextField("Name", "", "")
		f.input.SetValue("typed text")
		assert.Equal(t, "typed text", f.Value())
	})

	t.Run("view renders without panic", func(t *testing.T) {
		f := NewTextField("Name", "placeholder", "")
		view := f.View()
		assert.Contains(t, view, "Name")
	})

	t.Run("view changes with focus", func(t *testing.T) {
		f := NewTextField("Name", "", "")
		unfocused := f.View()

		f.Focus()
		focused := f.View()

		assert.NotEqual(t, unfocused, focused)
	})
}
