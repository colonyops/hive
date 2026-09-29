package form

import tea "charm.land/bubbletea/v2"

// Field is the interface implemented by all form field types.
type Field interface {
	Update(msg tea.Msg) (Field, tea.Cmd)
	View() string
	Focus() tea.Cmd
	Blur()
	Focused() bool
	Value() any    // string for text/textarea/select, []string for multi-select, domain types for presets
	Label() string // Display label for the field
}
