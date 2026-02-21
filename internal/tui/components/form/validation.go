package form

import (
	"fmt"
	"regexp"
)

// FieldValidation holds runtime validation rules for a form field.
type FieldValidation struct {
	Required  bool
	MinLength int
	MaxLength int
	Pattern   *regexp.Regexp
	Min       int // minimum selections (multi-select)
	Max       int // maximum selections (multi-select)
}

// ValidateText checks a text value against the validation rules.
func (v FieldValidation) ValidateText(value string) string {
	if v.Required && value == "" {
		return "required"
	}
	if value == "" {
		return ""
	}
	if v.MinLength > 0 && len(value) < v.MinLength {
		return fmt.Sprintf("minimum %d characters", v.MinLength)
	}
	if v.MaxLength > 0 && len(value) > v.MaxLength {
		return fmt.Sprintf("maximum %d characters", v.MaxLength)
	}
	if v.Pattern != nil && !v.Pattern.MatchString(value) {
		return fmt.Sprintf("must match pattern: %s", v.Pattern.String())
	}
	return ""
}

// ValidateSelection checks a selection count against the validation rules.
func (v FieldValidation) ValidateSelection(count int) string {
	if v.Required && count == 0 {
		return "at least one selection required"
	}
	if v.Min > 0 && count < v.Min {
		return fmt.Sprintf("select at least %d", v.Min)
	}
	if v.Max > 0 && count > v.Max {
		return fmt.Sprintf("select at most %d", v.Max)
	}
	return ""
}
