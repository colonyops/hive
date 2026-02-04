package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	lipgloss "charm.land/lipgloss/v2"
)

// FinalizationAction represents the action to take on finalization.
type FinalizationAction int

const (
	FinalizationActionNone FinalizationAction = iota
	FinalizationActionClipboard
	FinalizationActionSendToAgent
)

// FinalizationModal shows options for finalizing a review.
type FinalizationModal struct {
	feedback    string
	selectedIdx int
	options     []finalizationOption
	width       int
	height      int
	confirmed   bool
	cancelled   bool
	hasAgentCmd bool // Whether send-claude command is available
}

type finalizationOption struct {
	label       string
	description string
	action      FinalizationAction
}

// NewFinalizationModal creates a modal for choosing finalization action.
func NewFinalizationModal(feedback string, hasAgentCmd bool, width, height int) FinalizationModal {
	options := []finalizationOption{
		{
			label:       "Copy to clipboard",
			description: "Copy review feedback to system clipboard",
			action:      FinalizationActionClipboard,
		},
	}

	// Only show send to agent option if command is available
	if hasAgentCmd {
		options = append(options, finalizationOption{
			label:       "Send to Claude agent",
			description: "Send feedback directly to Claude in current session",
			action:      FinalizationActionSendToAgent,
		})
	}

	return FinalizationModal{
		feedback:    feedback,
		selectedIdx: 0,
		options:     options,
		width:       width,
		height:      height,
		hasAgentCmd: hasAgentCmd,
	}
}

// Update handles input events for the finalization modal.
func (m FinalizationModal) Update(msg tea.Msg) (FinalizationModal, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "j", "down", "tab":
			m.selectedIdx = (m.selectedIdx + 1) % len(m.options)
		case "k", "up", "shift+tab":
			m.selectedIdx = (m.selectedIdx - 1 + len(m.options)) % len(m.options)
		case "enter":
			m.confirmed = true
		case "esc":
			m.cancelled = true
		}
	}
	return m, nil
}

// View renders the finalization modal.
func (m FinalizationModal) View() string {
	title := "Finalize Review"

	var content strings.Builder
	content.WriteString(title + "\n\n")
	content.WriteString("Review completed. Choose how to save your feedback:\n\n")

	// Render options
	for i, opt := range m.options {
		prefix := "  "
		style := lipgloss.NewStyle()

		if i == m.selectedIdx {
			prefix = "▸ "
			style = style.Foreground(colorBlue).Bold(true)
		}

		content.WriteString(prefix)
		content.WriteString(style.Render(opt.label))
		content.WriteString("\n")

		// Description in gray
		descStyle := lipgloss.NewStyle().Foreground(colorGray)
		content.WriteString("  " + descStyle.Render(opt.description) + "\n\n")
	}

	content.WriteString("\n")
	content.WriteString(lipgloss.NewStyle().Foreground(colorGray).Render("[j/k] select • [enter] confirm • [esc] cancel"))

	// Center the modal
	modalWidth := 60
	modalContent := content.String()

	modalStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorBlue).
		Padding(1, 2).
		Width(modalWidth)

	return lipgloss.Place(
		m.width,
		m.height,
		lipgloss.Center,
		lipgloss.Center,
		modalStyle.Render(modalContent),
	)
}

// Confirmed returns true if user confirmed the selection.
func (m FinalizationModal) Confirmed() bool {
	return m.confirmed
}

// Cancelled returns true if user cancelled.
func (m FinalizationModal) Cancelled() bool {
	return m.cancelled
}

// SelectedAction returns the selected finalization action.
func (m FinalizationModal) SelectedAction() FinalizationAction {
	if m.selectedIdx >= 0 && m.selectedIdx < len(m.options) {
		return m.options[m.selectedIdx].action
	}
	return FinalizationActionNone
}
