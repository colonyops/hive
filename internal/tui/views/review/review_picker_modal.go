package review

import (
	"context"
	"strings"

	"charm.land/bubbles/v2/list"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	lipgloss "charm.land/lipgloss/v2"
	"github.com/rs/zerolog/log"

	"github.com/hay-kot/hive/internal/core/styles"
	"github.com/hay-kot/hive/internal/data/stores"
)

// DocumentPickerModal is a fuzzy search modal for selecting documents.
type DocumentPickerModal struct {
	documents    []Document
	filteredDocs []Document
	list         list.Model
	searchInput  textinput.Model
	width        int
	height       int
	cancelled    bool
	selectedDoc  *Document
	filterQuery  string
	store        *stores.ReviewStore // For checking which documents have active sessions
}

// NewDocumentPickerModal creates a new document picker modal.
// If store is provided, documents with active review sessions will be highlighted.
func NewDocumentPickerModal(documents []Document, width, height int, store *stores.ReviewStore) *DocumentPickerModal {
	// Create search input
	ti := textinput.New()
	ti.Placeholder = "Type to filter documents..."
	ti.CharLimit = 100
	ti.Focus()

	// Enable paste
	ti.KeyMap.Paste.SetEnabled(true)

	// Calculate initial modal and list dimensions
	modalWidth := max(int(float64(width)*0.8), 40)
	modalHeight := max(int(float64(height)*0.8), 10)
	listHeight := max(modalHeight-5, 3)

	// Build modal struct first so we can call buildTreeItemsWithSessions
	modal := &DocumentPickerModal{
		documents:    documents,
		filteredDocs: documents,
		searchInput:  ti,
		width:        width,
		height:       height,
		store:        store,
	}

	// Build tree items with session information BEFORE creating list
	items := modal.buildTreeItemsWithSessions(documents)

	// Create list with items, delegate, and initial size
	delegate := NewReviewTreeDelegate()
	l := list.New(items, delegate, modalWidth-4, listHeight)
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(false) // We handle filtering manually for fuzzy matching
	l.SetShowTitle(false)
	l.SetShowHelp(false) // Disable built-in help to avoid duplicate help text
	l.Styles.Title = lipgloss.NewStyle()

	modal.list = l

	// Select first non-header item by default
	if len(items) > 0 {
		for i, item := range items {
			if treeItem, ok := item.(TreeItem); ok && !treeItem.IsHeader {
				l.Select(i)
				break
			}
		}
	}

	return modal
}

// Update handles messages for the picker modal.
func (m *DocumentPickerModal) Update(msg tea.Msg) (*DocumentPickerModal, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}

	switch keyMsg.String() {
	case "esc", "q":
		m.cancelled = true
		return m, nil
	case "enter":
		// Select current item (skip headers)
		if item := m.list.SelectedItem(); item != nil {
			if treeItem, ok := item.(TreeItem); ok && !treeItem.IsHeader {
				m.selectedDoc = &treeItem.Document
			}
		}
		return m, nil
	case "up", "down":
		// Navigation - forward to list
		var cmd tea.Cmd
		m.list, cmd = m.list.Update(msg)
		return m, cmd
	default:
		// Text input - update filter
		var cmd tea.Cmd
		m.searchInput, cmd = m.searchInput.Update(msg)
		m.filterQuery = m.searchInput.Value()
		m.updateFilter()
		return m, cmd
	}
}

// updateFilter applies fuzzy filtering to the document list.
func (m *DocumentPickerModal) updateFilter() {
	query := strings.ToLower(m.filterQuery)

	if query == "" {
		// No filter - show all documents
		m.filteredDocs = m.documents
	} else {
		// Fuzzy match on filename and path
		filtered := make([]Document, 0)
		for _, doc := range m.documents {
			if fuzzyMatch(doc.RelPath, query) {
				filtered = append(filtered, doc)
			}
		}
		m.filteredDocs = filtered
	}

	// Rebuild tree items with session information
	items := m.buildTreeItemsWithSessions(m.filteredDocs)
	m.list.SetItems(items)

	// Reset selection to first non-header item
	if len(items) > 0 {
		// Find first non-header item
		for i, item := range items {
			if treeItem, ok := item.(TreeItem); ok && !treeItem.IsHeader {
				m.list.Select(i)
				break
			}
		}
	}
}

// buildTreeItemsWithSessions builds tree items and marks which documents have active sessions.
func (m *DocumentPickerModal) buildTreeItemsWithSessions(documents []Document) []list.Item {
	// Build base items
	items := BuildTreeItems(documents)

	// If no store, return items as-is
	if m.store == nil {
		return items
	}

	// Fetch all active sessions with counts in one query (optimized)
	ctx := context.Background()
	sessionMap, err := m.store.GetAllActiveSessionsWithCounts(ctx)
	if err != nil {
		// If query fails, return items without session info
		log.Debug().Err(err).Msg("review: failed to fetch active sessions")
		return items
	}

	// Mark items with active sessions using the pre-fetched map
	for i, item := range items {
		treeItem, ok := item.(TreeItem)
		if !ok || treeItem.IsHeader {
			continue
		}

		// Check if document has an active session in the map
		if sessionInfo, found := sessionMap[treeItem.Document.Path]; found {
			// Has active session - mark it
			treeItem.HasActiveSession = true
			treeItem.CommentCount = sessionInfo.CommentCount
		}

		items[i] = treeItem
	}

	return items
}

// fuzzyMatch checks if target contains all characters from query in order (case-insensitive).
func fuzzyMatch(target, query string) bool {
	target = strings.ToLower(target)
	queryIdx := 0

	for i := 0; i < len(target) && queryIdx < len(query); i++ {
		if target[i] == query[queryIdx] {
			queryIdx++
		}
	}

	return queryIdx == len(query)
}

// View renders the picker modal.
func (m *DocumentPickerModal) View() string {
	// Calculate modal dimensions (80% of screen)
	modalWidth := max(int(float64(m.width)*0.8), 40)
	modalHeight := max(int(float64(m.height)*0.8), 10)

	// Set list size (leave room for search input and help)
	listHeight := max(modalHeight-5, 3)
	m.list.SetSize(modalWidth-4, listHeight)

	// Set search input width (with minimum for placeholder visibility)
	searchWidth := max(modalWidth-4, 30)
	m.searchInput.SetWidth(searchWidth)

	// Build modal content
	title := styles.ModalTitleStyle.Render("Select Document")
	searchView := m.searchInput.View()
	listView := m.list.View()

	// Build help text with legend for active sessions
	helpText := "↑/↓ navigate  enter select  esc cancel"
	if m.store != nil {
		legendStyle := lipgloss.NewStyle().Foreground(styles.ColorPrimary)
		helpText += "  " + legendStyle.Render("●") + " active review"
	}
	help := styles.ModalHelpStyle.Render(helpText)

	content := lipgloss.JoinVertical(
		lipgloss.Left,
		title,
		"",
		searchView,
		"",
		listView,
		"",
		help,
	)

	return styles.ModalStyle.Render(content)
}

// Overlay renders the modal over the background content.
func (m *DocumentPickerModal) Overlay(background string, width, height int) string {
	modal := m.View()

	// Use Compositor/Layer for true overlay
	bgLayer := lipgloss.NewLayer(background)
	modalLayer := lipgloss.NewLayer(modal)

	// Center the modal (clamped to 0 for tiny terminals)
	modalW := lipgloss.Width(modal)
	modalH := lipgloss.Height(modal)
	centerX := max((width-modalW)/2, 0)
	centerY := max((height-modalH)/2, 0)
	modalLayer.X(centerX).Y(centerY).Z(1)

	compositor := lipgloss.NewCompositor(bgLayer, modalLayer)
	return compositor.Render()
}

// Cancelled returns true if the user cancelled the picker.
func (m *DocumentPickerModal) Cancelled() bool {
	return m.cancelled
}

// SelectedDocument returns the selected document, or nil if none selected.
func (m *DocumentPickerModal) SelectedDocument() *Document {
	return m.selectedDoc
}
