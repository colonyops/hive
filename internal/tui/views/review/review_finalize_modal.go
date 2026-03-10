package review

import (
	"strings"

	"charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"
	lipgloss "charm.land/lipgloss/v2"
	"github.com/colonyops/hive/internal/core/styles"
)

// FinalizationModal collects an optional general note before saving the review
// and copying it to the clipboard.
type FinalizationModal struct {
	feedback    string
	width       int
	height      int
	confirmed   bool
	cancelled   bool
	generalNote textarea.Model
}

// NewFinalizationModal creates a new finalization modal.
func NewFinalizationModal(feedback string, width, height int) FinalizationModal {
	contentWidth := max(min(width-14, 100), 40) // 14 = border(2) + padding(4*2) + margin
	ta := textarea.New()
	ta.Placeholder = "Optional general notes about this document..."
	ta.SetWidth(contentWidth)
	ta.SetHeight(4)
	ta.ShowLineNumbers = false
	ta.CharLimit = 2000
	ta.KeyMap.InsertNewline.SetEnabled(true)
	ta.KeyMap.Paste.SetEnabled(true)
	ta.Focus()

	return FinalizationModal{
		feedback:    feedback,
		width:       width,
		height:      height,
		generalNote: ta,
	}
}

// Update handles input events for the finalization modal.
func (m FinalizationModal) Update(msg tea.Msg) (FinalizationModal, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyPressMsg); ok {
		switch keyMsg.String() {
		case "esc":
			m.cancelled = true
			return m, nil
		case "ctrl+s":
			m.confirmed = true
			return m, nil
		}
	}

	// Forward all other keys to textarea.
	var cmd tea.Cmd
	m.generalNote, cmd = m.generalNote.Update(msg)
	return m, cmd
}

// View renders the finalization modal content (without overlay).
func (m FinalizationModal) View() string {
	contentWidth := max(min(m.width-14, 100), 40)

	var content strings.Builder

	content.WriteString("Finalize Review\n\n")
	content.WriteString(styles.TextMutedStyle.Render("General Notes") + "\n")
	content.WriteString(m.generalNote.View())
	content.WriteString("\n\n")
	content.WriteString(styles.TextMutedStyle.Render("ctrl+s: save & copy to clipboard  •  esc: cancel"))

	return styles.ReviewFinalizeModalStyle.Width(contentWidth).Render(content.String())
}

// Overlay renders the modal centered over the background content.
func (m FinalizationModal) Overlay(background string) string {
	modal := m.View()

	bgLayer := lipgloss.NewLayer(background)
	modalLayer := lipgloss.NewLayer(modal)

	modalW := lipgloss.Width(modal)
	modalH := lipgloss.Height(modal)
	centerX := max((m.width-modalW)/2, 0)
	centerY := max((m.height-modalH)/2, 0)
	modalLayer.X(centerX).Y(centerY).Z(1)

	compositor := lipgloss.NewCompositor(bgLayer, modalLayer)
	return compositor.Render()
}

// Confirmed returns true if the user confirmed.
func (m FinalizationModal) Confirmed() bool { return m.confirmed }

// Cancelled returns true if the user cancelled.
func (m FinalizationModal) Cancelled() bool { return m.cancelled }

// GeneralComment returns the trimmed general notes text.
func (m FinalizationModal) GeneralComment() string {
	return strings.TrimSpace(m.generalNote.Value())
}
