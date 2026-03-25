package tui

import (
	lipgloss "charm.land/lipgloss/v2"
	"github.com/colonyops/hive/internal/core/styles"
)

// Modal represents a confirmation dialog.
type Modal struct {
	title           string
	message         string
	visible         bool
	confirmSelected bool // true = confirm button selected, false = cancel button selected

	// Dangerous mode: renders in error colours and requires the user to type a
	// specific word before the confirmation is accepted.
	dangerous   bool
	requireText string // word the user must type (e.g. "delete")
	typedText   string // text typed so far
}

// NewModal creates a new modal with the given title and message.
func NewModal(title, message string) Modal {
	return Modal{
		title:           title,
		message:         message,
		visible:         true,
		confirmSelected: true, // default to confirm button
	}
}

// NewDangerousModal creates a modal rendered in error colours that requires the
// user to type requireText (e.g. "delete") before confirming.
// The cancel button is selected by default to reduce accidental confirmation.
func NewDangerousModal(title, message, requireText string) Modal {
	return Modal{
		title:           title,
		message:         message,
		visible:         true,
		dangerous:       true,
		requireText:     requireText,
		confirmSelected: false, // default to cancel for dangerous actions
	}
}

// ToggleSelection switches the selected button (button mode only).
func (m *Modal) ToggleSelection() {
	m.confirmSelected = !m.confirmSelected
}

// ConfirmSelected returns true if the action should proceed.
// In text-input mode this means the typed text matches requireText; in button
// mode it means the confirm button is selected.
func (m Modal) ConfirmSelected() bool {
	if m.IsTextInput() {
		return m.typedText == m.requireText
	}
	return m.confirmSelected
}

// IsTextInput reports whether this modal uses text-confirmation instead of buttons.
func (m Modal) IsTextInput() bool {
	return m.requireText != ""
}

// AddChar appends a character to the typed text.
func (m *Modal) AddChar(ch string) {
	m.typedText += ch
}

// DeleteChar removes the last character from the typed text.
func (m *Modal) DeleteChar() {
	if len(m.typedText) > 0 {
		m.typedText = m.typedText[:len(m.typedText)-1]
	}
}

// Visible returns whether the modal should be displayed.
func (m Modal) Visible() bool {
	return m.visible
}

// render returns the fully styled modal box as a string.
// This is the logical content ready to be composited onto a background.
func (m Modal) render() string {
	var titleStyle, modalStyle lipgloss.Style
	if m.dangerous {
		titleStyle = styles.ModalDangerTitleStyle
		modalStyle = styles.ModalDangerStyle
	} else {
		titleStyle = styles.ModalTitleStyle
		modalStyle = styles.ModalStyle
	}

	var actionRow string
	if m.IsTextInput() {
		promptLine := "Type " + `"` + m.requireText + `"` + " to confirm:"

		inputContent := "> " + m.typedText + "█"
		var inputLine string
		if m.ConfirmSelected() {
			inputLine = styles.ModalInputReadyStyle.Render(inputContent)
		} else {
			inputLine = styles.ModalInputStyle.Render(inputContent)
		}

		helpText := styles.ModalHelpStyle.Render("enter confirm  esc cancel")
		actionRow = lipgloss.JoinVertical(lipgloss.Left,
			styles.ModalHelpStyle.Render(promptLine),
			inputLine,
			helpText,
		)
	} else {
		var confirmBtn, cancelBtn string
		if m.confirmSelected {
			confirmBtn = styles.ModalButtonSelectedStyle.Render("Confirm")
			cancelBtn = styles.ModalButtonStyle.Render("Cancel")
		} else {
			confirmBtn = styles.ModalButtonStyle.Render("Confirm")
			cancelBtn = styles.ModalButtonSelectedStyle.Render("Cancel")
		}
		buttons := lipgloss.JoinHorizontal(lipgloss.Center, confirmBtn, "  ", cancelBtn)
		actionRow = lipgloss.JoinVertical(lipgloss.Left,
			buttons,
			styles.ModalHelpStyle.Render("←/→ select  enter confirm  esc cancel"),
		)
	}

	content := lipgloss.JoinVertical(
		lipgloss.Left,
		titleStyle.Render(m.title),
		"",
		m.message,
		"",
		actionRow,
	)

	return modalStyle.Render(content)
}

// Overlay renders the modal as a layer over the given background content.
func (m Modal) Overlay(background string, width, height int) string {
	if !m.visible {
		return background
	}

	modal := m.render()

	// Use Compositor/Layer for true overlay (background remains visible).
	bgLayer := lipgloss.NewLayer(background)
	modalLayer := lipgloss.NewLayer(modal)

	modalW := lipgloss.Width(modal)
	modalH := lipgloss.Height(modal)
	centerX := (width - modalW) / 2
	centerY := (height - modalH) / 2
	modalLayer.X(centerX).Y(centerY).Z(1)

	compositor := lipgloss.NewCompositor(bgLayer, modalLayer)
	return compositor.Render()
}
