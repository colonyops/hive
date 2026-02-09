package review

import (
	"charm.land/bubbles/v2/list"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
)

// PickerView handles the view and user interaction for the document picker.
type PickerView struct {
	controller   PickerController
	list         list.Model
	searchInput  textinput.Model
	width        int
	height       int
	selectedDoc  *Document
	cancelled    bool
	filterQuery  string
}

// NewPickerView creates a new PickerView with the provided controller and dimensions.
func NewPickerView(controller PickerController, width, height int) PickerView {
	// Create search input
	ti := textinput.New()
	ti.Placeholder = "Type to filter documents..."
	ti.CharLimit = 100
	ti.Focus()

	// Calculate initial list dimensions
	modalWidth := max(int(float64(width)*0.8), 40)
	modalHeight := max(int(float64(height)*0.8), 10)
	listHeight := max(modalHeight-5, 3)

	// Build tree items with star indicators for recent files
	items := buildTreeItemsWithIndicators(controller)

	// Create list
	delegate := NewReviewTreeDelegate()
	l := list.New(items, delegate, modalWidth-4, listHeight)
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(false) // Manual filtering for fuzzy matching
	l.SetShowTitle(false)
	l.SetShowHelp(false)

	view := PickerView{
		controller:  controller,
		list:        l,
		searchInput: ti,
		width:       width,
		height:      height,
	}

	// Select first non-header item by default
	view.selectFirstDocument()

	return view
}

// buildTreeItemsWithIndicators builds tree items and adds star prefix (★) for recent files.
func buildTreeItemsWithIndicators(controller PickerController) []list.Item {
	// Get all documents from controller
	documents := controller.documents

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

		// Add documents with star indicator for recent files
		for idx, doc := range docs {
			isLast := idx == len(docs)-1

			// Check if document is recent and add star prefix
			relPath := doc.RelPath
			if controller.IsRecent(doc) {
				relPath = "★ " + relPath
			}

			// Create a copy of the document with potentially modified RelPath
			modifiedDoc := doc
			modifiedDoc.RelPath = relPath

			item := TreeItem{
				IsHeader:     false,
				Document:     modifiedDoc,
				IsLastInType: isLast,
				CommentCount: 0,
			}
			items = append(items, item)
		}
	}

	return items
}

// Update handles messages for the picker view.
func (v PickerView) Update(msg tea.Msg) (PickerView, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return v, nil
	}

	switch keyMsg.String() {
	case "esc", "q":
		v.cancelled = true
		return v, nil
	case "enter":
		// Select current item (skip headers)
		if item := v.list.SelectedItem(); item != nil {
			if treeItem, ok := item.(TreeItem); ok && !treeItem.IsHeader {
				v.selectedDoc = &treeItem.Document
			}
		}
		return v, nil
	case "up", "down":
		// Navigation - forward to list
		var cmd tea.Cmd
		v.list, cmd = v.list.Update(msg)
		return v, cmd
	default:
		// Text input - update filter
		var cmd tea.Cmd
		v.searchInput, cmd = v.searchInput.Update(msg)
		v.filterQuery = v.searchInput.Value()
		v.updateFilter()
		return v, cmd
	}
}

// updateFilter applies filtering to the document list using the controller.
func (v *PickerView) updateFilter() {
	// Use controller's Filter method
	filtered := v.controller.Filter(v.filterQuery)

	// Rebuild tree items with indicators
	tempController := v.controller
	tempController.documents = filtered
	items := buildTreeItemsWithIndicators(tempController)
	v.list.SetItems(items)

	// Reset selection to first non-header item
	v.selectFirstDocument()
}

// selectFirstDocument selects the first non-header item in the list.
func (v *PickerView) selectFirstDocument() {
	items := v.list.Items()
	if len(items) == 0 {
		return
	}

	for i, item := range items {
		if treeItem, ok := item.(TreeItem); ok && !treeItem.IsHeader {
			v.list.Select(i)
			break
		}
	}
}

// View renders the picker view.
// This returns the core content without modal styling.
func (v PickerView) View() string {
	// Render search input and list
	searchView := v.searchInput.View()
	listView := v.list.View()

	return searchView + "\n\n" + listView
}

// SelectedDoc returns the selected document, or nil if none selected.
func (v PickerView) SelectedDoc() *Document {
	return v.selectedDoc
}

// Cancelled returns true if the user cancelled the picker.
func (v PickerView) Cancelled() bool {
	return v.cancelled
}
