package tui

import (
	tea "charm.land/bubbletea/v2"
	"github.com/hay-kot/hive/internal/data/db"
	"github.com/hay-kot/hive/internal/data/stores"
	review "github.com/hay-kot/hive/internal/tui/views/review"
)

// ReviewOnlyOptions configures the review-only TUI.
type ReviewOnlyOptions struct {
	Documents  []review.Document
	InitialDoc *review.Document
	ContextDir string
	DB         *db.DB
}

// ReviewOnlyModel is a minimal TUI for reviewing context documents.
type ReviewOnlyModel struct {
	reviewView  review.View
	initialDoc  *review.Document
	width       int
	height      int
	quitting    bool
	initialized bool
}

// NewReviewOnly creates a new review-only TUI model.
func NewReviewOnly(opts ReviewOnlyOptions) ReviewOnlyModel {
	// Create review store from DB queries
	store := stores.NewReviewStore(opts.DB)

	// Create review view
	reviewView := review.New(opts.Documents, opts.ContextDir, store)

	return ReviewOnlyModel{
		reviewView:  reviewView,
		initialDoc:  opts.InitialDoc,
		width:       80,
		height:      24,
		quitting:    false,
		initialized: false,
	}
}

// Init implements tea.Model.
func (m ReviewOnlyModel) Init() tea.Cmd {
	return m.reviewView.Init()
}

// Update implements tea.Model.
func (m ReviewOnlyModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.reviewView.SetSize(msg.Width, msg.Height)

		// After first resize (when we know window size), open picker or load initial doc
		if !m.initialized {
			m.initialized = true
			if m.initialDoc != nil {
				// Load initial document directly
				m.reviewView.LoadDocument(m.initialDoc)
			} else {
				// Open picker in fullscreen mode
				return m, m.reviewView.ShowDocumentPicker()
			}
		}
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			m.quitting = true
			return m, tea.Quit
		}

	case review.ReviewFinalizedMsg:
		// Review completed - exit
		m.quitting = true
		return m, tea.Quit
	}

	// Forward to review view
	var cmd tea.Cmd
	m.reviewView, cmd = m.reviewView.Update(msg)
	return m, cmd
}

// View implements tea.Model.
func (m ReviewOnlyModel) View() tea.View {
	if m.quitting {
		return tea.NewView("")
	}
	return tea.NewView(m.reviewView.View())
}
