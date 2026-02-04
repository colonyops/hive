package review

import (
	"context"
	"strings"

	"charm.land/bubbles/v2/list"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	lipgloss "charm.land/lipgloss/v2"

	"github.com/hay-kot/hive/internal/stores"
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

	// Create list with tree delegate
	delegate := NewReviewTreeDelegate()
	l := list.New(nil, delegate, 0, 0)
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(false) // We handle filtering manually for fuzzy matching
	l.SetShowTitle(false)
	l.SetShowHelp(false) // Disable built-in help to avoid duplicate help text
	l.Styles.Title = lipgloss.NewStyle()

	modal := &DocumentPickerModal{
		documents:    documents,
		filteredDocs: documents,
		list:         l,
		searchInput:  ti,
		width:        width,
		height:       height,
		store:        store,
	}

	// Build tree items with session information
	items := modal.buildTreeItemsWithSessions(documents)
	l.SetItems(items)

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

	// Check each document for active sessions
	ctx := context.Background()
	for i, item := range items {
		treeItem, ok := item.(TreeItem)
		if !ok || treeItem.IsHeader {
			continue
		}

		// Check if document has an active session
		session, err := m.store.GetSession(ctx, treeItem.Document.Path)
		if err == nil && !session.IsFinalized() {
			// Has active session - mark it
			treeItem.HasActiveSession = true

			// Count comments for badge
			comments, err := m.store.ListComments(ctx, session.ID)
			if err == nil {
				treeItem.CommentCount = len(comments)
			}

			items[i] = treeItem
		}
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
	modalWidth := int(float64(m.width) * 0.8)
	modalHeight := int(float64(m.height) * 0.8)
	if modalWidth < 40 {
		modalWidth = 40
	}
	if modalHeight < 10 {
		modalHeight = 10
	}

	// Set list size (leave room for search input and help)
	listHeight := modalHeight - 5
	if listHeight < 3 {
		listHeight = 3
	}
	m.list.SetSize(modalWidth-4, listHeight)

	// Set search input width (with minimum for placeholder visibility)
	searchWidth := modalWidth - 4
	if searchWidth < 30 {
		searchWidth = 30
	}
	m.searchInput.SetWidth(searchWidth)

	// Build modal content
	title := modalTitleStyle.Render("Select Document")
	searchView := m.searchInput.View()
	listView := m.list.View()

	// Build help text with legend for active sessions
	helpText := "↑/↓ navigate  enter select  esc cancel"
	if m.store != nil {
		legendStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#7aa2f7"))
		helpText += "  " + legendStyle.Render("●") + " active review"
	}
	help := modalHelpStyle.Render(helpText)

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

	return modalStyle.Render(content)
}

// Overlay renders the modal over the background content.
func (m *DocumentPickerModal) Overlay(background string, width, height int) string {
	modal := m.View()

	// Use Compositor/Layer for true overlay
	bgLayer := lipgloss.NewLayer(background)
	modalLayer := lipgloss.NewLayer(modal)

	// Center the modal
	modalW := lipgloss.Width(modal)
	modalH := lipgloss.Height(modal)
	centerX := (width - modalW) / 2
	centerY := (height - modalH) / 2
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
