package review

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	lipgloss "charm.land/lipgloss/v2"
	"github.com/colonyops/hive/internal/core/styles"
	"github.com/rs/zerolog/log"
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
}

type finalizationOption struct {
	label       string
	description string
	action      FinalizationAction
}

// NewFinalizationModal creates a modal for choosing finalization action.
func NewFinalizationModal(feedback string, width, height int) FinalizationModal {
	options := []finalizationOption{
		{
			label:       "Copy to clipboard",
			description: "Copy review feedback to system clipboard",
			action:      FinalizationActionClipboard,
		},
	}

	return FinalizationModal{
		feedback:    feedback,
		selectedIdx: 0,
		options:     options,
		width:       width,
		height:      height,
	}
}

// Update handles input events for the finalization modal.
func (m FinalizationModal) Update(msg tea.Msg) (FinalizationModal, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		keyStr := keyMsg.String()
		log.Debug().
			Str("key", keyStr).
			Int("current_idx", m.selectedIdx).
			Msg("finalization modal received key")

		switch keyStr {
		case "j", "down", "tab":
			m.selectedIdx = (m.selectedIdx + 1) % len(m.options)
			log.Debug().Int("new_idx", m.selectedIdx).Msg("moved selection down")
		case "k", "up", "shift+tab":
			m.selectedIdx = (m.selectedIdx - 1 + len(m.options)) % len(m.options)
			log.Debug().Int("new_idx", m.selectedIdx).Msg("moved selection up")
		case "enter":
			m.confirmed = true
			log.Debug().Msg("confirmed")
		case "esc":
			m.cancelled = true
			log.Debug().Msg("cancelled")
		}
	}
	return m, nil
}

// View renders the finalization modal content (without overlay).
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
			style = styles.ReviewFinalizeOptionStyle
		}

		content.WriteString(prefix)
		content.WriteString(style.Render(opt.label))
		content.WriteString("\n")

		// Description in gray
		content.WriteString("  " + styles.TextMutedStyle.Render(opt.description) + "\n\n")
	}

	content.WriteString("\n")
	content.WriteString(styles.TextMutedStyle.Render("[j/k] select • [enter] confirm • [esc] cancel"))

	// Style the modal
	modalContent := content.String()

	return styles.ReviewFinalizeModalStyle.Render(modalContent)
}

// Overlay renders the modal over the background content.
func (m FinalizationModal) Overlay(background string) string {
	modal := m.View()

	// Use Compositor/Layer for true overlay
	bgLayer := lipgloss.NewLayer(background)
	modalLayer := lipgloss.NewLayer(modal)

	// Center the modal (clamped to 0 for tiny terminals)
	modalW := lipgloss.Width(modal)
	modalH := lipgloss.Height(modal)
	centerX := max((m.width-modalW)/2, 0)
	centerY := max((m.height-modalH)/2, 0)
	modalLayer.X(centerX).Y(centerY).Z(1)

	compositor := lipgloss.NewCompositor(bgLayer, modalLayer)
	return compositor.Render()
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
