package form

import (
	"charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"
	lipgloss "charm.land/lipgloss/v2"
	"github.com/colonyops/hive/internal/core/styles"
)

// TextAreaField is a multi-line text input form field.
type TextAreaField struct {
	input   textarea.Model
	label   string
	focused bool
}

// NewTextAreaField creates a new multi-line text input field.
func NewTextAreaField(label, placeholder, defaultVal string) *TextAreaField {
	ta := textarea.New()
	ta.Placeholder = placeholder
	ta.SetHeight(4)
	ta.SetWidth(40)

	if defaultVal != "" {
		ta.SetValue(defaultVal)
	}

	return &TextAreaField{
		input: ta,
		label: label,
	}
}

func (f *TextAreaField) Update(msg tea.Msg) (Field, tea.Cmd) {
	if !f.focused {
		return f, nil
	}

	var cmd tea.Cmd
	f.input, cmd = f.input.Update(msg)
	return f, cmd
}

func (f *TextAreaField) View() string {
	titleStyle := styles.TextMutedStyle
	if f.focused {
		titleStyle = styles.FormTitleStyle
	}
	title := titleStyle.Render(f.label)

	content := lipgloss.JoinVertical(lipgloss.Left, title, f.input.View())

	borderStyle := styles.FormFieldStyle
	if f.focused {
		borderStyle = styles.FormFieldFocusedStyle
	}

	return borderStyle.Render(content)
}

func (f *TextAreaField) Focus() tea.Cmd {
	f.focused = true
	return f.input.Focus()
}

func (f *TextAreaField) Blur() {
	f.focused = false
	f.input.Blur()
}

func (f *TextAreaField) Focused() bool { return f.focused }
func (f *TextAreaField) Value() any    { return f.input.Value() }
func (f *TextAreaField) Label() string { return f.label }
