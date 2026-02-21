package form

import (
	"regexp"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
)

func TestFieldValidation_ValidateText(t *testing.T) {
	tests := []struct {
		name  string
		v     FieldValidation
		value string
		want  string
	}{
		{"no rules, empty", FieldValidation{}, "", ""},
		{"no rules, non-empty", FieldValidation{}, "hello", ""},
		{"required, empty", FieldValidation{Required: true}, "", "required"},
		{"required, non-empty", FieldValidation{Required: true}, "hello", ""},
		{"min_length, too short", FieldValidation{MinLength: 5}, "hi", "minimum 5 characters"},
		{"min_length, exact", FieldValidation{MinLength: 5}, "hello", ""},
		{"min_length, empty skips", FieldValidation{MinLength: 5}, "", ""},
		{"max_length, too long", FieldValidation{MaxLength: 3}, "hello", "maximum 3 characters"},
		{"max_length, exact", FieldValidation{MaxLength: 5}, "hello", ""},
		{"pattern, matches", FieldValidation{Pattern: regexp.MustCompile(`^\d+$`)}, "123", ""},
		{"pattern, no match", FieldValidation{Pattern: regexp.MustCompile(`^\d+$`)}, "abc", "must match pattern: ^\\d+$"},
		{"pattern, empty skips", FieldValidation{Pattern: regexp.MustCompile(`^\d+$`)}, "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.v.ValidateText(tt.value)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestFieldValidation_ValidateSelection(t *testing.T) {
	tests := []struct {
		name  string
		v     FieldValidation
		count int
		want  string
	}{
		{"no rules, zero", FieldValidation{}, 0, ""},
		{"required, zero", FieldValidation{Required: true}, 0, "at least one selection required"},
		{"required, non-zero", FieldValidation{Required: true}, 1, ""},
		{"min, too few", FieldValidation{Min: 2}, 1, "select at least 2"},
		{"min, exact", FieldValidation{Min: 2}, 2, ""},
		{"max, too many", FieldValidation{Max: 2}, 3, "select at most 2"},
		{"max, exact", FieldValidation{Max: 2}, 2, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.v.ValidateSelection(tt.count)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestDialog_ValidationBlocksSubmit(t *testing.T) {
	t.Run("required field blocks submit", func(t *testing.T) {
		f1 := NewTextField("Name", "", "", FieldValidation{Required: true})
		d := NewDialog("Test", []Field{f1}, []string{"name"})

		// Try to submit with empty field
		d.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyTab}))
		assert.False(t, d.Submitted(), "should not submit when required field is empty")
		assert.True(t, f1.Focused(), "focus should return to invalid field")
	})

	t.Run("valid field allows submit", func(t *testing.T) {
		f1 := NewTextField("Name", "", "Alice", FieldValidation{Required: true})
		d := NewDialog("Test", []Field{f1}, []string{"name"})

		d.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyTab}))
		assert.True(t, d.Submitted(), "should submit when required field has value")
	})

	t.Run("focuses first invalid field", func(t *testing.T) {
		f1 := NewTextField("Name", "", "Alice", FieldValidation{Required: true})
		f2 := NewTextField("Email", "", "", FieldValidation{Required: true})
		f3 := NewTextField("Phone", "", "", FieldValidation{Required: true})
		d := NewDialog("Test", []Field{f1, f2, f3}, []string{"name", "email", "phone"})

		// Tab to f2, then to f3
		d.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyTab}))
		d.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyTab}))
		assert.True(t, f3.Focused())

		// Try to submit (tab past f3) — should focus f2 (first invalid)
		d.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyTab}))
		assert.False(t, d.Submitted())
		assert.True(t, f2.Focused(), "should focus first invalid field (email)")
		assert.False(t, f3.Focused())
	})

	t.Run("min_length validation", func(t *testing.T) {
		f1 := NewTextField("Msg", "", "hi", FieldValidation{MinLength: 5})
		d := NewDialog("Test", []Field{f1}, []string{"msg"})

		d.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyTab}))
		assert.False(t, d.Submitted(), "should block submit when below min_length")
	})

	t.Run("error message renders in view", func(t *testing.T) {
		f1 := NewTextField("Name", "", "", FieldValidation{Required: true})
		d := NewDialog("Test", []Field{f1}, []string{"name"})

		// Trigger validation
		d.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyTab}))
		view := d.View()
		assert.Contains(t, view, "required")
	})
}
