package commands

import (
	"context"
	"fmt"
	"path/filepath"

	tea "charm.land/bubbletea/v2"
	"github.com/hay-kot/hive/internal/hive"
	"github.com/hay-kot/hive/internal/tui"
	review "github.com/hay-kot/hive/internal/tui/views/review"
	"github.com/urfave/cli/v3"
)

type ReviewCmd struct {
	flags  *Flags
	app    *hive.App
	file   string
	latest bool
}

// NewReviewCmd creates a new review command.
func NewReviewCmd(flags *Flags, app *hive.App) *ReviewCmd {
	return &ReviewCmd{flags: flags, app: app}
}

// Register adds the review command to the application.
func (cmd *ReviewCmd) Register(app *cli.Command) *cli.Command {
	app.Commands = append(app.Commands, &cli.Command{
		Name:  "review",
		Usage: "Review context documents (plans, research, etc.)",
		Description: `Review command opens a focused TUI for reviewing context documents.

The review TUI supports document navigation, inline comments, search,
and document picking. Comments are persisted per-document with session IDs.

Examples:
  hive review                    # Open picker to select a document
  hive review --latest           # Open most recently modified document
  hive review -f plans/my.md     # Open specific document directly`,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "file",
				Aliases:     []string{"f"},
				Usage:       "open specific document file (relative to context dir)",
				Destination: &cmd.file,
			},
			&cli.BoolFlag{
				Name:        "latest",
				Usage:       "open most recently modified document",
				Destination: &cmd.latest,
			},
		},
		Action: cmd.run,
	})

	return app
}

func (cmd *ReviewCmd) run(ctx context.Context, c *cli.Command) error {
	// Resolve context directory with session filtering
	contextDir, err := cmd.resolveContextDir(ctx)
	if err != nil {
		return err
	}

	// Discover documents from context directory
	documents, err := review.DiscoverDocuments(contextDir)
	if err != nil {
		return fmt.Errorf("failed to discover documents: %w", err)
	}

	if len(documents) == 0 {
		_, _ = fmt.Fprintf(c.Root().Writer, "No documents found in %s\n", contextDir)
		_, _ = fmt.Fprintln(c.Root().Writer, "Create .md or .txt files in subdirectories: plans/, research/, context/")
		return nil
	}

	// Determine initial document selection
	var initialDoc *review.Document
	if cmd.latest {
		// Use latest document
		controller := review.NewPickerController(documents)
		initialDoc = controller.GetLatest()
		if initialDoc == nil {
			return fmt.Errorf("no documents found")
		}
	} else if cmd.file != "" {
		// Use specific file
		absPath, err := filepath.Abs(cmd.file)
		if err != nil {
			return fmt.Errorf("resolve file path: %w", err)
		}

		// Search for matching document
		for i := range documents {
			if documents[i].Path == absPath {
				initialDoc = &documents[i]
				break
			}
		}

		if initialDoc == nil {
			return fmt.Errorf("file not found in context directory: %s", cmd.file)
		}
	}

	// Launch review TUI
	return cmd.launchReviewTUI(ctx, documents, initialDoc, contextDir)
}

// launchReviewTUI starts the review-only TUI with the given documents.
func (cmd *ReviewCmd) launchReviewTUI(ctx context.Context, documents []review.Document, initialDoc *review.Document, contextDir string) error {
	// Create review-only options
	opts := tui.ReviewOnlyOptions{
		Documents:  documents,
		InitialDoc: initialDoc,
		ContextDir: contextDir,
		DB:         cmd.app.DB,
	}

	// Create review-only model
	m := tui.NewReviewOnly(opts)

	// Create program
	p := tea.NewProgram(m)

	// Run program
	if _, err := p.Run(); err != nil {
		return fmt.Errorf("run review TUI: %w", err)
	}

	return nil
}

// resolveContextDir resolves the context directory, preferring the current session's repo.
func (cmd *ReviewCmd) resolveContextDir(ctx context.Context) (string, error) {
	// Detect current session
	sessionID, err := cmd.app.Sessions.DetectSession(ctx)
	if err != nil {
		return "", err
	}

	var repo string
	if sessionID != "" {
		// Get session to extract remote
		sess, err := cmd.app.Sessions.GetSession(ctx, sessionID)
		if err != nil {
			return "", err
		}
		repo = sess.Remote
	}

	// Resolve context dir (uses repo if set, otherwise detects from cwd)
	contextDir, err := cmd.app.Context.ResolveDir(ctx, repo, false)
	if err != nil {
		return "", err
	}

	// Ensure context dir exists
	_, err = cmd.app.Context.Init(contextDir)
	if err != nil {
		return "", err
	}

	return contextDir, nil
}
