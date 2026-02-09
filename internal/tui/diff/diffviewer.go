package diff

import (
	"context"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	lipgloss "charm.land/lipgloss/v2"
	"github.com/bluekeyes/go-gitdiff/gitdiff"
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

	// Visual selection state
	selectionMode  bool // Whether visual selection mode is active
	selectionStart int  // Line where selection started (0-indexed, relative to file)
	cursorLine     int  // Current cursor position (0-indexed, relative to file)
}

const (
	// headerHeight is the fixed height of the file info header (2 lines of content + 1 border line)
	headerHeight = 3
)

// NewDiffViewer creates a new diff viewer for the given file.
// If delta is available, syntax highlighting is enabled.
func NewDiffViewer(file *gitdiff.File) DiffViewerModel {
	// Check if delta is available
	deltaAvailable := CheckDeltaAvailable() == nil

	m := DiffViewerModel{
		file:           file,
		offset:         0,
		deltaAvailable: deltaAvailable,
		selectionMode:  false,
		selectionStart: 0,
		cursorLine:     0,
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

// Update handles keyboard input for scrolling and visual selection.
func (m DiffViewerModel) Update(msg tea.Msg) (DiffViewerModel, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		// Calculate content height (accounting for header)
		contentHeight := m.height - headerHeight
		if contentHeight < 1 {
			contentHeight = 1
		}

		maxLine := max(0, len(m.lines)-1)
		maxOffset := max(0, len(m.lines)-contentHeight)

		switch keyMsg.String() {
		case "v":
			// Toggle visual selection mode
			if !m.selectionMode {
				// Enter visual mode
				m.selectionMode = true
				m.selectionStart = m.cursorLine
			} else {
				// Exit visual mode
				m.selectionMode = false
			}

		case "j", "down":
			// Move cursor down one line
			if m.cursorLine < maxLine {
				m.cursorLine++
				// Scroll viewport if cursor moves beyond visible area
				if m.cursorLine >= m.offset+contentHeight {
					m.offset = min(m.offset+1, maxOffset)
				}
			}

		case "k", "up":
			// Move cursor up one line
			if m.cursorLine > 0 {
				m.cursorLine--
				// Scroll viewport if cursor moves before visible area
				if m.cursorLine < m.offset {
					m.offset = max(m.offset-1, 0)
				}
			}

		case "d", "ctrl+d":
			// Move cursor down half page
			oldCursor := m.cursorLine
			m.cursorLine = min(m.cursorLine+contentHeight/2, maxLine)
			// Scroll to keep cursor in view
			m.offset = min(m.offset+(m.cursorLine-oldCursor), maxOffset)

		case "u", "ctrl+u":
			// Move cursor up half page
			oldCursor := m.cursorLine
			m.cursorLine = max(m.cursorLine-contentHeight/2, 0)
			// Scroll to keep cursor in view
			m.offset = max(m.offset-(oldCursor-m.cursorLine), 0)

		case "g":
			// Jump cursor to top
			m.cursorLine = 0
			m.offset = 0

		case "G":
			// Jump cursor to bottom
			m.cursorLine = maxLine
			m.offset = maxOffset

		case "escape":
			// Exit visual mode
			if m.selectionMode {
				m.selectionMode = false
			}
		}
	}
	return m, nil
}

// View renders the visible portion of the diff with selection, cursor highlighting, and gutter.
func (m DiffViewerModel) View() string {
	// Handle empty states
	if m.file == nil {
		return m.renderEmptyState("No File Selected", "Select a file from the tree to view its diff")
	}

	if len(m.lines) == 0 {
		return m.renderEmptyState("Empty Diff", "This file has no changes")
	}

	// Render file info header (single line with filename and stats + separator)
	fileHeader := m.renderFileInfoHeader()
	fileHeaderHeight := 2 // filename line + separator

	// Calculate content height
	contentHeight := m.height - fileHeaderHeight
	if contentHeight < 1 {
		contentHeight = 1
	}

	// Calculate visible range for content
	endOffset := min(m.offset+contentHeight, len(m.lines))

	// Build output with gutter, selection/cursor highlighting
	var styledLines []string
	for i := m.offset; i < endOffset; i++ {
		line := m.lines[i]

		// Add gutter with cursor indicator
		var gutter string
		if i == m.cursorLine {
			gutter = styles.TextPrimaryStyle.Render("▶") + "│ "
		} else {
			gutter = " │ "
		}

		// Determine if this line should be styled
		isSelected := m.isLineSelected(i)

		// Apply selection style (cursor is shown via gutter)
		if isSelected {
			line = styles.ReviewSelectionStyle.Render(line)
		}

		styledLines = append(styledLines, gutter+line)
	}

	content := strings.Join(styledLines, "\n")

	// Combine file header and content
	return lipgloss.JoinVertical(lipgloss.Left, fileHeader, content)
}

// renderEmptyState renders a centered message when there's no content to display.
func (m DiffViewerModel) renderEmptyState(title, hint string) string {
	titleStyled := styles.TextForegroundBoldStyle.Render(title)
	hintStyled := styles.TextMutedStyle.Render(hint)

	content := lipgloss.JoinVertical(lipgloss.Center,
		"",
		titleStyled,
		hintStyled,
		"",
	)

	return lipgloss.Place(m.width, m.height,
		lipgloss.Center, lipgloss.Center,
		content)
}

// renderFileInfoHeader renders a compact file info header with filename and stats.
func (m DiffViewerModel) renderFileInfoHeader() string {
	if m.file == nil {
		return ""
	}

	// Get file name
	fileName := m.file.NewName
	if fileName == "" || fileName == "/dev/null" {
		fileName = m.file.OldName
	}

	// Get file icon
	icon := m.getFileIcon(fileName)

	// Calculate stats
	additions, deletions := m.calculateDiffStats()

	// Build info line: icon filename (-deletions, +additions)
	statsStr := fmt.Sprintf("(-%d, +%d)", deletions, additions)
	infoLine := icon + " " + fileName + " " + styles.TextMutedStyle.Render(statsStr)

	// Separator line - full width to match content gutters
	separator := strings.Repeat("─", m.width-1)

	return infoLine + "\n" + styles.TextMutedStyle.Render(separator)
}

// renderPanelHeader renders the panel header with branding (deprecated - now in unified header).
// calculateDiffStats counts additions and deletions in the current file.
func (m DiffViewerModel) calculateDiffStats() (int, int) {
	if m.file == nil {
		return 0, 0
	}

	var additions, deletions int
	for _, frag := range m.file.TextFragments {
		for _, line := range frag.Lines {
			switch line.Op {
			case gitdiff.OpAdd:
				additions++
			case gitdiff.OpDelete:
				deletions++
			}
		}
	}

	return additions, deletions
}

// getFileIcon returns an icon for the file based on its extension.
func (m DiffViewerModel) getFileIcon(path string) string {
	if path == "" {
		return styles.IconFileDefault
	}

	// Extract extension
	ext := filepath.Ext(path)
	base := filepath.Base(path)

	// Check special filenames first
	switch strings.ToLower(base) {
	case "readme.md", "readme":
		return styles.IconFileReadme
	case "dockerfile":
		return styles.IconFileDocker
	case "makefile":
		return styles.IconFileMakefile
	}

	// Check by extension
	switch strings.ToLower(ext) {
	case ".go":
		return styles.IconFileGo
	case ".js":
		return styles.IconFileJS
	case ".ts":
		return styles.IconFileTS
	case ".tsx":
		return styles.IconFileTSX
	case ".jsx":
		return styles.IconFileJSX
	case ".py":
		return styles.IconFilePython
	case ".md":
		return styles.IconFileMarkdown
	case ".json":
		return styles.IconFileJSON
	case ".yaml", ".yml":
		return styles.IconFileYAML
	case ".toml":
		return styles.IconFileTOML
	case ".xml":
		return styles.IconFileXML
	case ".html", ".htm":
		return styles.IconFileHTML
	case ".css":
		return styles.IconFileCSS
	case ".rs":
		return styles.IconFileRust
	case ".c":
		return styles.IconFileC
	case ".cpp", ".cc", ".cxx":
		return styles.IconFileCPP
	case ".java":
		return styles.IconFileJava
	case ".rb":
		return styles.IconFileRuby
	case ".php":
		return styles.IconFilePHP
	case ".sh", ".bash", ".zsh":
		return styles.IconFileShell
	case ".sql":
		return styles.IconFileSQL
	case ".vim":
		return styles.IconFileVim
	case ".lua":
		return styles.IconFileLua
	default:
		return styles.IconFileDefault
	}
}

// isLineSelected returns whether the given line index is within the current selection.
func (m DiffViewerModel) isLineSelected(lineIdx int) bool {
	if !m.selectionMode {
		return false
	}

	// Selection range is inclusive of both start and cursor
	start := min(m.selectionStart, m.cursorLine)
	end := max(m.selectionStart, m.cursorLine)

	return lineIdx >= start && lineIdx <= end
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
	m.cursorLine = 0
	m.selectionMode = false
	m.selectionStart = 0
	m.generateContent()
}

// SelectionMode returns whether visual selection mode is active.
func (m DiffViewerModel) SelectionMode() bool {
	return m.selectionMode
}

// CurrentLine returns the current cursor line position (1-indexed for display).
func (m DiffViewerModel) CurrentLine() int {
	return m.cursorLine + 1
}

// TotalLines returns the total number of lines in the diff.
func (m DiffViewerModel) TotalLines() int {
	return len(m.lines)
}

// GetSelectionInfo returns the current selection range if in visual mode.
// Returns start and end line indices (0-based), and whether selection is active.
func (m DiffViewerModel) GetSelectionInfo() (start int, end int, active bool) {
	if !m.selectionMode {
		return 0, 0, false
	}

	start = m.selectionStart
	end = m.cursorLine

	// Normalize so start <= end
	if start > end {
		start, end = end, start
	}

	return start, end, true
}
