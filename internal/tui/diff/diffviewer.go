package diff

import (
	"context"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/bluekeyes/go-gitdiff/gitdiff"
	"github.com/charmbracelet/lipgloss"
	"github.com/hay-kot/hive/internal/core/styles"
)

// DiffViewerModel displays the diff content of a single file with syntax highlighting.
type DiffViewerModel struct {
	file    *gitdiff.File // Current file being viewed
	content string        // Rendered diff content (may include delta highlighting)
	lines   []string      // Split lines for scrolling
	offset  int           // Current scroll offset (top visible line)
	width   int
	height  int

	deltaAvailable bool // Whether delta is available for syntax highlighting
}

// NewDiffViewer creates a new diff viewer for the given file.
// If delta is available, syntax highlighting is enabled.
func NewDiffViewer(file *gitdiff.File) DiffViewerModel {
	// Check if delta is available
	deltaAvailable := CheckDeltaAvailable() == nil

	m := DiffViewerModel{
		file:           file,
		offset:         0,
		deltaAvailable: deltaAvailable,
	}

	// Generate initial content
	m.generateContent()

	return m
}

// generateContent builds the diff content string from the file.
// If delta is available, applies syntax highlighting.
func (m *DiffViewerModel) generateContent() {
	if m.file == nil {
		m.content = ""
		m.lines = nil
		return
	}

	// Build unified diff format
	var sb strings.Builder

	// Write diff header
	sb.WriteString("--- ")
	sb.WriteString(m.file.OldName)
	sb.WriteString("\n")
	sb.WriteString("+++ ")
	sb.WriteString(m.file.NewName)
	sb.WriteString("\n")

	// Write text fragments (hunks)
	for _, frag := range m.file.TextFragments {
		// Write hunk header
		sb.WriteString("@@ -")
		sb.WriteString(formatRange(frag.OldPosition, frag.OldLines))
		sb.WriteString(" +")
		sb.WriteString(formatRange(frag.NewPosition, frag.NewLines))
		sb.WriteString(" @@")
		if frag.Comment != "" {
			sb.WriteString(" ")
			sb.WriteString(frag.Comment)
		}
		sb.WriteString("\n")

		// Write lines
		for _, line := range frag.Lines {
			switch line.Op {
			case gitdiff.OpAdd:
				sb.WriteString("+")
			case gitdiff.OpDelete:
				sb.WriteString("-")
			case gitdiff.OpContext:
				sb.WriteString(" ")
			}
			sb.WriteString(line.Line)
			// Line already includes newline from parser
			if !strings.HasSuffix(line.Line, "\n") {
				sb.WriteString("\n")
			}
		}
	}

	diff := sb.String()

	// Apply delta highlighting if available
	if m.deltaAvailable {
		ctx := context.Background()
		highlighted, err := ExecDelta(ctx, diff)
		if err == nil {
			m.content = highlighted
		} else {
			// Fallback to plain diff if delta fails
			m.content = diff
		}
	} else {
		m.content = diff
	}

	// Split into lines for scrolling
	m.lines = strings.Split(m.content, "\n")
}

// formatRange formats a hunk range (position, length) for unified diff format.
func formatRange(pos, length int64) string {
	if length == 1 {
		return strconv.FormatInt(pos, 10)
	}
	return strconv.FormatInt(pos, 10) + "," + strconv.FormatInt(length, 10)
}

// Update handles keyboard input for scrolling.
func (m DiffViewerModel) Update(msg tea.Msg) (DiffViewerModel, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		maxOffset := max(0, len(m.lines)-m.height)

		switch keyMsg.String() {
		case "j", "down":
			// Scroll down one line
			if m.offset < maxOffset {
				m.offset++
			}
		case "k", "up":
			// Scroll up one line
			if m.offset > 0 {
				m.offset--
			}
		case "d", "ctrl+d":
			// Scroll down half page
			m.offset = min(m.offset+m.height/2, maxOffset)
		case "u", "ctrl+u":
			// Scroll up half page
			m.offset = max(m.offset-m.height/2, 0)
		case "g":
			// Jump to top
			m.offset = 0
		case "G":
			// Jump to bottom
			m.offset = maxOffset
		}
	}
	return m, nil
}

// View renders the visible portion of the diff.
func (m DiffViewerModel) View() string {
	if m.file == nil {
		return styles.TextMutedStyle.Render("No file selected")
	}

	if len(m.lines) == 0 {
		return styles.TextMutedStyle.Render("Empty diff")
	}

	// Calculate visible range
	endOffset := min(m.offset+m.height, len(m.lines))
	visibleLines := m.lines[m.offset:endOffset]

	content := strings.Join(visibleLines, "\n")

	return lipgloss.NewStyle().
		Width(m.width).
		Height(m.height).
		Render(content)
}

// SetSize updates the dimensions of the diff viewer.
func (m *DiffViewerModel) SetSize(width, height int) {
	m.width = width
	m.height = height
}

// SetFile updates the file being viewed and regenerates content.
func (m *DiffViewerModel) SetFile(file *gitdiff.File) {
	m.file = file
	m.offset = 0 // Reset scroll position
	m.generateContent()
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
