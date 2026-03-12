package tui

import (
	tea "charm.land/bubbletea/v2"

	"github.com/colonyops/hive/internal/core/notify"
)

// HiveDocReviewCmd switches to the Docs tab, opening a specific document if Arg is set.
type HiveDocReviewCmd struct {
	Arg string // Optional document path argument
}

// Execute switches to the review tab and optionally opens a specific document.
func (c HiveDocReviewCmd) Execute(m *Model) tea.Cmd {
	if m.reviewView == nil {
		m.publishNotificationf(notify.LevelWarning, "review view is not available")
		return nil
	}

	m.activeView = ViewReview
	m.handler.SetActiveView(ViewReview)

	if c.Arg != "" {
		return m.reviewView.OpenDocumentByPath(c.Arg)
	}

	return nil
}
