package tui

import (
	"path/filepath"

	tea "charm.land/bubbletea/v2"
)

// DocumentNotFoundError is returned when a document cannot be found.
type DocumentNotFoundError struct{}

func (e *DocumentNotFoundError) Error() string {
	return "document not found"
}

// ErrDocumentNotFound is returned when a document cannot be found.
var ErrDocumentNotFound = &DocumentNotFoundError{}

// HiveDocReviewCmd activates the review tab with optional document selection.
type HiveDocReviewCmd struct {
	Arg string // Optional document path argument
}

// Execute switches to review view and optionally opens a specific document.
func (c HiveDocReviewCmd) Execute(m *Model) tea.Cmd {
	// Switch to review view
	m.activeView = ViewReview

	if m.reviewView == nil {
		// No review view available
		return nil
	}

	// If argument provided, try to open specific document
	if c.Arg != "" {
		return m.reviewView.OpenDocument(c.Arg)
	}

	// Otherwise show document picker modal
	return m.reviewView.ShowDocumentPicker()
}

// openDocumentMsg is sent when a document should be opened.
type openDocumentMsg struct {
	path string
	err  error
}

// OpenDocument attempts to open a specific document by path.
// Path can be absolute or relative to context directory.
func (v *ReviewView) OpenDocument(path string) tea.Cmd {
	return func() tea.Msg {
		// Try to find document by matching path
		var found *ReviewDocument
		for i := range v.list.Items() {
			item, ok := v.list.Items()[i].(ReviewTreeItem)
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
			return openDocumentMsg{path: path, err: ErrDocumentNotFound}
		}

		return openDocumentMsg{path: found.Path}
	}
}

// ShowDocumentPicker shows the fuzzy search document picker modal.
func (v *ReviewView) ShowDocumentPicker() tea.Cmd {
	// Create and show the picker modal
	v.pickerModal = NewDocumentPickerModal(v.getAllDocuments(), v.width, v.height)
	return nil
}

// getAllDocuments returns all documents from the tree items.
func (v *ReviewView) getAllDocuments() []ReviewDocument {
	var docs []ReviewDocument
	for _, item := range v.list.Items() {
		if treeItem, ok := item.(ReviewTreeItem); ok && !treeItem.IsHeader {
			docs = append(docs, treeItem.Document)
		}
	}
	return docs
}
