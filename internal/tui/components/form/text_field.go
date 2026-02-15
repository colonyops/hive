package form

import (
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	lipgloss "charm.land/lipgloss/v2"
	"github.com/colonyops/hive/internal/core/styles"
)

// TextField is a single-line text input form field.
type TextField struct {
	input   textinput.Model
	label   string
	focused bool
}

// NewTextField creates a new single-line text input field.
func NewTextField(label, placeholder, defaultVal string) *TextField {
	ti := textinput.New()
	ti.Placeholder = placeholder
	ti.Prompt = ""
	ti.SetWidth(40)

	if defaultVal != "" {
		ti.SetValue(defaultVal)
	}

	inputStyles := textinput.DefaultStyles(true)
	inputStyles.Cursor.Color = styles.ColorPrimary
	inputStyles.Focused.Placeholder = lipgloss.NewStyle().Foreground(styles.ColorMuted)
	inputStyles.Blurred.Placeholder = lipgloss.NewStyle().Foreground(styles.ColorMuted)
	ti.SetStyles(inputStyles)

	return &TextField{
		input: ti,
		label: label,
	}
}

func (f *TextField) Update(msg tea.Msg) (Field, tea.Cmd) {
	if !f.focused {
		return f, nil
	}

	var cmd tea.Cmd
	f.input, cmd = f.input.Update(msg)
	return f, cmd
}

func (f *TextField) View() string {
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

func (f *TextField) Focus() tea.Cmd {
	f.focused = true
	return f.input.Focus()
}

func (f *TextField) Blur() {
	f.focused = false
	f.input.Blur()
}

func (f *TextField) Focused() bool { return f.focused }
func (f *TextField) Value() any    { return f.input.Value() }
func (f *TextField) Label() string { return f.label }
