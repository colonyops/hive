package tui

import (
	"fmt"
	"io"
	"strings"
	"time"

	"charm.land/bubbles/v2/list"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	lipgloss "charm.land/lipgloss/v2"
)

// reviewFinalizedMsg is sent when review is finalized.
type reviewFinalizedMsg struct {
	feedback string
}

// ReviewView manages the review interface.
type ReviewView struct {
	list              list.Model
	viewport          viewport.Model
	watcher           *DocumentWatcher
	contextDir        string
	width             int
	height            int
	previewMode       bool                // True when showing dual-column layout
	fullScreen        bool                // True when showing document in full-screen
	selectedDoc       *ReviewDocument     // Currently selected document for preview
	selectionMode     bool                // True when in visual selection mode
	selectionStart    int                 // Line number where selection starts (1-indexed)
	cursorLine        int                 // Line number where cursor is positioned (1-indexed)
	activeSession     *ReviewSession      // Current review session with comments
	commentModal      *ReviewCommentModal // Active comment entry modal
	confirmModal      *ConfirmModal       // Active confirmation modal
	feedbackGenerated string              // Generated feedback (for clipboard)
}

// NewReviewView creates a new review view.
// If contextDir is non-empty, it will watch for file changes.
func NewReviewView(documents []ReviewDocument, contextDir string) ReviewView {
	items := BuildReviewTreeItems(documents)
	delegate := NewReviewTreeDelegate()
	l := list.New(items, delegate, 0, 0)
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(true)
	l.SetShowTitle(false)
	l.SetShowFilter(false)
	l.Styles.TitleBar = lipgloss.NewStyle()
	l.Styles.Title = lipgloss.NewStyle()

	// Configure help to match sessions view
	helpStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#565f89"))
	l.Help.Styles.ShortKey = helpStyle
	l.Help.Styles.ShortDesc = helpStyle
	l.Help.Styles.ShortSeparator = helpStyle
	l.Help.Styles.FullKey = helpStyle
	l.Help.Styles.FullDesc = helpStyle
	l.Help.Styles.FullSeparator = helpStyle
	l.Help.ShortSeparator = " • "
	l.Styles.HelpStyle = lipgloss.NewStyle().PaddingLeft(1)

	// Initialize watcher if context directory is provided
	var watcher *DocumentWatcher
	if contextDir != "" {
		w, err := NewDocumentWatcher(contextDir)
		if err == nil {
			watcher = w
		}
	}

	// Create viewport for document preview
	vp := viewport.New()

	return ReviewView{
		list:        l,
		viewport:    vp,
		watcher:     watcher,
		contextDir:  contextDir,
		previewMode: true, // Enable preview by default
		cursorLine:  1,    // Initialize cursor at line 1
	}
}

// SetSize updates the view dimensions.
func (v *ReviewView) SetSize(width, height int) {
	v.width = width
	v.height = height

	if v.previewMode && width >= 80 {
		// Dual-column mode: 25% list, 75% preview
		listWidth := int(float64(width) * 0.25)
		v.list.SetSize(listWidth, height)

		// Preview gets remaining width minus divider
		previewWidth := width - listWidth - 1
		v.viewport = viewport.New(viewport.WithWidth(previewWidth), viewport.WithHeight(height))

		// Reload document if one is selected
		if v.selectedDoc != nil {
			v.loadDocument(v.selectedDoc)
		}
	} else {
		// Single column mode
		v.list.SetSize(width, height)
	}
}

// Init initializes the review view and starts the file watcher.
func (v ReviewView) Init() tea.Cmd {
	if v.watcher != nil {
		return v.watcher.Start()
	}
	return nil
}

// Update handles messages.
// The underlying list handles j/k navigation, Enter selection, and / filtering.
func (v ReviewView) Update(msg tea.Msg) (ReviewView, tea.Cmd) {
	switch msg := msg.(type) {
	case documentChangeMsg:
		// Rebuild tree with new documents
		items := BuildReviewTreeItems(msg.documents)
		v.list.SetItems(items)
		// Continue watching for more changes
		if v.watcher != nil {
			return v, v.watcher.Start()
		}
		return v, nil

	case tea.KeyMsg:
		// Handle confirmation modal if active (MUST be first to prevent key conflicts)
		if v.confirmModal != nil {
			modal, cmd := v.confirmModal.Update(msg)
			v.confirmModal = &modal

			if v.confirmModal.Confirmed() {
				// Generate feedback and finalize
				feedback := GenerateReviewFeedback(v.activeSession, v.selectedDoc.RelPath)
				v.feedbackGenerated = feedback
				v.confirmModal = nil
				// Clear active session
				v.activeSession = nil
				// Reload document without comments
				v.loadDocument(v.selectedDoc)
				// Return message to trigger clipboard copy
				return v, func() tea.Msg {
					return reviewFinalizedMsg{feedback: feedback}
				}
			}

			if v.confirmModal.Cancelled() {
				v.confirmModal = nil
				return v, cmd
			}

			return v, cmd
		}

		// Handle comment modal if active (MUST be after confirm modal)
		if v.commentModal != nil {
			modal, cmd := v.commentModal.Update(msg)
			v.commentModal = &modal

			if v.commentModal.Submitted() {
				// Create comment and add to session
				v.addComment(v.commentModal.Value())
				v.commentModal = nil
				v.selectionMode = false
				v.renderWithComments()
				return v, cmd
			}

			if v.commentModal.Cancelled() {
				v.commentModal = nil
				return v, cmd
			}

			return v, cmd
		}

		// Handle full-screen toggle
		switch msg.String() {
		case "enter":
			// Toggle full-screen mode if a document is selected
			if v.selectedDoc != nil && !v.list.SettingFilter() {
				v.fullScreen = !v.fullScreen
				// Adjust viewport size for full-screen
				if v.fullScreen {
					v.viewport = viewport.New(viewport.WithWidth(v.width), viewport.WithHeight(v.height))
					v.loadDocument(v.selectedDoc)
					// Initialize cursor to center of viewport
					v.cursorLine = max(1, v.viewport.VisibleLineCount()/2)
				} else {
					// Return to dual-column mode
					v.SetSize(v.width, v.height)
				}
				return v, nil
			}
		case "esc":
			// Exit full-screen mode
			if v.fullScreen {
				v.fullScreen = false
				v.SetSize(v.width, v.height)
				return v, nil
			}
		}

		// Handle visual selection mode and finalization
		if v.fullScreen && v.selectedDoc != nil {
			switch msg.String() {
			case "f":
				// Finalize review - show confirmation if there are comments
				if v.activeSession != nil && len(v.activeSession.Comments) > 0 {
					confirmMsg := fmt.Sprintf("Finalize review with %d comment(s)?", len(v.activeSession.Comments))
					modal := NewConfirmModal(confirmMsg)
					v.confirmModal = &modal
					return v, nil
				}
			case "c":
				// Open comment modal if in selection mode
				if v.selectionMode {
					contextText := v.getSelectedText()
					// Calculate selection range from anchor to cursor
					start := min(v.selectionStart, v.cursorLine)
					end := max(v.selectionStart, v.cursorLine)
					modal := NewReviewCommentModal(start, end, contextText, v.width, v.height)
					v.commentModal = &modal
					return v, nil
				}
			case "V", "shift+v":
				// Enter or exit visual selection mode
				if !v.selectionMode {
					v.selectionMode = true
					// Set selection anchor to cursor position
					v.selectionStart = v.cursorLine
					v.renderSelection()
					return v, nil
				} else {
					// Exit selection mode (keep cursor position)
					v.selectionMode = false
					v.renderSelection()
					return v, nil
				}
			case "v":
				// Also allow lowercase v to toggle visual mode
				if v.selectionMode {
					v.selectionMode = false
					v.renderSelection()
					return v, nil
				}
			case "esc":
				// Exit selection mode without action
				if v.selectionMode {
					v.selectionMode = false
					v.renderSelection()
					return v, nil
				}
			}
		}

		// Handle navigation in full-screen mode
		if v.fullScreen {
			switch msg.String() {
			case "j", "down":
				// Move cursor down (selection extends automatically if in visual mode)
				v.moveCursorDown(1)
				v.renderSelection()
				return v, nil
			case "k", "up":
				// Move cursor up (selection extends automatically if in visual mode)
				v.moveCursorUp(1)
				v.renderSelection()
				return v, nil
			case "ctrl+d":
				v.viewport.HalfPageDown()
				// Update cursor to center of viewport
				v.cursorLine = v.viewport.YOffset() + v.viewport.VisibleLineCount()/2
				v.renderSelection()
				return v, nil
			case "ctrl+u":
				v.viewport.HalfPageUp()
				// Update cursor to center of viewport
				v.cursorLine = v.viewport.YOffset() + v.viewport.VisibleLineCount()/2
				v.renderSelection()
				return v, nil
			case "g":
				v.viewport.GotoTop()
				v.cursorLine = 1
				v.renderSelection()
				return v, nil
			case "G":
				v.viewport.GotoBottom()
				if v.selectedDoc != nil {
					v.cursorLine = len(v.selectedDoc.RenderedLines)
				}
				v.renderSelection()
				return v, nil
			}
		}
	}

	// Track previous selection
	prevIndex := v.list.Index()

	var cmd tea.Cmd
	v.list, cmd = v.list.Update(msg)

	// Detect selection change
	if v.list.Index() != prevIndex {
		selectedDoc := v.SelectedDocument()
		v.loadDocument(selectedDoc)
	}

	return v, cmd
}

// View renders the review view.
func (v ReviewView) View() string {
	var baseView string

	// Full-screen mode: show viewport with status bar
	if v.fullScreen {
		contentHeight := v.height - 1 // Reserve 1 line for status bar
		content := v.viewport.View()
		statusBar := v.renderStatusBar()

		// Truncate content if needed to make room for status bar
		contentLines := strings.Split(content, "\n")
		if len(contentLines) > contentHeight {
			contentLines = contentLines[:contentHeight]
		}

		baseView = strings.Join(contentLines, "\n") + "\n" + statusBar
	} else if !v.previewMode || v.width < 80 {
		// Single column mode
		baseView = v.list.View()
	} else {
		// Dual-column mode: list | divider | preview
		listView := v.list.View()
		previewView := v.viewport.View()

		// Create vertical divider
		dividerStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#565f89"))
		dividerLines := make([]string, v.height)
		for i := range dividerLines {
			dividerLines[i] = dividerStyle.Render("│")
		}
		divider := strings.Join(dividerLines, "\n")

		baseView = lipgloss.JoinHorizontal(lipgloss.Top, listView, divider, previewView)
	}

	// Overlay confirmation modal if active
	if v.confirmModal != nil {
		modalContent := v.confirmModal.View()
		modalStyle := lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#7aa2f7")).
			Padding(1, 2).
			Background(lipgloss.Color("#1a1b26"))

		modal := modalStyle.Render(modalContent)

		// Center the modal
		modalW := lipgloss.Width(modal)
		modalH := lipgloss.Height(modal)
		x := (v.width - modalW) / 2
		y := (v.height - modalH) / 2

		// Use compositor to overlay modal
		bgLayer := lipgloss.NewLayer(baseView)
		modalLayer := lipgloss.NewLayer(modal)
		modalLayer.X(x).Y(y).Z(1)

		compositor := lipgloss.NewCompositor(bgLayer, modalLayer)
		return compositor.Render()
	}

	// Overlay comment modal if active
	if v.commentModal != nil {
		modalContent := v.commentModal.View()
		modalStyle := lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#7aa2f7")).
			Padding(1, 2).
			Background(lipgloss.Color("#1a1b26"))

		modal := modalStyle.Render(modalContent)

		// Center the modal
		modalW := lipgloss.Width(modal)
		modalH := lipgloss.Height(modal)
		x := (v.width - modalW) / 2
		y := (v.height - modalH) / 2

		// Use compositor to overlay modal
		bgLayer := lipgloss.NewLayer(baseView)
		modalLayer := lipgloss.NewLayer(modal)
		modalLayer.X(x).Y(y).Z(1)

		compositor := lipgloss.NewCompositor(bgLayer, modalLayer)
		return compositor.Render()
	}

	return baseView
}

// SelectedDocument returns the currently selected document, or nil if none.
func (v ReviewView) SelectedDocument() *ReviewDocument {
	item := v.list.SelectedItem()
	if item == nil {
		return nil
	}
	if reviewItem, ok := item.(ReviewTreeItem); ok {
		if reviewItem.IsHeader {
			return nil
		}
		return &reviewItem.Document
	}
	return nil
}

// loadDocument loads and renders a document for preview.
func (v *ReviewView) loadDocument(doc *ReviewDocument) {
	v.selectedDoc = doc
	if doc == nil {
		v.viewport.SetContent("")
		return
	}

	// Reset cursor to top when loading new document
	v.cursorLine = 1

	// Calculate preview width for rendering
	previewWidth := v.width
	if v.previewMode && v.width >= 80 {
		listWidth := int(float64(v.width) * 0.25)
		previewWidth = v.width - listWidth - 1
	}

	// Render document
	rendered, err := doc.Render(previewWidth)
	if err != nil {
		v.viewport.SetContent("Error rendering document: " + err.Error())
		return
	}

	v.viewport.SetContent(rendered)
	v.viewport.GotoTop()
}

// moveCursorDown moves cursor down by n lines, scrolling if needed.
func (v *ReviewView) moveCursorDown(n int) {
	if v.selectedDoc == nil {
		return
	}

	maxLine := len(v.selectedDoc.RenderedLines)
	v.cursorLine = min(v.cursorLine+n, maxLine)
	v.ensureCursorVisible()
}

// moveCursorUp moves cursor up by n lines, scrolling if needed.
func (v *ReviewView) moveCursorUp(n int) {
	v.cursorLine = max(v.cursorLine-n, 1)
	v.ensureCursorVisible()
}

// ensureCursorVisible scrolls viewport to keep cursor visible.
func (v *ReviewView) ensureCursorVisible() {
	offset := v.viewport.YOffset()
	visibleHeight := v.viewport.VisibleLineCount()

	// Cursor above viewport - scroll up
	if v.cursorLine < offset+1 {
		v.viewport.SetYOffset(v.cursorLine - 1)
	}

	// Cursor below viewport - scroll down
	if v.cursorLine > offset+visibleHeight {
		v.viewport.SetYOffset(v.cursorLine - visibleHeight)
	}
}

// renderSelection re-renders the document with selection and cursor highlighting.
func (v *ReviewView) renderSelection() {
	if v.selectedDoc == nil {
		return
	}

	// Calculate preview width for rendering
	previewWidth := v.width
	if v.previewMode && v.width >= 80 {
		listWidth := int(float64(v.width) * 0.25)
		previewWidth = v.width - listWidth - 1
	}

	// Render document
	rendered, err := v.selectedDoc.Render(previewWidth)
	if err != nil {
		v.viewport.SetContent("Error rendering document: " + err.Error())
		return
	}

	// Apply cursor and selection highlighting
	rendered = v.highlightSelection(rendered)

	v.viewport.SetContent(rendered)
}

// highlightSelection applies background color to cursor and selected lines.
// Also adds gutter indicators for commented lines.
func (v *ReviewView) highlightSelection(content string) string {
	lines := strings.Split(content, "\n")

	// Get commented line numbers
	commentedLines := v.getCommentedLines()

	// Gutter styles
	gutterIndicatorStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#e0af68")). // Gold/yellow
		Bold(true)
	gutterEmptyStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#565f89"))

	// Styles with explicit width to fill viewport (accounting for gutter)
	gutterWidth := 2 // "● " or "  "
	contentWidth := v.viewport.Width() - gutterWidth
	selectionStyle := lipgloss.NewStyle().
		Background(lipgloss.Color("#3b4261")).
		Width(contentWidth)
	cursorStyle := lipgloss.NewStyle().
		Background(lipgloss.Color("#2a3158")).
		Width(contentWidth)

	// Calculate selection range if in visual mode
	var start, end int
	if v.selectionMode {
		start = min(v.selectionStart, v.cursorLine)
		end = max(v.selectionStart, v.cursorLine)
	}

	// Apply gutter and highlighting
	for i := range lines {
		lineNum := i + 1

		// Add gutter indicator
		var gutter string
		if commentedLines[lineNum] {
			gutter = gutterIndicatorStyle.Render("● ")
		} else {
			gutter = gutterEmptyStyle.Render("  ")
		}

		// Apply cursor highlight (always visible in full-screen)
		if lineNum == v.cursorLine {
			lines[i] = gutter + cursorStyle.Render(lines[i])
		} else if v.selectionMode && lineNum >= start && lineNum <= end {
			// Apply selection highlight (overrides cursor style if overlapping)
			lines[i] = gutter + selectionStyle.Render(lines[i])
		} else {
			lines[i] = gutter + lines[i]
		}
	}

	return strings.Join(lines, "\n")
}

// getCommentedLines returns a map of line numbers that have comments.
func (v *ReviewView) getCommentedLines() map[int]bool {
	commented := make(map[int]bool)
	if v.activeSession == nil {
		return commented
	}

	for _, comment := range v.activeSession.Comments {
		for line := comment.StartLine; line <= comment.EndLine; line++ {
			commented[line] = true
		}
	}

	return commented
}

// renderStatusBar creates a status bar showing mode and position info.
func (v ReviewView) renderStatusBar() string {
	// Determine mode
	mode := "NORMAL"
	modeStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#c0caf5")).
		Background(lipgloss.Color("#3b4261")).
		Bold(true).
		Padding(0, 1)

	if v.selectionMode {
		mode = "VISUAL"
		modeStyle = modeStyle.Background(lipgloss.Color("#7aa2f7"))
	}

	// Calculate total lines
	totalLines := 0
	if v.selectedDoc != nil {
		totalLines = len(v.selectedDoc.RenderedLines)
	}

	// Position info
	posInfo := fmt.Sprintf("Line %d/%d", v.cursorLine, totalLines)
	posStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#565f89")).
		Background(lipgloss.Color("#1f2335")).
		Padding(0, 1)

	// Build status bar
	leftPart := modeStyle.Render(mode)
	rightPart := posStyle.Render(posInfo)

	// Calculate spacing to fill width
	usedWidth := lipgloss.Width(leftPart) + lipgloss.Width(rightPart)
	spacing := strings.Repeat(" ", max(0, v.width-usedWidth))

	bgStyle := lipgloss.NewStyle().Background(lipgloss.Color("#1f2335"))
	return leftPart + bgStyle.Render(spacing) + rightPart
}

// getSelectedText extracts the text from the selected line range.
func (v *ReviewView) getSelectedText() string {
	if v.selectedDoc == nil || len(v.selectedDoc.RenderedLines) == 0 {
		return ""
	}

	// Calculate selection range from anchor to cursor
	start := min(v.selectionStart, v.cursorLine)
	end := max(v.selectionStart, v.cursorLine)

	// Extract selected lines (adjust for 1-indexed)
	var selectedLines []string
	for i := start - 1; i < end && i < len(v.selectedDoc.RenderedLines); i++ {
		selectedLines = append(selectedLines, v.selectedDoc.RenderedLines[i])
	}

	return strings.Join(selectedLines, "\n")
}

// addComment creates a new comment and adds it to the active session.
func (v *ReviewView) addComment(commentText string) {
	if v.selectedDoc == nil {
		return
	}

	// Initialize session if needed
	if v.activeSession == nil {
		v.activeSession = &ReviewSession{
			ID:         fmt.Sprintf("session-%d", time.Now().Unix()),
			DocPath:    v.selectedDoc.Path,
			Comments:   []ReviewComment{},
			CreatedAt:  time.Now(),
			ModifiedAt: time.Now(),
		}
	}

	// Calculate selection range from anchor to cursor
	start := min(v.selectionStart, v.cursorLine)
	end := max(v.selectionStart, v.cursorLine)

	// Create comment
	comment := ReviewComment{
		ID:          fmt.Sprintf("comment-%d", time.Now().UnixNano()),
		SessionID:   v.activeSession.ID,
		StartLine:   start,
		EndLine:     end,
		ContextText: v.getSelectedText(),
		CommentText: commentText,
		CreatedAt:   time.Now(),
	}

	v.activeSession.Comments = append(v.activeSession.Comments, comment)
	v.activeSession.ModifiedAt = time.Now()
}

// renderWithComments renders the document with inline comments.
func (v *ReviewView) renderWithComments() {
	if v.selectedDoc == nil {
		return
	}

	// Calculate preview width for rendering
	previewWidth := v.width
	if v.previewMode && v.width >= 80 {
		listWidth := int(float64(v.width) * 0.25)
		previewWidth = v.width - listWidth - 1
	}

	// Render document
	rendered, err := v.selectedDoc.Render(previewWidth)
	if err != nil {
		v.viewport.SetContent("Error rendering document: " + err.Error())
		return
	}

	// Insert comments inline if session exists
	if v.activeSession != nil && len(v.activeSession.Comments) > 0 {
		rendered = v.insertCommentsInline(rendered)
	}

	v.viewport.SetContent(rendered)
}

// insertCommentsInline inserts comments after their referenced lines.
func (v *ReviewView) insertCommentsInline(content string) string {
	lines := strings.Split(content, "\n")

	// Group comments by end line
	commentsByLine := make(map[int][]ReviewComment)
	for _, comment := range v.activeSession.Comments {
		commentsByLine[comment.EndLine] = append(commentsByLine[comment.EndLine], comment)
	}

	// Insert comments after their lines
	commentStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#7aa2f7")).
		Italic(true)

	var result []string
	for i, line := range lines {
		result = append(result, line)

		lineNum := i + 1
		if comments, ok := commentsByLine[lineNum]; ok {
			for _, comment := range comments {
				commentLine := commentStyle.Render("  ▸ " + comment.CommentText)
				result = append(result, commentLine)
			}
		}
	}

	return strings.Join(result, "\n")
}

// ReviewTreeItem represents an item in the review tree.
type ReviewTreeItem struct {
	IsHeader     bool           // True if this is a document type header
	HeaderName   string         // Document type name (e.g., "Plans", "Research")
	Document     ReviewDocument // The document (when !IsHeader)
	IsLastInType bool           // True if last document in this type group
	CommentCount int            // Number of comments on this document
}

// FilterValue returns the value used for filtering.
func (i ReviewTreeItem) FilterValue() string {
	if i.IsHeader {
		return ""
	}
	return i.Document.RelPath
}

// BuildReviewTreeItems converts documents into tree items grouped by type.
func BuildReviewTreeItems(documents []ReviewDocument) []list.Item {
	if len(documents) == 0 {
		return nil
	}

	items := make([]list.Item, 0)

	// Group documents by type
	groups := make(map[DocumentType][]ReviewDocument)
	for _, doc := range documents {
		groups[doc.Type] = append(groups[doc.Type], doc)
	}

	// Render in order: Plans, Research, Context, Other
	typeOrder := []DocumentType{DocTypePlan, DocTypeResearch, DocTypeContext, DocTypeOther}

	for _, docType := range typeOrder {
		docs, exists := groups[docType]
		if !exists || len(docs) == 0 {
			continue
		}

		// Add header
		header := ReviewTreeItem{
			IsHeader:   true,
			HeaderName: docType.String(),
		}
		items = append(items, header)

		// Add documents
		for idx, doc := range docs {
			isLast := idx == len(docs)-1
			item := ReviewTreeItem{
				IsHeader:     false,
				Document:     doc,
				IsLastInType: isLast,
				CommentCount: 0, // TODO: Load from store in Phase 6
			}
			items = append(items, item)
		}
	}

	return items
}

// ReviewTreeDelegate handles rendering of review tree items.
type ReviewTreeDelegate struct {
	styles ReviewTreeDelegateStyles
}

// ReviewTreeDelegateStyles defines the styles for review tree rendering.
type ReviewTreeDelegateStyles struct {
	HeaderNormal   lipgloss.Style
	HeaderSelected lipgloss.Style
	TreeLine       lipgloss.Style
	DocName        lipgloss.Style
	DocMeta        lipgloss.Style
	Selected       lipgloss.Style
	SelectedBorder lipgloss.Style
}

// DefaultReviewTreeDelegateStyles returns the default styles.
func DefaultReviewTreeDelegateStyles() ReviewTreeDelegateStyles {
	return ReviewTreeDelegateStyles{
		HeaderNormal:   lipgloss.NewStyle().Bold(true).Foreground(colorWhite),
		HeaderSelected: lipgloss.NewStyle().Bold(true).Foreground(colorBlue),
		TreeLine:       lipgloss.NewStyle().Foreground(colorGray),
		DocName:        lipgloss.NewStyle().Foreground(colorWhite),
		DocMeta:        lipgloss.NewStyle().Foreground(colorGray),
		Selected:       lipgloss.NewStyle().Foreground(colorBlue).Bold(true),
		SelectedBorder: lipgloss.NewStyle().Foreground(colorBlue),
	}
}

// NewReviewTreeDelegate creates a new review tree delegate.
func NewReviewTreeDelegate() ReviewTreeDelegate {
	return ReviewTreeDelegate{
		styles: DefaultReviewTreeDelegateStyles(),
	}
}

// Height returns the height of each item.
func (d ReviewTreeDelegate) Height() int {
	return 1
}

// Spacing returns the spacing between items.
func (d ReviewTreeDelegate) Spacing() int {
	return 0
}

// Update handles item updates.
func (d ReviewTreeDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd {
	return nil
}

// Render renders a single review tree item.
func (d ReviewTreeDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	reviewItem, ok := item.(ReviewTreeItem)
	if !ok {
		return
	}

	isSelected := index == m.Index()

	var line string
	if reviewItem.IsHeader {
		line = d.renderHeader(reviewItem, isSelected)
	} else {
		line = d.renderDocument(reviewItem, isSelected)
	}

	// Selection indicator
	var prefix string
	if isSelected {
		prefix = d.styles.SelectedBorder.Render("┃") + " "
	} else {
		prefix = "  "
	}

	_, _ = fmt.Fprintf(w, "%s%s", prefix, line)
}

// renderHeader renders a document type header.
func (d ReviewTreeDelegate) renderHeader(item ReviewTreeItem, isSelected bool) string {
	nameStyle := d.styles.HeaderNormal
	if isSelected {
		nameStyle = d.styles.HeaderSelected
	}
	return nameStyle.Render(item.HeaderName)
}

// renderDocument renders a document entry with tree prefix.
func (d ReviewTreeDelegate) renderDocument(item ReviewTreeItem, isSelected bool) string {
	// Tree prefix
	var prefix string
	if item.IsLastInType {
		prefix = treeLast
	} else {
		prefix = treeBranch
	}
	prefixStyled := d.styles.TreeLine.Render(prefix)

	// Document name
	nameStyle := d.styles.DocName
	if isSelected {
		nameStyle = d.styles.Selected
	}
	name := nameStyle.Render(item.Document.RelPath)

	// Comment count (if any)
	var comments string
	if item.CommentCount > 0 {
		comments = d.styles.DocMeta.Render(fmt.Sprintf(" (%d)", item.CommentCount))
	}

	return fmt.Sprintf("%s %s%s", prefixStyled, name, comments)
}
