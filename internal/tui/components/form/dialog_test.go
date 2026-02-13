package form

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDialog(t *testing.T) {
	t.Run("creation focuses first field", func(t *testing.T) {
		f1 := NewTextField("Name", "", "")
		f2 := NewTextField("Email", "", "")
		d := NewDialog("Test", []Field{f1, f2}, []string{"name", "email"})

		assert.True(t, f1.Focused())
		assert.False(t, f2.Focused())
		assert.False(t, d.Submitted())
		assert.False(t, d.Cancelled())
	})

	t.Run("empty dialog", func(t *testing.T) {
		d := NewDialog("Empty", []Field{}, []string{})
		assert.False(t, d.Submitted())
		assert.False(t, d.Cancelled())
		assert.Empty(t, d.FormValues())
	})

	t.Run("tab advances focus", func(t *testing.T) {
		f1 := NewTextField("A", "", "")
		f2 := NewTextField("B", "", "")
		f3 := NewTextField("C", "", "")
		d := NewDialog("Test", []Field{f1, f2, f3}, []string{"a", "b", "c"})

		// Tab: A -> B
		d.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyTab}))
		assert.False(t, f1.Focused())
		assert.True(t, f2.Focused())
		assert.False(t, f3.Focused())

		// Tab: B -> C
		d.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyTab}))
		assert.False(t, f1.Focused())
		assert.False(t, f2.Focused())
		assert.True(t, f3.Focused())
	})

	t.Run("tab past last field submits", func(t *testing.T) {
		f1 := NewTextField("A", "", "")
		d := NewDialog("Test", []Field{f1}, []string{"a"})

		d.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyTab}))
		assert.True(t, d.Submitted())
	})

	t.Run("shift+tab retreats focus", func(t *testing.T) {
		f1 := NewTextField("A", "", "")
		f2 := NewTextField("B", "", "")
		d := NewDialog("Test", []Field{f1, f2}, []string{"a", "b"})

		// Tab forward to B
		d.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyTab}))
		assert.True(t, f2.Focused())

		// Shift+Tab back to A
		d.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyTab, Mod: tea.ModShift}))
		assert.True(t, f1.Focused())
		assert.False(t, f2.Focused())
	})

	t.Run("shift+tab on first field stays", func(t *testing.T) {
		f1 := NewTextField("A", "", "")
		d := NewDialog("Test", []Field{f1}, []string{"a"})

		d.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyTab, Mod: tea.ModShift}))
		assert.True(t, f1.Focused())
		assert.False(t, d.Submitted())
	})

	t.Run("enter advances focus on non-textarea", func(t *testing.T) {
		f1 := NewTextField("A", "", "")
		f2 := NewTextField("B", "", "")
		d := NewDialog("Test", []Field{f1, f2}, []string{"a", "b"})

		d.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
		assert.False(t, f1.Focused())
		assert.True(t, f2.Focused())
	})

	t.Run("enter on last non-textarea field submits", func(t *testing.T) {
		f1 := NewTextField("A", "", "")
		d := NewDialog("Test", []Field{f1}, []string{"a"})

		d.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
		assert.True(t, d.Submitted())
	})

	t.Run("enter on textarea does not advance", func(t *testing.T) {
		f1 := NewTextAreaField("Body", "", "")
		f2 := NewTextField("Name", "", "")
		d := NewDialog("Test", []Field{f1, f2}, []string{"body", "name"})

		// Enter should be forwarded to textarea, not advance focus
		d.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
		assert.True(t, f1.Focused())
		assert.False(t, f2.Focused())
		assert.False(t, d.Submitted())
	})

	t.Run("escape cancels", func(t *testing.T) {
		f1 := NewTextField("A", "", "")
		d := NewDialog("Test", []Field{f1}, []string{"a"})

		d.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEscape}))
		assert.True(t, d.Cancelled())
		assert.False(t, d.Submitted())
	})

	t.Run("FormValues extracts all values", func(t *testing.T) {
		f1 := NewTextField("Name", "", "Alice")
		f2 := NewTextField("Email", "", "alice@test.com")
		d := NewDialog("Test", []Field{f1, f2}, []string{"name", "email"})

		vals := d.FormValues()
		assert.Equal(t, "Alice", vals["name"])
		assert.Equal(t, "alice@test.com", vals["email"])
	})

	t.Run("FormValues with multi-select", func(t *testing.T) {
		f1 := NewMultiSelectFormField("Pick", []string{"a", "b", "c"})
		d := NewDialog("Test", []Field{f1}, []string{"items"})

		// Toggle first item
		f1.Focus()
		f1.Update(tea.KeyPressMsg(tea.Key{Code: ' '}))

		vals := d.FormValues()
		selected, ok := vals["items"].([]string)
		require.True(t, ok)
		assert.Equal(t, []string{"a"}, selected)
	})

	t.Run("view renders without panic", func(t *testing.T) {
		f1 := NewTextField("Name", "enter name", "")
		f2 := NewSelectFormField("Color", []string{"red", "blue"}, "")
		d := NewDialog("Test Form", []Field{f1, f2}, []string{"name", "color"})

		view := d.View()
		assert.Contains(t, view, "Name")
		assert.Contains(t, view, "Color")
		assert.Contains(t, view, "tab")
	})

	t.Run("view with empty dialog", func(t *testing.T) {
		d := NewDialog("Empty", []Field{}, []string{})
		view := d.View()
		assert.Contains(t, view, "tab")
	})
}
