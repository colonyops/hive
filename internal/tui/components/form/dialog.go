package form

import (
	tea "charm.land/bubbletea/v2"
	lipgloss "charm.land/lipgloss/v2"
	"github.com/hay-kot/hive/internal/core/styles"
)

// filterer is an optional interface for fields that support list filtering.
type filterer interface {
	IsFiltering() bool
}

// Dialog is a form container that manages focus cycling, submission, and
// cancellation across a set of form fields.
type Dialog struct {
	fields       []Field
	variables    []string // parallel slice: variable name for each field
	focusedField int
	submitted    bool
	cancelled    bool
	Title        string
}

// NewDialog creates a form dialog with the given fields and variable names.
// The first field is focused automatically.
func NewDialog(title string, fields []Field, variables []string) *Dialog {
	d := &Dialog{
		fields:    fields,
		variables: variables,
		Title:     title,
	}
	if len(fields) > 0 {
		fields[0].Focus()
	}
	return d
}

// Update handles key input for the dialog, managing focus cycling and submit/cancel.
func (d *Dialog) Update(msg tea.Msg) (*Dialog, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return d.updateFocusedField(msg)
	}

	key := keyMsg.String()

	switch key {
	case "tab":
		return d.advanceFocus()
	case "shift+tab":
		return d.retreatFocus()
	case "enter":
		if d.isTextAreaFocused() {
			// Let textarea handle enter for newline insertion
			return d.updateFocusedField(msg)
		}
		return d.advanceFocus()
	case "esc":
		if d.isFocusedFieldFiltering() {
			// Let the field handle esc to exit filter mode
			return d.updateFocusedField(msg)
		}
		d.cancelled = true
		return d, nil
	}

	return d.updateFocusedField(msg)
}

// View renders all fields vertically with spacing and help text.
func (d *Dialog) View() string {
	var parts []string
	for i, field := range d.fields {
		if i > 0 {
			parts = append(parts, "")
		}
		parts = append(parts, field.View())
	}

	help := styles.TextMutedStyle.Render("tab: next  shift+tab: prev  enter: submit  esc: cancel")
	parts = append(parts, "", help)

	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}

// FormValues returns a map of variable names to field values.
func (d *Dialog) FormValues() map[string]any {
	result := make(map[string]any, len(d.fields))
	for i, field := range d.fields {
		result[d.variables[i]] = field.Value()
	}
	return result
}

// Submitted returns whether the form was submitted.
func (d *Dialog) Submitted() bool { return d.submitted }

// Cancelled returns whether the form was cancelled.
func (d *Dialog) Cancelled() bool { return d.cancelled }

func (d *Dialog) advanceFocus() (*Dialog, tea.Cmd) {
	if len(d.fields) == 0 {
		return d, nil
	}

	next := d.focusedField + 1
	if next >= len(d.fields) {
		// Past the last field — submit
		d.submitted = true
		return d, nil
	}

	d.fields[d.focusedField].Blur()
	d.focusedField = next
	cmd := d.fields[d.focusedField].Focus()
	return d, cmd
}

func (d *Dialog) retreatFocus() (*Dialog, tea.Cmd) {
	if len(d.fields) == 0 || d.focusedField == 0 {
		return d, nil
	}

	d.fields[d.focusedField].Blur()
	d.focusedField--
	cmd := d.fields[d.focusedField].Focus()
	return d, cmd
}

func (d *Dialog) updateFocusedField(msg tea.Msg) (*Dialog, tea.Cmd) {
	if len(d.fields) == 0 {
		return d, nil
	}

	var cmd tea.Cmd
	d.fields[d.focusedField], cmd = d.fields[d.focusedField].Update(msg)
	return d, cmd
}

func (d *Dialog) isTextAreaFocused() bool {
	if len(d.fields) == 0 {
		return false
	}
	_, ok := d.fields[d.focusedField].(*TextAreaField)
	return ok
}

func (d *Dialog) isFocusedFieldFiltering() bool {
	if len(d.fields) == 0 {
		return false
	}
	if f, ok := d.fields[d.focusedField].(filterer); ok {
		return f.IsFiltering()
	}
	return false
}
