package diff

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	lipgloss "charm.land/lipgloss/v2"
	"github.com/bluekeyes/go-gitdiff/gitdiff"
	"github.com/hay-kot/hive/internal/core/styles"
)

// cachedDiff stores generated diff content for quick retrieval.
type cachedDiff struct {
	content string
	lines   []string
}

// diffGeneratedMsg is sent when async diff generation completes.
type diffGeneratedMsg struct {
	filePath string
	content  string
	lines    []string
}

// editorFinishedMsg is sent when the editor process completes.
type editorFinishedMsg struct {
	err error
}

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

	// Caching and async generation
	cache   map[string]*cachedDiff // Cache of generated diffs by file path
	loading bool                   // Whether diff is being generated asynchronously
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
		cache:          make(map[string]*cachedDiff),
		loading:        false,
	}

	// Don't generate content here - will be done async on first SetFile call

	return m
}

// generateDiffContent builds the diff content string from the file.
// If delta is available, applies syntax highlighting.
// This function is separate from the model to allow async execution.
func generateDiffContent(file *gitdiff.File, deltaAvailable bool) (string, []string) {
	if file == nil {
		return "", nil
	}

	// Build unified diff format
	var sb strings.Builder

	// Write diff header
	sb.WriteString("--- ")
	sb.WriteString(file.OldName)
	sb.WriteString("\n")
	sb.WriteString("+++ ")
	sb.WriteString(file.NewName)
	sb.WriteString("\n")

	// Write text fragments (hunks)
	for _, frag := range file.TextFragments {
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
	var content string
	if deltaAvailable {
		ctx := context.Background()
		highlighted, err := ExecDelta(ctx, diff)
		if err == nil {
			content = highlighted
		} else {
			// Fallback to plain diff if delta fails
			content = diff
		}
	} else {
		content = diff
	}

	// Split into lines for scrolling
	lines := strings.Split(content, "\n")

	return content, lines
}

// formatRange formats a hunk range (position, length) for unified diff format.
func formatRange(pos, length int64) string {
	if length == 1 {
		return strconv.FormatInt(pos, 10)
	}
	return strconv.FormatInt(pos, 10) + "," + strconv.FormatInt(length, 10)
}

// generateDiffCmd creates a tea.Cmd that generates diff content asynchronously.
func (m *DiffViewerModel) generateDiffCmd(filePath string) tea.Cmd {
	file := m.file          // Capture current file
	deltaAvailable := m.deltaAvailable

	return func() tea.Msg {
		content, lines := generateDiffContent(file, deltaAvailable)
		return diffGeneratedMsg{
			filePath: filePath,
			content:  content,
			lines:    lines,
		}
	}
}

// openInEditor returns a tea.Cmd that suspends the TUI and opens the current file in $EDITOR.
func (m *DiffViewerModel) openInEditor() tea.Cmd {
	if m.file == nil {
		return nil
	}

	// Get file path to edit
	filePath := m.file.NewName
	if filePath == "" || filePath == "/dev/null" {
		// If the file was deleted, try to open the old name
		filePath = m.file.OldName
	}

	// If still no valid path, abort
	if filePath == "" || filePath == "/dev/null" {
		return nil
	}

	// Get editor from environment (default to vi if not set)
	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = "vi"
	}

	// Create the command
	c := exec.Command(editor, filePath)

	// ExecProcess suspends the TUI, runs the command, then resumes
	return tea.ExecProcess(c, func(err error) tea.Msg {
		return editorFinishedMsg{err: err}
	})
}

// Update handles keyboard input for scrolling and visual selection.
func (m DiffViewerModel) Update(msg tea.Msg) (DiffViewerModel, tea.Cmd) {
	// Handle async diff generation completion
	if msg, ok := msg.(diffGeneratedMsg); ok {
		// Only apply if still viewing this file
		currentPath := ""
		if m.file != nil {
			currentPath = m.file.NewName
			if currentPath == "" || currentPath == "/dev/null" {
				currentPath = m.file.OldName
			}
		}

		if currentPath == msg.filePath {
			// Cache the result
			m.cache[msg.filePath] = &cachedDiff{
				content: msg.content,
				lines:   msg.lines,
			}

			// Apply to model
			m.content = msg.content
			m.lines = msg.lines
			m.loading = false
		}
		// Ignore stale messages for files we're no longer viewing
		return m, nil
	}

	// Handle editor finished
	if _, ok := msg.(editorFinishedMsg); ok {
		// Editor has finished - TUI is resuming
		// Could refresh diff or handle errors here if needed
		return m, nil
	}

	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		// Calculate content height (accounting for header)
		contentHeight := m.height - headerHeight
		if contentHeight < 1 {
			contentHeight = 1
		}

		maxLine := max(0, len(m.lines)-1)
		maxOffset := max(0, len(m.lines)-contentHeight)

		switch keyMsg.String() {
		case "e":
			// Open file in editor
			return m, m.openInEditor()

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

	if m.loading {
		return m.renderEmptyState("Generating Diff...", "Please wait while the diff is being generated")
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
// Returns a tea.Cmd for async diff generation if needed.
func (m *DiffViewerModel) SetFile(file *gitdiff.File) tea.Cmd {
	m.file = file
	m.offset = 0 // Reset scroll position
	m.cursorLine = 0
	m.selectionMode = false
	m.selectionStart = 0

	if file == nil {
		m.content = ""
		m.lines = nil
		m.loading = false
		return nil
	}

	// Get file path for caching
	filePath := file.NewName
	if filePath == "" || filePath == "/dev/null" {
		filePath = file.OldName
	}

	// Check cache first
	if cached, ok := m.cache[filePath]; ok {
		m.content = cached.content
		m.lines = cached.lines
		m.loading = false
		return nil
	}

	// Cache miss - generate asynchronously
	m.loading = true
	m.content = ""
	m.lines = nil
	return m.generateDiffCmd(filePath)
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
