package review

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	lipgloss "charm.land/lipgloss/v2"
)

// CommentModal handles comment entry for selected text.
type CommentModal struct {
	textInput      textinput.Model
	lineRange      string // e.g., "Lines 10-15"
	contextPreview string // First 100 chars of selected text
	width          int
	height         int
	submitted      bool
	cancelled      bool
}

// NewCommentModal creates a new comment modal.
func NewCommentModal(startLine, endLine int, contextText string, width, height int) CommentModal {
	ti := textinput.New()
	ti.Placeholder = "Enter your review comment..."
	ti.Focus()
	ti.SetWidth(width - 10) // Account for padding and borders

	// Format line range
	lineRange := fmt.Sprintf("Lines %d-%d", startLine, endLine)
	if startLine == endLine {
		lineRange = fmt.Sprintf("Line %d", startLine)
	}

	// Format context preview - show first 20 lines + ... + last 3 lines
	contextPreview := formatContextPreview(contextText)

	return CommentModal{
		textInput:      ti,
		lineRange:      lineRange,
		contextPreview: contextPreview,
		width:          width,
		height:         height,
	}
}

// formatContextPreview formats multi-line context: first 20 lines + ... + last 3 lines.
func formatContextPreview(text string) string {
	lines := strings.Split(text, "\n")

	// If 23 lines or fewer, show all
	if len(lines) <= 23 {
		return text
	}

	// Show first 20 lines
	first := strings.Join(lines[:20], "\n")
	// Show last 3 lines
	last := strings.Join(lines[len(lines)-3:], "\n")

	return first + "\n...\n" + last
}

// Update handles messages.
func (m CommentModal) Update(msg tea.Msg) (CommentModal, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		switch keyMsg.String() {
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
func (m CommentModal) View() string {
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(colorBlue).
		MarginBottom(1)

	labelStyle := lipgloss.NewStyle().
		Foreground(colorGray).
		MarginBottom(1)

	contextStyle := lipgloss.NewStyle().
		Foreground(colorLightGray).
		Italic(true).
		MarginBottom(1)

	helpStyle := lipgloss.NewStyle().
		Foreground(colorGray).
		MarginTop(1)

	content := strings.Join([]string{
		titleStyle.Render("Add Review Comment"),
		labelStyle.Render(m.lineRange),
		contextStyle.Render(m.contextPreview),
		m.textInput.View(),
		helpStyle.Render("enter: submit • esc: cancel"),
	}, "\n")

	return content
}

// Submitted returns true if the comment was submitted.
func (m CommentModal) Submitted() bool {
	return m.submitted
}

// Cancelled returns true if the modal was cancelled.
func (m CommentModal) Cancelled() bool {
	return m.cancelled
}

// Value returns the entered comment text.
func (m CommentModal) Value() string {
	return m.textInput.Value()
}

// SetExistingComment pre-fills the modal with existing comment text for editing.
func (m *CommentModal) SetExistingComment(text string) {
	m.textInput.SetValue(text)
	// Position cursor at end of text
	m.textInput.CursorEnd()
}
