package commands

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	tea "charm.land/bubbletea/v2"
	"github.com/colonyops/hive/internal/hive"
	"github.com/colonyops/hive/internal/tui"
	review "github.com/colonyops/hive/internal/tui/views/review"
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
		Usage: "Review and annotate markdown documents",
		Description: `Review command opens a focused TUI for reviewing markdown documents.

The review TUI supports document navigation, inline comments, search,
and document picking. Comments are persisted per-document with session IDs.

With --file, you can review any markdown file without requiring context directory.
Without --file, the picker mode discovers documents from your context directory.

Examples:
  hive review                        # Open picker (requires context dir)
  hive review --latest               # Open latest document (requires context dir)
  hive review -f plans/my.md         # Open file relative to context dir
  hive review -f ./notes.md          # Open file relative to current directory
  hive review -f /tmp/notes.md       # Open file with absolute path`,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "file",
				Aliases:     []string{"f"},
				Usage:       "path to markdown file (absolute or relative to cwd)",
				Destination: &cmd.file,
			},
			&cli.BoolFlag{
				Name:        "latest",
				Usage:       "open most recently modified document (requires context dir)",
				Destination: &cmd.latest,
			},
		},
		Action: cmd.run,
	})

	return app
}

func (cmd *ReviewCmd) run(ctx context.Context, c *cli.Command) error {
	// If --file is specified, load directly without context directory requirement
	if cmd.file != "" {
		return cmd.runWithDirectFile(ctx)
	}

	// For picker/latest modes, require context directory
	return cmd.runWithContextDir(ctx, c)
}

// runWithDirectFile loads a specific file directly without context directory requirements.
func (cmd *ReviewCmd) runWithDirectFile(ctx context.Context) error {
	// Resolve file path (absolute or relative to cwd)
	var targetPath string
	if filepath.IsAbs(cmd.file) {
		targetPath = cmd.file
	} else {
		cwd, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("failed to get current directory: %w", err)
		}
		targetPath = filepath.Join(cwd, cmd.file)
	}

	// Clean the path
	targetPath = filepath.Clean(targetPath)

	// Verify file exists
	info, err := os.Stat(targetPath)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("file not found: %s", targetPath)
		}
		return fmt.Errorf("failed to access file: %w", err)
	}

	if info.IsDir() {
		return fmt.Errorf("path is a directory, not a file: %s", targetPath)
	}

	// Create a single document for the specified file
	doc := review.Document{
		Path:    targetPath,
		RelPath: filepath.Base(targetPath),
		ModTime: info.ModTime(),
	}

	// Launch review TUI with single document
	// contextDir is the parent directory of the file
	contextDir := filepath.Dir(targetPath)
	return cmd.launchReviewTUI(ctx, []review.Document{doc}, &doc, contextDir)
}

// runWithContextDir uses context directory discovery for picker/latest modes.
func (cmd *ReviewCmd) runWithContextDir(ctx context.Context, c *cli.Command) error {
	// Resolve context directory with session filtering
	contextDir, err := cmd.resolveContextDir(ctx)
	if err != nil {
		return err
	}

	// Check if directory exists before discovering
	if _, err := os.Stat(contextDir); os.IsNotExist(err) {
		return fmt.Errorf("context directory does not exist: %s\nRun 'hive ctx init' to create it", contextDir)
	} else if err != nil {
		return fmt.Errorf("failed to access context directory %s: %w", contextDir, err)
	}

	// Discover documents from context directory
	documents, err := review.DiscoverDocuments(contextDir)
	if err != nil {
		return fmt.Errorf("failed to discover documents: %w", err)
	}

	if len(documents) == 0 {
		if _, err := fmt.Fprintf(c.Root().Writer, "No documents found in %s\n", contextDir); err != nil {
			return fmt.Errorf("failed to write output: %w", err)
		}
		if _, err := fmt.Fprintln(c.Root().Writer, "Create .md or .txt files in subdirectories: plans/, research/, context/"); err != nil {
			return fmt.Errorf("failed to write output: %w", err)
		}
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
	}

	// Launch review TUI
	return cmd.launchReviewTUI(ctx, documents, initialDoc, contextDir)
}

// launchReviewTUI starts the review-only TUI with the given documents.
func (cmd *ReviewCmd) launchReviewTUI(ctx context.Context, documents []review.Document, initialDoc *review.Document, contextDir string) error {
	// Create review-only options
	opts := tui.ReviewOnlyOptions{
		Documents:        documents,
		InitialDoc:       initialDoc,
		ContextDir:       contextDir,
		DB:               cmd.app.DB,
		CommentLineWidth: cmd.app.Config.Review.CommentLineWidthOrDefault(),
		CopyCommand:      cmd.app.Config.CopyCommand,
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

// resolveContextDir resolves the context directory for the current working directory.
// Uses the same logic as 'hive ctx' - detects git remote from cwd and resolves to context dir.
func (cmd *ReviewCmd) resolveContextDir(ctx context.Context) (string, error) {
	// Let ResolveDir detect from current working directory
	// This uses the same git remote detection as 'hive ctx'
	contextDir, err := cmd.app.Context.ResolveDir(ctx, "", false)
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
