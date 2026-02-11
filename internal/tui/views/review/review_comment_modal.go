package review

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"
	"github.com/hay-kot/hive/internal/core/styles"
)

// CommentModal handles multiline comment entry for selected text.
// Uses textarea for multiline input with the following keybindings:
//   - Enter: Insert newline
//   - Ctrl+Enter or Ctrl+S: Submit comment
//   - Esc: Cancel modal
type CommentModal struct {
	textArea       textarea.Model
	lineRange      string // e.g., "Lines 10-15"
	contextPreview string // First 100 chars of selected text
	width          int
	height         int
	submitted      bool
	cancelled      bool
}

// NewCommentModal creates a new comment modal.
// maxWidth constrains the modal width (e.g., from config.Review.CommentLineWidth).
func NewCommentModal(startLine, endLine int, contextText string, width, height int, maxWidth int) CommentModal {
	// Constrain modal width to maxWidth (with padding for borders)
	modalWidth := min(width-10, maxWidth+10) // +10 for padding/borders

	ta := textarea.New()
	ta.Placeholder = "Enter your review comment..."
	ta.Focus()
	ta.SetWidth(modalWidth - 10) // Account for padding and borders
	ta.SetHeight(5)              // 5 lines of input height
	ta.ShowLineNumbers = false
	ta.CharLimit = 1000

	// Enable multiline input and paste
	ta.KeyMap.InsertNewline.SetEnabled(true)
	ta.KeyMap.Paste.SetEnabled(true)

	// Format line range
	lineRange := fmt.Sprintf("Lines %d-%d", startLine, endLine)
	if startLine == endLine {
		lineRange = fmt.Sprintf("Line %d", startLine)
	}

	// Format context preview - show first 20 lines + ... + last 3 lines
	contextPreview := formatContextPreview(contextText)

	return CommentModal{
		textArea:       ta,
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
		case "ctrl+enter", "ctrl+s":
			// Submit with Ctrl+Enter or Ctrl+S
			if m.textArea.Value() != "" {
				m.submitted = true
				return m, nil
			}
		case "esc":
			// Cancel modal
			m.cancelled = true
			return m, nil
			// Note: Regular "enter" is handled by textarea to insert newline
		}
	}

	// Forward all other keys to textarea (including Enter for newline)
	var cmd tea.Cmd
	m.textArea, cmd = m.textArea.Update(msg)
	return m, cmd
}

// View renders the modal.
func (m CommentModal) View() string {
	content := strings.Join([]string{
		styles.ReviewCommentTitleStyle.Render("Add Review Comment"),
		styles.ReviewCommentLabelStyle.Render(m.lineRange),
		styles.ReviewCommentContextStyle.Render(m.contextPreview),
		m.textArea.View(),
		styles.ReviewCommentHelpStyle.Render("ctrl+enter/ctrl+s: submit • esc: cancel"),
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
	return m.textArea.Value()
}

// SetExistingComment pre-fills the modal with existing comment text for editing.
func (m *CommentModal) SetExistingComment(text string) {
	m.textArea.SetValue(text)
	// Position cursor at end of text
	m.textArea.CursorEnd()
}
