package review

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"charm.land/bubbles/v2/list"
	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	lipgloss "charm.land/lipgloss/v2"
	"github.com/google/uuid"
	"github.com/rs/zerolog/log"

	corereview "github.com/hay-kot/hive/internal/core/review"
	"github.com/hay-kot/hive/internal/stores"
	"github.com/hay-kot/hive/internal/styles"
	"github.com/hay-kot/hive/internal/tui/components"
)

// ReviewFinalizedMsg is sent when review is finalized and copied to clipboard.
type ReviewFinalizedMsg struct {
	Feedback string
}

// SendToAgentMsg is sent when feedback should be sent to Claude agent.
type SendToAgentMsg struct {
	Feedback string
}

// reviewDiscardedMsg is sent when review is discarded (internal only).
type reviewDiscardedMsg struct{}

// View manages the review interface.
type View struct {
	list              list.Model
	viewport          viewport.Model
	watcher           *DocumentWatcher
	contextDir        string
	store             *stores.ReviewStore // SQLite persistence for review sessions
	width             int
	height            int
	previewMode       bool                     // True when showing dual-column layout
	fullScreen        bool                     // True when showing document in full-screen
	selectedDoc       *Document                // Currently selected document for preview
	selectionMode     bool                     // True when in visual selection mode
	selectionStart    int                      // Line number where selection starts (1-indexed)
	cursorLine        int                      // Line number where cursor is positioned (1-indexed)
	activeSession     *Session                 // Current review session with comments
	commentModal      *CommentModal            // Active comment entry modal
	confirmModal      *components.ConfirmModal // Active confirmation modal
	finalizationModal *FinalizationModal       // Active finalization options modal
	pickerModal       *DocumentPickerModal     // Active document picker modal
	feedbackGenerated string                   // Generated feedback (for clipboard)
	searchMode        bool                     // True when in search/filter mode
	searchInput       textinput.Model          // Search input field
	searchQuery       string                   // Current search query
	searchMatches     []int                    // Line numbers of search matches (1-indexed)
	searchMatchIndex  int                      // Current match index in searchMatches
	hasAgentCommand   bool                     // Whether send-claude command is available
	pendingDeleteLine int                      // Line number for pending comment deletion (0 if none)
	pendingDiscard    bool                     // True when waiting for discard confirmation
	editingCommentID  string                   // ID of comment being edited (empty if creating new)
	lineMapping       map[int]int              // Maps document line numbers to display line numbers (nil when no comments)
}

// New creates a new review view.
// If contextDir is non-empty, it will watch for file changes.
// If store is non-nil, comments will be persisted to the database.
func New(documents []Document, contextDir string, store *stores.ReviewStore) View {
	items := BuildTreeItems(documents)
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

	// Initialize search input
	ti := textinput.New()
	ti.Placeholder = "Search..."
	ti.CharLimit = 100

	return View{
		list:        l,
		viewport:    vp,
		watcher:     watcher,
		contextDir:  contextDir,
		store:       store,
		previewMode: false, // Disable dual-column preview by default
		fullScreen:  false, // Start without a document (will show picker or message)
		cursorLine:  1,     // Initialize cursor at line 1
		searchInput: ti,
	}
}

// SetSize updates the view dimensions.
func (v *View) SetSize(width, height int) {
	v.width = width
	v.height = height

	// Always use full-screen mode for documents
	v.viewport = viewport.New(viewport.WithWidth(width), viewport.WithHeight(height))

	// Reload document if one is selected
	if v.selectedDoc != nil {
		v.loadDocument(v.selectedDoc)
	}
}

// Init initializes the review view and starts the file watcher.
func (v View) Init() tea.Cmd {
	if v.watcher != nil {
		return v.watcher.Start()
	}
	return nil
}

// Update handles messages.
// The underlying list handles j/k navigation, Enter selection, and / filtering.
func (v View) Update(msg tea.Msg) (View, tea.Cmd) {
	switch msg := msg.(type) {
	case DocumentChangeMsg:
		// Rebuild tree with new documents
		log.Debug().
			Int("document_count", len(msg.Documents)).
			Msg("review: rebuilding document tree from file watcher")
		items := BuildTreeItems(msg.Documents)
		v.list.SetItems(items)

		// Refresh currently open document if one is selected
		if v.selectedDoc != nil {
			// Find updated version of current document
			for _, doc := range msg.Documents {
				if doc.Path == v.selectedDoc.Path {
					// Reload the document to refresh the view
					v.loadDocument(&doc)
					break
				}
			}
		}

		// Continue watching for more changes
		if v.watcher != nil {
			return v, v.watcher.Start()
		}
		return v, nil

	case reviewDiscardedMsg:
		// Clear active session and reload document
		v.activeSession = nil
		v.updateTreeItemCommentCount()
		if v.selectedDoc != nil {
			v.loadDocument(v.selectedDoc)
		}
		return v, nil

	case tea.KeyMsg:
		// Handle document picker modal if active (MUST be first to prevent key conflicts)
		if v.pickerModal != nil {
			modal, cmd := v.pickerModal.Update(msg)
			v.pickerModal = modal

			if v.pickerModal.SelectedDocument() != nil {
				// User selected a document - open it
				doc := v.pickerModal.SelectedDocument()
				v.pickerModal = nil
				v.loadDocument(doc)
				return v, cmd
			}

			if v.pickerModal.Cancelled() {
				v.pickerModal = nil
				return v, cmd
			}

			return v, cmd
		}

		// Handle finalization modal for choosing action
		if v.finalizationModal != nil {
			modal, cmd := v.finalizationModal.Update(msg)
			v.finalizationModal = &modal

			if v.finalizationModal.Confirmed() {
				action := v.finalizationModal.SelectedAction()
				v.finalizationModal = nil

				// Finalize session in database if store is available
				if v.store != nil && v.activeSession != nil {
					ctx := context.Background()
					_ = v.store.FinalizeSession(ctx, v.activeSession.ID)
					// Ignore errors - finalization is best effort
				}

				// Clear active session
				v.activeSession = nil
				// Reload document without comments
				v.loadDocument(v.selectedDoc)

				// Execute selected action
				switch action {
				case FinalizationActionClipboard:
					return v, func() tea.Msg {
						return ReviewFinalizedMsg{Feedback: v.feedbackGenerated}
					}
				case FinalizationActionSendToAgent:
					return v, func() tea.Msg {
						return SendToAgentMsg{Feedback: v.feedbackGenerated}
					}
				case FinalizationActionNone:
					// User cancelled, do nothing
				}

				return v, cmd
			}

			if v.finalizationModal.Cancelled() {
				v.finalizationModal = nil
				return v, cmd
			}

			return v, cmd
		}

		// Handle confirmation modal if active (keep for backward compatibility)
		if v.confirmModal != nil {
			modal, cmd := v.confirmModal.Update(msg)
			v.confirmModal = &modal

			if v.confirmModal.Confirmed() {
				// Check if this is a review discard confirmation
				if v.pendingDiscard {
					v.pendingDiscard = false
					v.confirmModal = nil
					return v, v.discardReview()
				}

				// Check if this is a comment deletion confirmation
				if v.pendingDeleteLine > 0 {
					// Execute deletion
					v.deleteCommentsAtLine(v.pendingDeleteLine)
					v.pendingDeleteLine = 0
					v.confirmModal = nil
					v.renderSelection()
					return v, cmd
				}

				// Otherwise, it's a finalization confirmation
				// Generate feedback and finalize
				feedback := GenerateReviewFeedback(v.activeSession, v.selectedDoc.RelPath)
				v.feedbackGenerated = feedback
				v.confirmModal = nil

				// Finalize session in database if store is available
				if v.store != nil && v.activeSession != nil {
					ctx := context.Background()
					_ = v.store.FinalizeSession(ctx, v.activeSession.ID)
					// Ignore errors - finalization is best effort
				}

				// Clear active session
				v.activeSession = nil
				// Reload document without comments
				v.loadDocument(v.selectedDoc)
				// Return message to trigger clipboard copy
				return v, func() tea.Msg {
					return ReviewFinalizedMsg{Feedback: feedback}
				}
			}

			if v.confirmModal.Cancelled() {
				v.confirmModal = nil
				v.pendingDeleteLine = 0  // Clear pending delete
				v.pendingDiscard = false // Clear pending discard
				return v, cmd
			}

			return v, cmd
		}

		// Handle comment modal if active
		if v.commentModal != nil {
			modal, cmd := v.commentModal.Update(msg)
			v.commentModal = &modal

			if v.commentModal.Submitted() {
				// Check if we're editing an existing comment or creating a new one
				if v.editingCommentID != "" {
					// Update existing comment
					v.updateComment(v.editingCommentID, v.commentModal.Value())
					v.editingCommentID = ""
				} else {
					// Create new comment
					v.addComment(v.commentModal.Value())
					v.selectionMode = false
				}
				v.commentModal = nil
				v.renderSelection()
				return v, cmd
			}

			if v.commentModal.Cancelled() {
				v.commentModal = nil
				v.editingCommentID = "" // Clear editing state
				v.renderSelection()
				return v, cmd
			}

			return v, cmd
		}

		// Handle search mode input FIRST (before other Enter handlers)
		if v.searchMode && v.fullScreen {
			switch msg.String() {
			case "enter":
				// Find matches and jump to first
				v.searchQuery = v.searchInput.Value()
				v.searchMode = false
				v.findSearchMatches()
				if len(v.searchMatches) > 0 {
					v.searchMatchIndex = 0
					v.jumpToMatch(v.searchMatches[0])
				}
				v.renderSelection()
				return v, nil
			default:
				// Update search input
				var cmd tea.Cmd
				v.searchInput, cmd = v.searchInput.Update(msg)
				return v, cmd
			}
		}

		// Handle esc key
		if msg.String() == "esc" {
			// Priority order: search mode > visual mode > close document
			if v.searchMode {
				// Exit search mode
				v.searchMode = false
				v.searchQuery = ""
				v.searchInput.SetValue("")
				v.renderSelection()
				return v, nil
			}
			// Exit visual mode if active
			if v.selectionMode {
				v.selectionMode = false
				v.renderSelection()
				return v, nil
			}
			// Close document (return to "no document" view)
			if v.selectedDoc != nil {
				v.selectedDoc = nil
				v.fullScreen = false
				v.activeSession = nil
				return v, nil
			}
		}

		// Handle visual selection mode and finalization
		if v.fullScreen && v.selectedDoc != nil {
			switch msg.String() {
			case "f":
				// Finalize review - show finalization options if there are comments
				if v.activeSession != nil && len(v.activeSession.Comments) > 0 {
					// Generate feedback now so we can pass it to the modal
					feedback := GenerateReviewFeedback(v.activeSession, v.selectedDoc.RelPath)
					modal := NewFinalizationModal(feedback, v.hasAgentCommand, v.width, v.height)
					v.finalizationModal = &modal
					v.feedbackGenerated = feedback
					return v, nil
				}
			case "c":
				// Open comment modal if in selection mode
				if v.selectionMode {
					contextText := v.getSelectedText()
					// Calculate selection range from anchor to cursor
					start := min(v.selectionStart, v.cursorLine)
					end := max(v.selectionStart, v.cursorLine)
					modal := NewCommentModal(start, end, contextText, v.width, v.height)
					v.commentModal = &modal
					return v, nil
				}
			case "e":
				// Edit comment on current cursor line
				if !v.selectionMode && v.activeSession != nil {
					// Find comment at cursor line
					for _, comment := range v.activeSession.Comments {
						if v.cursorLine >= comment.StartLine && v.cursorLine <= comment.EndLine {
							// Open comment modal pre-filled with existing comment
							modal := NewCommentModal(
								comment.StartLine,
								comment.EndLine,
								comment.ContextText,
								v.width,
								v.height,
							)
							modal.SetExistingComment(comment.CommentText)
							v.commentModal = &modal
							v.editingCommentID = comment.ID // Track which comment is being edited
							return v, nil
						}
					}
				}
			case "d":
				// Delete comment(s) on current cursor line
				if !v.selectionMode && v.activeSession != nil {
					// Check if there are comments at the cursor line
					hasComment := false
					for _, comment := range v.activeSession.Comments {
						if v.cursorLine >= comment.StartLine && v.cursorLine <= comment.EndLine {
							hasComment = true
							break
						}
					}

					if hasComment {
						// Show confirmation modal
						v.pendingDeleteLine = v.cursorLine
						modal := components.NewConfirmModal("Delete comment(s) at this line?")
						v.confirmModal = &modal
						return v, nil
					}
				}
			case "D", "shift+d":
				// Discard entire review
				if !v.selectionMode && v.activeSession != nil && len(v.activeSession.Comments) > 0 {
					// Show confirmation modal with comment count
					commentCount := len(v.activeSession.Comments)
					message := fmt.Sprintf("Discard review? This will permanently delete %d comment(s). This cannot be undone.", commentCount)
					modal := components.NewConfirmModal(message)
					v.confirmModal = &modal
					v.pendingDiscard = true
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
			}
		}

		// Handle 'p' to open document picker
		if msg.String() == "p" && !v.selectionMode && !v.searchMode {
			return v, v.ShowDocumentPicker()
		}

		// Handle '/' to enter search mode in full-screen
		if v.fullScreen && !v.selectionMode && msg.String() == "/" {
			v.searchMode = true
			v.searchInput.Focus()
			v.searchInput.SetValue("")
			v.searchQuery = ""
			// Clear previous search results
			v.searchMatches = nil
			v.searchMatchIndex = 0
			return v, nil
		}

		// Handle search navigation (n/N for next/previous match)
		if v.fullScreen && v.searchQuery != "" && len(v.searchMatches) > 0 {
			switch msg.String() {
			case "n":
				// Jump to next match
				v.searchMatchIndex = (v.searchMatchIndex + 1) % len(v.searchMatches)
				v.jumpToMatch(v.searchMatches[v.searchMatchIndex])
				v.renderSelection()
				return v, nil
			case "N", "shift+n":
				// Jump to previous match
				v.searchMatchIndex = (v.searchMatchIndex - 1 + len(v.searchMatches)) % len(v.searchMatches)
				v.jumpToMatch(v.searchMatches[v.searchMatchIndex])
				v.renderSelection()
				return v, nil
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
				// Update cursor to center of viewport (in display coordinates)
				displayLine := v.viewport.YOffset() + v.viewport.VisibleLineCount()/2
				// Map back to document coordinates
				v.cursorLine = v.mapDisplayToDoc(displayLine, v.lineMapping)
				// If mapped to comment line (0), adjust to nearest valid document line
				if v.cursorLine == 0 && v.selectedDoc != nil {
					v.cursorLine = 1 // Default to first line
				}
				v.renderSelection()
				return v, nil
			case "ctrl+u":
				v.viewport.HalfPageUp()
				// Update cursor to center of viewport (in display coordinates)
				displayLine := v.viewport.YOffset() + v.viewport.VisibleLineCount()/2
				// Map back to document coordinates
				v.cursorLine = v.mapDisplayToDoc(displayLine, v.lineMapping)
				// If mapped to comment line (0), adjust to nearest valid document line
				if v.cursorLine == 0 && v.selectedDoc != nil {
					v.cursorLine = 1 // Default to first line
				}
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

	// Don't forward keys to list when in full-screen mode
	if v.fullScreen {
		return v, nil
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
func (v View) View() string {
	var baseView string

	switch {
	case v.selectedDoc == nil:
		// Show helpful message if no document is selected
		messageStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("#c0caf5")).
			Padding(2, 4)

		helpStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("#7aa2f7")).
			Bold(true)

		message := "No document selected\n\n"
		message += helpStyle.Render("Press 'p'") + " to open document picker\n"
		message += helpStyle.Render("Press 'tab'") + " to switch to another view"

		baseView = messageStyle.Render(message)
	case v.fullScreen:
		// Full-screen mode: show viewport with status bar
		contentHeight := v.height - 1 // Reserve 1 line for status bar
		content := v.viewport.View()
		statusBar := v.renderStatusBar()

		// Truncate content if needed to make room for status bar
		contentLines := strings.Split(content, "\n")
		if len(contentLines) > contentHeight {
			contentLines = contentLines[:contentHeight]
		}

		baseView = strings.Join(contentLines, "\n") + "\n" + statusBar
	default:
		// Fallback to list view (should not normally happen)
		baseView = v.list.View()
	}

	// Overlay document picker modal if active (highest priority)
	if v.pickerModal != nil {
		return v.pickerModal.Overlay(baseView, v.width, v.height)
	}

	// Overlay finalization modal if active
	if v.finalizationModal != nil {
		return v.finalizationModal.Overlay(baseView)
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
func (v View) SelectedDocument() *Document {
	item := v.list.SelectedItem()
	if item == nil {
		return nil
	}
	if reviewItem, ok := item.(TreeItem); ok {
		if reviewItem.IsHeader {
			return nil
		}
		return &reviewItem.Document
	}
	return nil
}

// calculateContentHash computes SHA256 hash of file content.
func calculateContentHash(path string) (string, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	hash := sha256.Sum256(content)
	return hex.EncodeToString(hash[:]), nil
}

// loadDocument loads and renders a document for preview.
// Also loads any existing review session from the database.
func (v *View) loadDocument(doc *Document) {
	v.selectedDoc = doc
	if doc == nil {
		v.viewport.SetContent("")
		v.activeSession = nil
		v.fullScreen = false
		return
	}

	log.Debug().
		Str("path", doc.RelPath).
		Str("type", doc.Type.String()).
		Msg("review: loading document")

	// Enter full-screen mode when loading a document
	v.fullScreen = true

	// Adjust viewport size for full-screen
	v.viewport = viewport.New(viewport.WithWidth(v.width), viewport.WithHeight(v.height))

	// Reset cursor to top when loading new document
	v.cursorLine = 1

	// Clear line mapping on document load (will be rebuilt when rendering)
	v.lineMapping = nil

	// Load existing session from database if store is available
	if v.store != nil {
		ctx := context.Background()

		// Calculate current content hash
		currentHash, err := calculateContentHash(doc.Path)
		if err == nil {
			// Try to get session with matching hash
			dbSession, err := v.store.GetSessionByHash(ctx, doc.Path, currentHash)
			if err == nil {
				// Skip finalized sessions - they should not be edited
				if dbSession.IsFinalized() {
					log.Debug().
						Str("session_id", dbSession.ID).
						Str("document", doc.RelPath).
						Msg("review: skipping finalized session")
				} else {
					log.Debug().
						Str("session_id", dbSession.ID).
						Str("document", doc.RelPath).
						Msg("review: loaded existing session")

					// Load comments from database
					dbComments, err := v.store.ListComments(ctx, dbSession.ID)
					if err == nil {
						// Convert to TUI types
						comments := make([]Comment, 0, len(dbComments))
						for _, dbComment := range dbComments {
							comments = append(comments, Comment{
								ID:          dbComment.ID,
								SessionID:   dbComment.SessionID,
								StartLine:   dbComment.StartLine,
								EndLine:     dbComment.EndLine,
								ContextText: dbComment.ContextText,
								CommentText: dbComment.CommentText,
								CreatedAt:   dbComment.CreatedAt,
							})
						}

						v.activeSession = &Session{
							ID:         dbSession.ID,
							DocPath:    dbSession.DocumentPath,
							Comments:   comments,
							CreatedAt:  dbSession.CreatedAt,
							ModifiedAt: time.Now(),
						}
					}
				}
			} else {
				// No matching session found, cleanup stale sessions
				_ = v.store.CleanupStaleSessions(ctx, doc.Path, currentHash)
			}
		}
		// If hash calculation or session load fails, activeSession remains nil
	}

	// Render document using full width
	rendered, err := doc.Render(v.width)
	if err != nil {
		v.viewport.SetContent("Error rendering document: " + err.Error())
		return
	}

	v.viewport.SetContent(rendered)
	v.viewport.GotoTop()

	// Render selection to show comments immediately if session was loaded
	if v.activeSession != nil && len(v.activeSession.Comments) > 0 {
		v.renderSelection()
	}
}

// moveCursorDown moves cursor down by n lines, scrolling if needed.
func (v *View) moveCursorDown(n int) {
	if v.selectedDoc == nil {
		return
	}

	maxLine := len(v.selectedDoc.RenderedLines)
	v.cursorLine = min(v.cursorLine+n, maxLine)
	v.ensureCursorVisible()
}

// moveCursorUp moves cursor up by n lines, scrolling if needed.
func (v *View) moveCursorUp(n int) {
	v.cursorLine = max(v.cursorLine-n, 1)
	v.ensureCursorVisible()
}

// ensureCursorVisible scrolls viewport to keep cursor visible.
// Accounts for status bar in full-screen mode to prevent cursor from being hidden.
// Uses display coordinates when comments are inserted inline.
func (v *View) ensureCursorVisible() {
	// Map cursor line from document coordinates to display coordinates
	displayCursorLine := v.mapDocToDisplay(v.cursorLine, v.lineMapping)

	offset := v.viewport.YOffset()
	visibleHeight := v.viewport.VisibleLineCount()

	// In full-screen mode, reserve 1 line for status bar
	if v.fullScreen {
		visibleHeight--
	}

	// Cursor above viewport - scroll up
	if displayCursorLine < offset+1 {
		v.viewport.SetYOffset(displayCursorLine - 1)
	}

	// Cursor below viewport - scroll down
	// Keep cursor at least 1 line away from bottom to avoid status bar overlap
	if displayCursorLine > offset+visibleHeight {
		v.viewport.SetYOffset(displayCursorLine - visibleHeight)
	}
}

// renderSelection re-renders the document with comments, selection and cursor highlighting.
func (v *View) renderSelection() {
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

	// Insert comments inline if session exists and build line mapping
	if v.activeSession != nil && len(v.activeSession.Comments) > 0 {
		var mappedContent string
		mappedContent, v.lineMapping = v.insertCommentsInline(rendered)
		rendered = mappedContent
	} else {
		// Clear line mapping when no comments
		v.lineMapping = nil
	}

	// Apply cursor and selection highlighting (includes search match highlighting)
	rendered = v.highlightSelection(rendered, v.lineMapping)

	v.viewport.SetContent(rendered)
}

// findSearchMatches finds all lines matching the search query and stores their line numbers.
func (v *View) findSearchMatches() {
	v.searchMatches = nil
	v.searchMatchIndex = 0

	if v.searchQuery == "" || v.selectedDoc == nil {
		return
	}

	// Case-insensitive search
	queryLower := strings.ToLower(v.searchQuery)

	// Search through rendered lines (strip ANSI codes for searching)
	for i, line := range v.selectedDoc.RenderedLines {
		// Strip ANSI codes before searching
		cleanLine := ansiStripPattern.ReplaceAllString(line, "")
		if strings.Contains(strings.ToLower(cleanLine), queryLower) {
			v.searchMatches = append(v.searchMatches, i+1) // Store 1-indexed line numbers
		}
	}
}

// jumpToMatch moves the cursor to the specified line and scrolls to make it visible.
// lineNum should be in document coordinates; this function will map to display coordinates for scrolling.
func (v *View) jumpToMatch(lineNum int) {
	v.cursorLine = lineNum
	v.ensureCursorVisible()
}

// highlightSelection applies background color to cursor and selected lines.
// Also highlights line numbers of commented lines.
// lineMapping maps document line numbers to display line numbers (nil if no comments inserted).
func (v *View) highlightSelection(content string, lineMapping map[int]int) string {
	lines := strings.Split(content, "\n")

	// Build reverse lookup map once for O(1) lookups
	displayToDoc := buildDisplayToDocMap(lineMapping)

	// Get commented line numbers (in document coordinates)
	commentedLines := v.getCommentedLines()

	// Create map of search matches for quick lookup (in display coordinates)
	searchMatchLines := make(map[int]bool)
	for _, docLineNum := range v.searchMatches {
		displayLineNum := v.mapDocToDisplay(docLineNum, lineMapping)
		searchMatchLines[displayLineNum] = true
	}

	// Styles for cursor and selection (no explicit width)
	selectionStyle := lipgloss.NewStyle().Background(lipgloss.Color("#3b4261"))
	cursorStyle := lipgloss.NewStyle().Background(lipgloss.Color("#2a3158"))
	searchMatchStyle := lipgloss.NewStyle().Background(lipgloss.Color("#565f89"))        // Subtle highlight for other matches
	currentSearchMatchStyle := lipgloss.NewStyle().Background(lipgloss.Color("#f7768e")) // Bright for current match

	// Style for commented line numbers
	commentedLineNumStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#1a1b26")). // Dark text
		Background(lipgloss.Color("#e0af68")). // Gold background
		Bold(true)

	// Calculate selection range if in visual mode (map to display coordinates)
	var start, end int
	if v.selectionMode {
		docStart := min(v.selectionStart, v.cursorLine)
		docEnd := max(v.selectionStart, v.cursorLine)
		start = v.mapDocToDisplay(docStart, lineMapping)
		end = v.mapDocToDisplay(docEnd, lineMapping)
	}

	// Determine current search match line number (map to display coordinates)
	var currentSearchLine int
	if len(v.searchMatches) > 0 && v.searchMatchIndex >= 0 && v.searchMatchIndex < len(v.searchMatches) {
		docLine := v.searchMatches[v.searchMatchIndex]
		currentSearchLine = v.mapDocToDisplay(docLine, lineMapping)
	}

	// Map cursor line to display coordinates
	displayCursorLine := v.mapDocToDisplay(v.cursorLine, lineMapping)

	// Apply highlighting (priority: current search > cursor > visual selection > other search > comments > normal)
	for i := range lines {
		displayLineNum := i + 1
		line := lines[i]

		// Map back to document line number for comment checking
		var docLineNum int
		if displayToDoc != nil {
			if doc, ok := displayToDoc[displayLineNum]; ok {
				docLineNum = doc
			} else {
				// Comment line - not a document line
				docLineNum = 0
			}
		} else {
			docLineNum = displayLineNum
		}

		// Check if line will be highlighted with cursor/selection/search
		willBeHighlighted := displayLineNum == currentSearchLine ||
			displayLineNum == displayCursorLine ||
			(v.selectionMode && displayLineNum >= start && displayLineNum <= end) ||
			searchMatchLines[displayLineNum]

		// Only highlight line number for comments if line won't be highlighted otherwise
		if commentedLines[docLineNum] && !willBeHighlighted {
			line = v.highlightLineNumber(line, commentedLineNumStyle)
		}

		// Apply highlighting based on priority
		switch {
		case displayLineNum == currentSearchLine:
			// Current search match (highest priority)
			lines[i] = currentSearchMatchStyle.Render(line)
		case displayLineNum == displayCursorLine:
			// Cursor highlight
			lines[i] = cursorStyle.Render(line)
		case v.selectionMode && displayLineNum >= start && displayLineNum <= end:
			// Visual selection highlight
			lines[i] = selectionStyle.Render(line)
		case searchMatchLines[displayLineNum]:
			// Other search matches (subtle)
			lines[i] = searchMatchStyle.Render(line)
		default:
			lines[i] = line
		}
	}

	return strings.Join(lines, "\n")
}

// highlightLineNumber applies a style to the line number and separator of a rendered line.
// Assumes format: "<number> │ <content>"
func (v *View) highlightLineNumber(line string, style lipgloss.Style) string {
	// Find the separator " │ "
	sepIdx := strings.Index(line, " │ ")
	if sepIdx == -1 {
		return line // No line number found
	}

	// Extract parts
	lineNum := line[:sepIdx]
	separator := " │ "
	content := line[sepIdx+len(separator):]

	// Style the line number + separator together (entire gutter)
	gutter := lineNum + separator
	styledGutter := style.Render(gutter)
	return styledGutter + content
}

// getCommentedLines returns a map of line numbers that have comments.
func (v *View) getCommentedLines() map[int]bool {
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

// mapDocToDisplay maps a document line number to a display line number.
// If lineMapping is nil (no comments inserted), returns the same line number.
func (v *View) mapDocToDisplay(docLine int, lineMapping map[int]int) int {
	if lineMapping == nil {
		return docLine
	}
	if displayLine, ok := lineMapping[docLine]; ok {
		return displayLine
	}
	return docLine // fallback to same line if not in mapping
}

// buildDisplayToDocMap builds a reverse lookup map from display lines to document lines.
// Returns nil if lineMapping is nil (no comments inserted).
func buildDisplayToDocMap(lineMapping map[int]int) map[int]int {
	if lineMapping == nil {
		return nil
	}

	displayToDoc := make(map[int]int, len(lineMapping))
	for docLine, displayLine := range lineMapping {
		displayToDoc[displayLine] = docLine
	}
	return displayToDoc
}

// mapDisplayToDoc maps a display line number back to a document line number.
// If lineMapping is nil (no comments inserted), returns the same line number.
func (v *View) mapDisplayToDoc(displayLine int, lineMapping map[int]int) int {
	if lineMapping == nil {
		return displayLine
	}
	// Reverse lookup: find document line that maps to this display line
	for docLine, dispLine := range lineMapping {
		if dispLine == displayLine {
			return docLine
		}
	}
	// If display line is a comment line (not found in mapping), return 0 or -1
	// to indicate it's not a document line
	return 0
}

// renderStatusBar creates a status bar showing mode and position info.
func (v View) renderStatusBar() string {
	// Show search input when in search mode
	if v.searchMode {
		searchStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("#7aa2f7")).
			Background(lipgloss.Color("#1f2335")).
			Bold(true)

		bgStyle := lipgloss.NewStyle().Background(lipgloss.Color("#1f2335"))
		prefix := searchStyle.Render("/")
		input := v.searchInput.View()
		remaining := max(0, v.width-lipgloss.Width(prefix)-lipgloss.Width(input))

		return prefix + input + bgStyle.Render(strings.Repeat(" ", remaining))
	}

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
	} else if v.searchQuery != "" && len(v.searchMatches) > 0 {
		// Show search match count when search is active
		mode = fmt.Sprintf("SEARCH | Match %d/%d", v.searchMatchIndex+1, len(v.searchMatches))
		modeStyle = modeStyle.Background(lipgloss.Color("#f7768e"))
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

	// Help text
	var helpText string
	if v.selectionMode {
		helpText = "c:comment • v/esc:exit visual"
	} else {
		helpText = "V:visual • p:picker • e:edit • d:delete • /:search • f:finalize • esc:close"
	}
	helpStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#565f89")).
		Background(lipgloss.Color("#1f2335")).
		Padding(0, 1)

	// Build status bar
	leftPart := modeStyle.Render(mode)
	middlePart := helpStyle.Render(helpText)
	rightPart := posStyle.Render(posInfo)

	// Calculate spacing to distribute
	usedWidth := lipgloss.Width(leftPart) + lipgloss.Width(middlePart) + lipgloss.Width(rightPart)
	availableSpace := max(0, v.width-usedWidth)

	// Split spacing: left spacing between mode and help, right spacing between help and position
	leftSpacing := availableSpace / 2
	rightSpacing := availableSpace - leftSpacing

	bgStyle := lipgloss.NewStyle().Background(lipgloss.Color("#1f2335"))
	return leftPart + bgStyle.Render(strings.Repeat(" ", leftSpacing)) + middlePart + bgStyle.Render(strings.Repeat(" ", rightSpacing)) + rightPart
}

// getSelectedText extracts the text from the selected line range.
func (v *View) getSelectedText() string {
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
func (v *View) addComment(commentText string) {
	if v.selectedDoc == nil {
		return
	}

	ctx := context.Background()

	// Initialize session if needed
	if v.activeSession == nil {
		sessionID := uuid.NewString()

		// Create session in database if store is available
		if v.store != nil {
			// Calculate content hash
			contentHash, err := calculateContentHash(v.selectedDoc.Path)
			if err != nil {
				contentHash = "" // Fallback to empty hash
			}

			if contentHash != "" {
				dbSession, err := v.store.CreateSession(ctx, v.selectedDoc.Path, contentHash)
				if err != nil {
					// Session might already exist, try to get it by hash
					dbSession, err = v.store.GetSessionByHash(ctx, v.selectedDoc.Path, contentHash)
					if err != nil {
						// Failed to create or get session, fall back to in-memory only
						sessionID = fmt.Sprintf("session-%d", time.Now().Unix())
					} else {
						sessionID = dbSession.ID
					}
				} else {
					sessionID = dbSession.ID
				}
			}
		}

		v.activeSession = &Session{
			ID:         sessionID,
			DocPath:    v.selectedDoc.Path,
			Comments:   []Comment{},
			CreatedAt:  time.Now(),
			ModifiedAt: time.Now(),
		}
	}

	// Calculate selection range from anchor to cursor
	start := min(v.selectionStart, v.cursorLine)
	end := max(v.selectionStart, v.cursorLine)

	// Create comment
	comment := Comment{
		ID:          uuid.NewString(),
		SessionID:   v.activeSession.ID,
		StartLine:   start,
		EndLine:     end,
		ContextText: v.getSelectedText(),
		CommentText: commentText,
		CreatedAt:   time.Now(),
	}

	// Save to database if store is available
	if v.store != nil {
		dbComment := corereview.Comment{
			ID:          comment.ID,
			SessionID:   comment.SessionID,
			StartLine:   comment.StartLine,
			EndLine:     comment.EndLine,
			ContextText: comment.ContextText,
			CommentText: comment.CommentText,
			CreatedAt:   comment.CreatedAt,
		}
		_ = v.store.SaveComment(ctx, dbComment)
		// Ignore errors - keep comment in memory even if DB save fails
	}

	v.activeSession.Comments = append(v.activeSession.Comments, comment)
	v.activeSession.ModifiedAt = time.Now()

	log.Debug().
		Str("session_id", v.activeSession.ID).
		Int("start_line", comment.StartLine).
		Int("end_line", comment.EndLine).
		Int("total_comments", len(v.activeSession.Comments)).
		Msg("review: added comment")

	// Update comment count in tree
	v.updateTreeItemCommentCount()
}

// updateComment updates the text of an existing comment.
func (v *View) updateComment(commentID, newText string) {
	if v.activeSession == nil {
		return
	}

	ctx := context.Background()

	// Find and update the comment
	for i, comment := range v.activeSession.Comments {
		if comment.ID == commentID {
			v.activeSession.Comments[i].CommentText = newText
			v.activeSession.ModifiedAt = time.Now()

			// Update in database if store is available
			if v.store != nil {
				dbComment := corereview.Comment{
					ID:          comment.ID,
					SessionID:   comment.SessionID,
					StartLine:   comment.StartLine,
					EndLine:     comment.EndLine,
					ContextText: comment.ContextText,
					CommentText: newText,
					CreatedAt:   comment.CreatedAt,
				}
				_ = v.store.UpdateComment(ctx, dbComment)
				// Ignore errors - keep updated text in memory
			}

			log.Debug().
				Str("comment_id", commentID).
				Int("start_line", comment.StartLine).
				Int("end_line", comment.EndLine).
				Msg("review: updated comment")
			break
		}
	}
}

// deleteCommentsAtLine removes all comments that include the specified line number.
func (v *View) deleteCommentsAtLine(lineNum int) {
	if v.activeSession == nil || len(v.activeSession.Comments) == 0 {
		return
	}

	ctx := context.Background()

	// Filter out comments that include this line
	var remainingComments []Comment
	for _, comment := range v.activeSession.Comments {
		// Keep comment if it doesn't include the cursor line
		if lineNum < comment.StartLine || lineNum > comment.EndLine {
			remainingComments = append(remainingComments, comment)
		} else if v.store != nil {
			// Delete from database if store is available
			_ = v.store.DeleteComment(ctx, comment.ID)
			// Ignore errors - deletion is best effort
		}
	}

	v.activeSession.Comments = remainingComments
	v.activeSession.ModifiedAt = time.Now()

	log.Debug().
		Int("line", lineNum).
		Int("remaining_comments", len(remainingComments)).
		Msg("review: deleted comment(s) at line")

	// Update comment count in tree
	v.updateTreeItemCommentCount()

	// Clear session if no comments remain
	if len(v.activeSession.Comments) == 0 {
		v.activeSession = nil
	}
}

// discardReview discards the entire review session, deleting it from the database.
func (v *View) discardReview() tea.Cmd {
	if v.activeSession == nil {
		return nil
	}

	sessionID := v.activeSession.ID

	return func() tea.Msg {
		// Delete from database (CASCADE deletes comments)
		if v.store != nil {
			ctx := context.Background()
			if err := v.store.DeleteSession(ctx, sessionID); err != nil {
				// Return error message but continue with in-memory cleanup
				return fmt.Errorf("failed to delete session from database: %w", err)
			}
		}

		return reviewDiscardedMsg{}
	}
}

// updateTreeItemCommentCount updates the comment count badge in the tree for the current document.
func (v *View) updateTreeItemCommentCount() {
	if v.selectedDoc == nil {
		return
	}

	items := v.list.Items()
	commentCount := 0
	if v.activeSession != nil {
		commentCount = len(v.activeSession.Comments)
	}

	// Find and update the tree item for the current document
	for i, item := range items {
		if treeItem, ok := item.(TreeItem); ok && !treeItem.IsHeader {
			if treeItem.Document.Path == v.selectedDoc.Path {
				treeItem.CommentCount = commentCount
				items[i] = treeItem
				v.list.SetItems(items)
				return
			}
		}
	}
}

// insertCommentsInline inserts comments after their referenced lines.
// Returns the rendered content and a mapping from document line numbers to display line numbers.
func (v *View) insertCommentsInline(content string) (string, map[int]int) {
	lines := strings.Split(content, "\n")
	originalLineCount := len(lines)

	// Group comments by end line
	commentsByLine := make(map[int][]Comment)
	for _, comment := range v.activeSession.Comments {
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
	commentStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#F74D50")). // More vibrant pink
		Background(lipgloss.Color("#282A36")). // Subtle background
		Padding(0, 1).
		Bold(true)

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
			icon := styles.IconProfile
			// Add increased indentation (6 spaces) for visual separation
			indent := "      "
			commentLine := indent + commentStyle.Render(fmt.Sprintf("%s %s", icon, comment.CommentText))
			commentLines = append(commentLines, commentLine)
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

// TreeItem represents an item in the review tree.
type TreeItem struct {
	IsHeader         bool     // True if this is a document type header
	HeaderName       string   // Document type name (e.g., "Plans", "Research")
	Document         Document // The document (when !IsHeader)
	IsLastInType     bool     // True if last document in this type group
	CommentCount     int      // Number of comments on this document
	HasActiveSession bool     // True if document has an active (non-finalized) review session
}

// FilterValue returns the value used for filtering.
func (i TreeItem) FilterValue() string {
	if i.IsHeader {
		return ""
	}
	return i.Document.RelPath
}

// BuildTreeItems converts documents into tree items grouped by type.
// Note: CommentCount is intentionally set to 0 to keep this function simple and stateless.
// Callers that need comment counts should enrich the items separately (see buildTreeItemsWithSessions in picker modal).
func BuildTreeItems(documents []Document) []list.Item {
	if len(documents) == 0 {
		return nil
	}

	items := make([]list.Item, 0)

	// Group documents by type
	groups := make(map[DocumentType][]Document)
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
		header := TreeItem{
			IsHeader:   true,
			HeaderName: docType.String(),
		}
		items = append(items, header)

		// Add documents
		for idx, doc := range docs {
			isLast := idx == len(docs)-1
			item := TreeItem{
				IsHeader:     false,
				Document:     doc,
				IsLastInType: isLast,
				CommentCount: 0, // Intentionally 0 - callers should enrich if needed
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
	reviewItem, ok := item.(TreeItem)
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
func (d ReviewTreeDelegate) renderHeader(item TreeItem, isSelected bool) string {
	nameStyle := d.styles.HeaderNormal
	if isSelected {
		nameStyle = d.styles.HeaderSelected
	}
	return nameStyle.Render(item.HeaderName)
}

// renderDocument renders a document entry with tree prefix.
func (d ReviewTreeDelegate) renderDocument(item TreeItem, isSelected bool) string {
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

	// Active session indicator (shown before comment count)
	var sessionIndicator string
	if item.HasActiveSession {
		// Blue dot to indicate active review session
		indicatorStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#7aa2f7"))
		sessionIndicator = indicatorStyle.Render(" ●")
	}

	// Comment count (if any)
	var comments string
	if item.CommentCount > 0 {
		comments = d.styles.DocMeta.Render(fmt.Sprintf(" (%d)", item.CommentCount))
	}

	return fmt.Sprintf("%s %s%s%s", prefixStyled, name, sessionIndicator, comments)
}

// Accessor methods for cross-package access.

// Width returns the view width.
func (v *View) Width() int {
	return v.width
}

// Height returns the view height.
func (v *View) Height() int {
	return v.height
}

// Store returns the review store.
func (v *View) Store() *stores.ReviewStore {
	return v.store
}

// List returns the list model.
func (v *View) List() *list.Model {
	return &v.list
}

// SetPickerModal sets the document picker modal.
func (v *View) SetPickerModal(modal *DocumentPickerModal) {
	v.pickerModal = modal
}

// HasActiveSession returns whether there is an active review session.
func (v *View) HasActiveSession() bool {
	return v.activeSession != nil
}

// SelectedDocPath returns the path of the selected document, or empty string if none.
func (v *View) SelectedDocPath() string {
	if v.selectedDoc == nil {
		return ""
	}
	return v.selectedDoc.Path
}

// GetAllDocuments returns all documents from the tree items.
func (v *View) GetAllDocuments() []Document {
	var docs []Document
	for _, item := range v.list.Items() {
		if treeItem, ok := item.(TreeItem); ok && !treeItem.IsHeader {
			docs = append(docs, treeItem.Document)
		}
	}
	return docs
}

// ShowDocumentPicker shows the fuzzy search document picker modal.
func (v *View) ShowDocumentPicker() tea.Cmd {
	// Create and show the picker modal
	v.pickerModal = NewDocumentPickerModal(v.GetAllDocuments(), v.width, v.height, v.store)
	return nil
}

// SetHasAgentCommand sets whether the send-claude command is available.
func (v *View) SetHasAgentCommand(has bool) {
	v.hasAgentCommand = has
}

// LoadDocument loads and renders a document for preview.
func (v *View) LoadDocument(doc *Document) {
	v.loadDocument(doc)
}

// CanShowInTabBar returns true if the review view should be shown in tab bar.
// This is true when there's an active session with a selected document.
func (v *View) CanShowInTabBar() bool {
	return v.activeSession != nil && v.selectedDoc != nil
}

// OpenDocumentMsg is a message sent when attempting to open a document.
type OpenDocumentMsg struct {
	Path string
	Err  error
}

// OpenDocumentByPath attempts to open a specific document by path.
// Path can be absolute or relative to context directory.
// Returns a command that sends an OpenDocumentMsg.
func (v *View) OpenDocumentByPath(path string) tea.Cmd {
	return func() tea.Msg {
		// Try to find document by matching path
		var found *Document
		for i := range v.list.Items() {
			item, ok := v.list.Items()[i].(TreeItem)
			if !ok || item.IsHeader {
				continue
			}

			// Check if path matches (either full path or relative path)
			if item.Document.Path == path || item.Document.RelPath == path {
				doc := item.Document
				found = &doc
				break
			}

			// Also check basename match
			if filepath.Base(item.Document.Path) == filepath.Base(path) {
				doc := item.Document
				found = &doc
				break
			}
		}

		if found == nil {
			// Return error message that will be handled by model
			return OpenDocumentMsg{Path: path, Err: fmt.Errorf("document not found")}
		}

		// Return success message with path
		return OpenDocumentMsg{Path: found.Path}
	}
}
