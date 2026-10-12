package commands

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	tea "charm.land/bubbletea/v2"
	"github.com/colonyops/hive/cmd/hive/internal/app"
	"github.com/colonyops/hive/cmd/hive/internal/tui"
	review "github.com/colonyops/hive/cmd/hive/internal/tui/views/review"
	"github.com/urfave/cli/v3"
)

type ReviewCmd struct {
	flags  *Flags
	app    *app.App
	file   string
	latest bool
}

// NewReviewCmd creates a new review command.
func NewReviewCmd(flags *Flags, app *app.App) *ReviewCmd {
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

	// The file's own folder is listed on a change. Its space is known only
	// when the file sits in the drive, directly or through a checkout's .hive
	// link; outside the drive nothing refreshes the list.
	scope := review.Scope{Dir: filepath.Dir(targetPath)}
	if space, _, ok := cmd.app.Drive().SpaceAt(scope.Dir); ok {
		scope.Space = space
	}
	return cmd.launchReviewTUI(ctx, []review.Document{doc}, &doc, scope)
}

// runWithContextDir uses context directory discovery for picker/latest modes.
func (cmd *ReviewCmd) runWithContextDir(ctx context.Context, c *cli.Command) error {
	scope, err := cmd.resolveScope(ctx)
	if err != nil {
		return err
	}
	contextDir := scope.Dir

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
	return cmd.launchReviewTUI(ctx, documents, initialDoc, scope)
}

// launchReviewTUI runs the review-only TUI. The drive watcher runs for the
// program's lifetime so a document edited meanwhile refreshes the list.
func (cmd *ReviewCmd) launchReviewTUI(ctx context.Context, documents []review.Document, initialDoc *review.Document, scope review.Scope) error {
	drive := cmd.app.Drive()
	if err := drive.Watch(ctx); err != nil {
		cmd.app.Logger.Warn().Err(err).Msg("drive watcher unavailable; documents will not refresh while open")
	}
	defer func() {
		if err := drive.Close(); err != nil {
			cmd.app.Logger.Debug().Err(err).Msg("drive watcher not closed cleanly")
		}
	}()

	m := tui.NewReviewOnly(tui.ReviewOnlyOptions{
		Logger:      cmd.app.Logger,
		Bus:         cmd.app.Bus(),
		Documents:   documents,
		InitialDoc:  initialDoc,
		Scope:       scope,
		Reviews:     cmd.app.Reviews(),
		CopyCommand: cmd.app.Config.CopyCommand,
	})
	if _, err := tea.NewProgram(m).Run(); err != nil {
		return fmt.Errorf("run review TUI: %w", err)
	}
	return nil
}

// resolveScope is the space of the repository behind the current directory,
// the detection 'hive ctx' uses, with its directory created.
func (cmd *ReviewCmd) resolveScope(ctx context.Context) (review.Scope, error) {
	space, err := cmd.app.Drive().ResolveSpace(ctx, "", false)
	if err != nil {
		return review.Scope{}, err
	}
	dir, err := cmd.app.Drive().SpaceDir(space)
	if err != nil {
		return review.Scope{}, err
	}
	if _, err := cmd.app.Drive().Init(dir); err != nil {
		return review.Scope{}, err
	}
	return review.Scope{Space: space, Dir: dir}, nil
}
