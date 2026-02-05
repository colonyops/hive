package tui

import (
	tea "charm.land/bubbletea/v2"
	"github.com/hay-kot/hive/internal/core/git"
	"github.com/hay-kot/hive/internal/tui/views/review"
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

// Execute shows document picker on current view, then switches to review when document selected.
// When invoked without an argument, it scopes to the selected session's repository context.
func (c HiveDocReviewCmd) Execute(m *Model) tea.Cmd {
	if m.reviewView == nil {
		// No review view available
		return nil
	}

	// If argument provided, open document directly and switch to review view
	if c.Arg != "" {
		m.activeView = ViewReview
		m.handler.SetActiveView(ViewReview)
		return m.reviewView.OpenDocumentByPath(c.Arg)
	}

	// Get documents scoped to the selected session's repository
	// If a session is selected, use its remote to find the context directory
	var docs []review.Document
	selected := m.selectedSession()
	if selected != nil && selected.Remote != "" {
		owner, repo := git.ExtractOwnerRepo(selected.Remote)
		if owner != "" && repo != "" {
			contextDir := m.cfg.RepoContextDir(owner, repo)
			docs, _ = review.DiscoverDocuments(contextDir)
		}
	}

	// Fallback to the review view's documents if no session-specific docs found
	if len(docs) == 0 {
		docs = m.reviewView.GetAllDocuments()
	}

	if len(docs) == 0 {
		m.err = ErrDocumentNotFound
		return nil
	}

	// Show document picker modal on current view (Sessions)
	// Don't switch to Review until document is selected
	m.docPickerModal = review.NewDocumentPickerModal(docs, m.width, m.height, m.reviewView.Store())
	return nil
}
