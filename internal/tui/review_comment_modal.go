package tui

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	lipgloss "charm.land/lipgloss/v2"
)

// ReviewCommentModal handles comment entry for selected text.
type ReviewCommentModal struct {
	textInput    textinput.Model
	lineRange    string // e.g., "Lines 10-15"
	contextPreview string // First 100 chars of selected text
	width        int
	height       int
	submitted    bool
	cancelled    bool
}

// NewReviewCommentModal creates a new comment modal.
func NewReviewCommentModal(startLine, endLine int, contextText string, width, height int) ReviewCommentModal {
	ti := textinput.New()
	ti.Placeholder = "Enter your review comment..."
	ti.Focus()
	ti.SetWidth(width - 10) // Account for padding and borders

	// Format line range
	lineRange := fmt.Sprintf("Lines %d-%d", startLine, endLine)
	if startLine == endLine {
		lineRange = fmt.Sprintf("Line %d", startLine)
	}

	// Truncate context preview
	contextPreview := contextText
	if len(contextPreview) > 100 {
		contextPreview = contextPreview[:97] + "..."
	}

	return ReviewCommentModal{
		textInput:      ti,
		lineRange:      lineRange,
		contextPreview: contextPreview,
		width:          width,
		height:         height,
	}
}

// Update handles messages.
func (m ReviewCommentModal) Update(msg tea.Msg) (ReviewCommentModal, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "enter":
			if m.textInput.Value() != "" {
				m.submitted = true
				return m, nil
			}
		case "esc":
			m.cancelled = true
			return m, nil
		}
	}

	var cmd tea.Cmd
	m.textInput, cmd = m.textInput.Update(msg)
	return m, cmd
}

// View renders the modal.
func (m ReviewCommentModal) View() string {
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#7aa2f7")).
		MarginBottom(1)

	labelStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#565f89")).
		MarginBottom(1)

	contextStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#9aa5ce")).
		Italic(true).
		MarginBottom(1)

	helpStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#565f89")).
		MarginTop(1)

	content := strings.Join([]string{
		titleStyle.Render("Add Review Comment"),
		labelStyle.Render(m.lineRange),
		contextStyle.Render("\"" + m.contextPreview + "\""),
		m.textInput.View(),
		helpStyle.Render("enter: submit • esc: cancel"),
	}, "\n")

	return content
}

// Submitted returns true if the comment was submitted.
func (m ReviewCommentModal) Submitted() bool {
	return m.submitted
}

// Cancelled returns true if the modal was cancelled.
func (m ReviewCommentModal) Cancelled() bool {
	return m.cancelled
}

// Value returns the entered comment text.
func (m ReviewCommentModal) Value() string {
	return m.textInput.Value()
}
