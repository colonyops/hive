package review

import (
	"strings"

	"charm.land/bubbles/v2/viewport"

	corereview "github.com/colonyops/hive/internal/core/review"
	"github.com/colonyops/hive/internal/core/styles"
)

// DocumentView handles document rendering with line numbers, comments, and cursor highlighting.
type DocumentView struct {
	viewport       viewport.Model
	document       *Document   // Currently loaded document
	cursorLine     int         // 1-indexed cursor position
	selectionStart int         // 1-indexed selection anchor (0 if none)
	lineMapping    map[int]int // Maps document line numbers to display line numbers (nil when no comments)
	searchMatches  []int       // Line numbers of search matches (1-indexed, in document coordinates)
	searchIndex    int         // Current match index in searchMatches
	width          int         // Viewport width
	height         int         // Viewport height
}

// NewDocumentView creates a new DocumentView instance.
func NewDocumentView(doc *Document) DocumentView {
	vp := viewport.New()

	return DocumentView{
		viewport:       vp,
		document:       doc,
		cursorLine:     1,
		selectionStart: 0,
		lineMapping:    nil,
		searchMatches:  nil,
		searchIndex:    0,
		width:          80,
		height:         24,
	}
}

// SetSize updates the viewport dimensions.
func (dv *DocumentView) SetSize(width, height int) {
	dv.width = width
	dv.height = height
	dv.viewport = viewport.New(viewport.WithWidth(width), viewport.WithHeight(height))

	// Re-render document with new size
	if dv.document != nil {
		rendered, err := dv.document.Render(width)
		if err == nil {
			dv.viewport.SetContent(rendered)
		}
	}
}

// Render returns basic document rendering with line numbers.
func (dv *DocumentView) Render() string {
	if dv.document == nil {
		return ""
	}

	rendered, err := dv.document.Render(dv.width)
	if err != nil {
		return "Error rendering document: " + err.Error()
	}

	return rendered
}

// RenderWithComments inserts comments inline after their referenced lines.
// Returns the rendered content with comments inserted.
func (dv *DocumentView) RenderWithComments(comments []corereview.Comment) string {
	if dv.document == nil {
		return ""
	}

	// First render the base document
	rendered, err := dv.document.Render(dv.width)
	if err != nil {
		return "Error rendering document: " + err.Error()
	}

	// If no comments, return base rendering with cleared line mapping
	if len(comments) == 0 {
		dv.lineMapping = nil
		return rendered
	}

	// Insert comments inline and build line mapping
	var mappedContent string
	mappedContent, dv.lineMapping = dv.insertCommentsInline(rendered, comments)

	return mappedContent
}

// MoveCursor moves the cursor up or down by delta lines.
// Positive delta moves down, negative moves up.
func (dv *DocumentView) MoveCursor(delta int) {
	if dv.document == nil {
		return
	}

	maxLine := len(dv.document.RenderedLines)
	if delta > 0 {
		// Move down
		dv.cursorLine = min(dv.cursorLine+delta, maxLine)
	} else {
		// Move up
		dv.cursorLine = max(dv.cursorLine+delta, 1)
	}

	dv.ensureCursorVisible()
}

// SetSelection sets the visual selection range.
func (dv *DocumentView) SetSelection(start, end int) {
	dv.selectionStart = start
	// Cursor line is used as the end of selection
	dv.cursorLine = end
}

// ClearSelection clears the visual selection.
func (dv *DocumentView) ClearSelection() {
	dv.selectionStart = 0
}

// GetSelectedText extracts the text from the selected line range.
// Returns empty string if no selection or document is nil.
func (dv *DocumentView) GetSelectedText() string {
	if dv.document == nil || len(dv.document.RenderedLines) == 0 || dv.selectionStart == 0 {
		return ""
	}

	// Calculate selection range from anchor to cursor
	start := min(dv.selectionStart, dv.cursorLine)
	end := max(dv.selectionStart, dv.cursorLine)

	// Extract selected lines (adjust for 1-indexed)
	var selectedLines []string
	for i := start - 1; i < end && i < len(dv.document.RenderedLines); i++ {
		selectedLines = append(selectedLines, dv.document.RenderedLines[i])
	}

	return strings.Join(selectedLines, "\n")
}

// JumpToLine moves the cursor to the specified line and scrolls to make it visible.
func (dv *DocumentView) JumpToLine(line int) {
	if dv.document == nil {
		return
	}

	maxLine := len(dv.document.RenderedLines)
	dv.cursorLine = max(1, min(line, maxLine))
	dv.ensureCursorVisible()
}

// HighlightSearchMatches sets the search match positions and current index.
// matches should be line numbers in document coordinates (1-indexed).
func (dv *DocumentView) HighlightSearchMatches(matches []int, index int) {
	dv.searchMatches = matches
	dv.searchIndex = index
}

// ensureCursorVisible scrolls viewport to keep cursor visible.
// Accounts for status bar in full-screen mode to prevent cursor from being hidden.
// Uses display coordinates when comments are inserted inline.
func (dv *DocumentView) ensureCursorVisible() {
	// Map cursor line from document coordinates to display coordinates
	displayCursorLine := dv.mapDocToDisplay(dv.cursorLine, dv.lineMapping)

	offset := dv.viewport.YOffset()
	visibleHeight := dv.viewport.VisibleLineCount()

	// Reserve 1 line for status bar
	visibleHeight--

	// Cursor above viewport - scroll up
	if displayCursorLine < offset+1 {
		dv.viewport.SetYOffset(displayCursorLine - 1)
	}

	// Cursor below viewport - scroll down
	// Keep cursor at least 1 line away from bottom to avoid status bar overlap
	if displayCursorLine > offset+visibleHeight {
		dv.viewport.SetYOffset(displayCursorLine - visibleHeight)
	}
}

// wrapComment wraps comment text to fit within the content width.
// indent specifies number of spaces to add at the start of the first line.
// Continuation lines get 2 additional spaces of indentation.
func (dv *DocumentView) wrapComment(text string, indent int) []string {
	maxWidth := contentWrapWidth(dv.width)

	// Split text into words
	words := strings.Fields(text)
	if len(words) == 0 {
		return []string{strings.Repeat(" ", indent)}
	}

	var lines []string
	var currentLine strings.Builder
	firstLineIndent := strings.Repeat(" ", indent)
	continuationIndent := strings.Repeat(" ", indent+2) // 2 extra spaces for continuation
	isFirstLine := true

	// Add indent to first line
	currentLine.WriteString(firstLineIndent)
	currentLineLen := indent

	for i, word := range words {
		wordLen := len(word)
		spaceLen := 0
		if i > 0 {
			spaceLen = 1 // Space before word (except first word)
		}

		// Check if adding this word would exceed maxWidth
		if currentLineLen+spaceLen+wordLen > maxWidth && currentLineLen > indent {
			// Finish current line and start new one
			lines = append(lines, currentLine.String())
			currentLine.Reset()

			// Use continuation indent for subsequent lines
			currentLine.WriteString(continuationIndent)
			currentLine.WriteString(word)
			currentLineLen = indent + 2 + wordLen // +2 for continuation indent
			isFirstLine = false
		} else {
			// Add word to current line
			if i > 0 {
				currentLine.WriteString(" ")
			}
			currentLine.WriteString(word)
			currentLineLen += spaceLen + wordLen
		}
	}

	// Add final line if not empty
	minLen := indent
	if !isFirstLine {
		minLen = indent + 2 // Continuation lines have 2 extra spaces
	}
	if currentLine.Len() > minLen {
		lines = append(lines, currentLine.String())
	}

	return lines
}

// insertCommentsInline inserts comments after their referenced lines.
// Returns the rendered content and a mapping from document line numbers to display line numbers.
func (dv *DocumentView) insertCommentsInline(content string, comments []corereview.Comment) (string, map[int]int) {
	lines := strings.Split(content, "\n")
	originalLineCount := len(lines)

	// Group comments by end line
	commentsByLine := make(map[int][]corereview.Comment)
	for _, comment := range comments {
		commentsByLine[comment.EndLine] = append(commentsByLine[comment.EndLine], comment)
	}

	// Get sorted line numbers to insert in reverse order (prevents offset issues)
	lineNumbers := make([]int, 0, len(commentsByLine))
	for lineNum := range commentsByLine {
		lineNumbers = append(lineNumbers, lineNum)
	}
	// Sort in descending order to insert from bottom to top
	for i := 0; i < len(lineNumbers); i++ {
		for j := i + 1; j < len(lineNumbers); j++ {
			if lineNumbers[i] < lineNumbers[j] {
				lineNumbers[i], lineNumbers[j] = lineNumbers[j], lineNumbers[i]
			}
		}
	}

	// Insert comments after their lines with enhanced visual styling
	commentStyle := styles.ReviewInlineCommentStyle

	// Track how many comment lines were inserted before each document line
	// This allows us to map document line numbers to display line numbers
	insertedBeforeLine := make(map[int]int) // document line -> number of comment lines inserted before it

	// Insert comments in reverse order to avoid offset issues
	for _, lineNum := range lineNumbers {
		if lineNum < 1 || lineNum > len(lines) {
			continue // Skip invalid line numbers
		}

		comments := commentsByLine[lineNum]
		// Build comment lines to insert
		commentLines := make([]string, 0, len(comments))
		for _, comment := range comments {
			icon := styles.IconComment
			// Use adaptive wrapping with 6 spaces indentation for visual separation
			wrappedLines := dv.wrapComment(icon+" "+comment.CommentText, 6)
			// Apply styling to each wrapped line
			for _, wrappedLine := range wrappedLines {
				styledLine := commentStyle.Render(wrappedLine)
				commentLines = append(commentLines, styledLine)
			}
		}

		// Insert comment lines after the target line
		insertPos := lineNum
		lines = append(lines[:insertPos], append(commentLines, lines[insertPos:]...)...)

		// Track insertions for lines after this one
		numInserted := len(commentLines)
		for i := lineNum + 1; i <= originalLineCount; i++ {
			insertedBeforeLine[i] += numInserted
		}
	}

	// Build the mapping: document line number -> display line number
	lineMapping := make(map[int]int)
	for docLine := 1; docLine <= originalLineCount; docLine++ {
		displayLine := docLine + insertedBeforeLine[docLine]
		lineMapping[docLine] = displayLine
	}

	return strings.Join(lines, "\n"), lineMapping
}

// mapDocToDisplay maps a document line number to a display line number.
// If lineMapping is nil (no comments inserted), returns the same line number.
func (dv *DocumentView) mapDocToDisplay(docLine int, lineMapping map[int]int) int {
	if lineMapping == nil {
		return docLine
	}
	if displayLine, ok := lineMapping[docLine]; ok {
		return displayLine
	}
	return docLine // fallback to same line if not in mapping
}
