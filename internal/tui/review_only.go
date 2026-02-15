package tui

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/colonyops/hive/internal/data/db"
	"github.com/colonyops/hive/internal/data/stores"
	review "github.com/colonyops/hive/internal/tui/views/review"
)

// ReviewOnlyOptions configures the review-only TUI.
type ReviewOnlyOptions struct {
	Documents        []review.Document
	InitialDoc       *review.Document
	ContextDir       string // Directory for saving feedback files (e.g., context directory)
	DB               *db.DB
	CommentLineWidth int
	CopyCommand      string // Shell command for copying to clipboard (e.g., "pbcopy" on macOS)
}

// ReviewOnlyModel is a minimal TUI for reviewing context documents.
type ReviewOnlyModel struct {
	reviewView  review.View
	initialDoc  *review.Document
	contextDir  string // Directory for saving feedback files
	copyCommand string
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
	reviewView := review.New(opts.Documents, opts.ContextDir, store, opts.CommentLineWidth)

	return ReviewOnlyModel{
		reviewView:  reviewView,
		initialDoc:  opts.InitialDoc,
		contextDir:  opts.ContextDir,
		copyCommand: opts.CopyCommand,
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
		// Only handle quit keys if review view doesn't have an active editor
		if !m.reviewView.HasActiveEditor() {
			switch msg.String() {
			case "ctrl+c", "q":
				m.quitting = true
				return m, tea.Quit
			}
		}

	case review.ReviewFinalizedMsg:
		// Print feedback to stderr first (so user can retrieve it even if clipboard fails)
		if msg.Feedback != "" {
			fmt.Fprintln(os.Stderr, "")
			fmt.Fprintln(os.Stderr, "=== Review Feedback ===")
			fmt.Fprintln(os.Stderr, msg.Feedback)
			fmt.Fprintln(os.Stderr, "======================")
			fmt.Fprintln(os.Stderr, "")
		}

		// Try to copy to clipboard (best effort)
		if err := m.copyToClipboard(msg.Feedback); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: Failed to copy to clipboard: %v\n", err)

			// Save to file as fallback
			if filePath, saveErr := m.saveFeedbackToFile(msg.Feedback); saveErr == nil {
				fmt.Fprintf(os.Stderr, "Feedback saved to: %s\n", filePath)
			} else {
				fmt.Fprintf(os.Stderr, "Failed to save feedback to file: %v\n", saveErr)
				fmt.Fprintln(os.Stderr, "Feedback is printed above and can be retrieved from terminal history.")
			}
		}

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

// copyToClipboard copies the given text to the system clipboard using the configured copy command.
func (m ReviewOnlyModel) copyToClipboard(text string) error {
	if m.copyCommand == "" {
		return fmt.Errorf("no copy command configured")
	}

	// Split the command into program and args
	parts := strings.Fields(m.copyCommand)
	if len(parts) == 0 {
		return fmt.Errorf("empty copy command")
	}

	cmd := exec.Command(parts[0], parts[1:]...)
	cmd.Stdin = strings.NewReader(text)
	return cmd.Run()
}

// saveFeedbackToFile saves feedback to a timestamped file in the context directory.
// Returns the path to the saved file.
func (m ReviewOnlyModel) saveFeedbackToFile(feedback string) (string, error) {
	if feedback == "" {
		return "", fmt.Errorf("no feedback to save")
	}

	// Determine save directory
	saveDir := m.contextDir
	if saveDir == "" {
		// Fall back to current directory if context dir not set
		var err error
		saveDir, err = os.Getwd()
		if err != nil {
			return "", fmt.Errorf("failed to get current directory: %w", err)
		}
	}

	// Create a timestamped filename
	timestamp := time.Now().Format("2006-01-02-150405")
	filename := fmt.Sprintf("review-feedback-%s.md", timestamp)
	filePath := filepath.Join(saveDir, filename)

	// Write feedback to file
	if err := os.WriteFile(filePath, []byte(feedback), 0o644); err != nil {
		return "", fmt.Errorf("failed to write file: %w", err)
	}

	return filePath, nil
}
