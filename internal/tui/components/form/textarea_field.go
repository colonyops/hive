package form

import (
	"charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"
	lipgloss "charm.land/lipgloss/v2"
	"github.com/colonyops/hive/internal/core/styles"
)

// TextAreaField is a multi-line text input form field.
type TextAreaField struct {
	input      textarea.Model
	label      string
	focused    bool
	validation FieldValidation
	errMsg     string
}

// NewTextAreaField creates a new multi-line text input field.
func NewTextAreaField(label, placeholder, defaultVal string, opts ...FieldValidation) *TextAreaField {
	ta := textarea.New()
	ta.Placeholder = placeholder
	ta.SetHeight(4)
	ta.SetWidth(40)

	if defaultVal != "" {
		ta.SetValue(defaultVal)
	}

	f := &TextAreaField{
		input: ta,
		label: label,
	}
	if len(opts) > 0 {
		f.validation = opts[0]
	}
	return f
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

	parts := []string{title, f.input.View()}
	if f.errMsg != "" {
		parts = append(parts, styles.FormErrorStyle.Render(f.errMsg))
	}
	content := lipgloss.JoinVertical(lipgloss.Left, parts...)

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

func (f *TextAreaField) Validate() string {
	f.errMsg = f.validation.ValidateText(f.input.Value())
	return f.errMsg
}
