package tui

import (
	"strings"

	"charm.land/bubbles/v2/list"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	lipgloss "charm.land/lipgloss/v2"
)

// DocumentPickerModal is a fuzzy search modal for selecting documents.
type DocumentPickerModal struct {
	documents    []ReviewDocument
	filteredDocs []ReviewDocument
	list         list.Model
	searchInput  textinput.Model
	width        int
	height       int
	cancelled    bool
	selectedDoc  *ReviewDocument
	filterQuery  string
}

// NewDocumentPickerModal creates a new document picker modal.
func NewDocumentPickerModal(documents []ReviewDocument, width, height int) *DocumentPickerModal {
	// Create search input
	ti := textinput.New()
	ti.Placeholder = "Type to filter documents..."
	ti.CharLimit = 100
	ti.Focus()

	// Build tree items (headers + documents grouped by type)
	items := BuildReviewTreeItems(documents)

	// Create list with tree delegate
	delegate := NewReviewTreeDelegate()
	l := list.New(items, delegate, 0, 0)
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(false) // We handle filtering manually for fuzzy matching
	l.SetShowTitle(false)
	l.Styles.Title = lipgloss.NewStyle()

	return &DocumentPickerModal{
		documents:    documents,
		filteredDocs: documents,
		list:         l,
		searchInput:  ti,
		width:        width,
		height:       height,
	}
}

// Update handles messages for the picker modal.
func (m *DocumentPickerModal) Update(msg tea.Msg) (*DocumentPickerModal, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			m.cancelled = true
			return m, nil
		case "enter":
			// Select current item (skip headers)
			if item := m.list.SelectedItem(); item != nil {
				if treeItem, ok := item.(ReviewTreeItem); ok && !treeItem.IsHeader {
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

	return m, nil
}

// updateFilter applies fuzzy filtering to the document list.
func (m *DocumentPickerModal) updateFilter() {
	query := strings.ToLower(m.filterQuery)

	if query == "" {
		// No filter - show all documents
		m.filteredDocs = m.documents
	} else {
		// Fuzzy match on filename and path
		filtered := make([]ReviewDocument, 0)
		for _, doc := range m.documents {
			if fuzzyMatch(doc.RelPath, query) {
				filtered = append(filtered, doc)
			}
		}
		m.filteredDocs = filtered
	}

	// Rebuild tree items
	items := BuildReviewTreeItems(m.filteredDocs)
	m.list.SetItems(items)

	// Reset selection to first non-header item
	if len(items) > 0 {
		// Find first non-header item
		for i, item := range items {
			if treeItem, ok := item.(ReviewTreeItem); ok && !treeItem.IsHeader {
				m.list.Select(i)
				break
			}
		}
	}
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

	// Build modal content
	title := modalTitleStyle.Render("Select Document")
	searchView := m.searchInput.View()
	listView := m.list.View()
	help := modalHelpStyle.Render("↑/↓ navigate  enter select  esc cancel")

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
func (m *DocumentPickerModal) SelectedDocument() *ReviewDocument {
	return m.selectedDoc
}
