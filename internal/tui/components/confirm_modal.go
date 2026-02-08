package components

import (
	tea "charm.land/bubbletea/v2"

	"github.com/hay-kot/hive/internal/core/styles"
)

// ConfirmModal is a simple yes/no confirmation dialog.
type ConfirmModal struct {
	message   string
	confirmed bool
	cancelled bool
}

// NewConfirmModal creates a new confirmation modal.
func NewConfirmModal(message string) ConfirmModal {
	return ConfirmModal{
		message: message,
	}
}

// Update handles input for the confirmation modal.
func (m ConfirmModal) Update(msg tea.Msg) (ConfirmModal, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}

	switch keyMsg.String() {
	case "y", "Y", "enter":
		m.confirmed = true
		return m, nil
	case "n", "N", "esc":
		m.cancelled = true
		return m, nil
	}

	return m, nil
}

// View renders the confirmation modal.
func (m ConfirmModal) View() string {
	message := styles.ConfirmMessageStyle.Render(m.message)
	prompt := styles.TextPrimaryBoldStyle.Render("Continue? (y/n)")

	return message + "\n" + prompt
}

// Confirmed returns true if user confirmed.
func (m ConfirmModal) Confirmed() bool {
	return m.confirmed
}

// Cancelled returns true if user cancelled.
func (m ConfirmModal) Cancelled() bool {
	return m.cancelled
}
