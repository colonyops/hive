package commands

import (
	"context"
	"fmt"
	"os"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/bluekeyes/go-gitdiff/gitdiff"
	"github.com/hay-kot/hive/internal/core/git"
	"github.com/hay-kot/hive/internal/hive"
	"github.com/hay-kot/hive/internal/tui/diff"
	"github.com/hay-kot/hive/pkg/executil"
	"github.com/urfave/cli/v3"
)

type ReviewCmd struct {
	flags *Flags
	app   *hive.App

	// diff flags
	diffStaged bool
	diffBase   string
	diffName   string
}

// NewReviewCmd creates a new review command.
func NewReviewCmd(flags *Flags, app *hive.App) *ReviewCmd {
	return &ReviewCmd{flags: flags, app: app}
}

// Register adds the review command to the application.
func (cmd *ReviewCmd) Register(app *cli.Command) *cli.Command {
	app.Commands = append(app.Commands, &cli.Command{
		Name:  "review",
		Usage: "Interactive code review tools",
		Description: `Review commands provide TUI interfaces for reviewing code and diffs.

Use 'hive review diff' to review git diffs with inline comment support.`,
		Commands: []*cli.Command{
			cmd.diffCmd(),
		},
	})

	return app
}

func (cmd *ReviewCmd) diffCmd() *cli.Command {
	return &cli.Command{
		Name:  "diff",
		Usage: "Review git diffs in an interactive TUI",
		Description: `Launches an interactive TUI for reviewing git diffs with inline comment support.

By default, reviews uncommitted changes (working directory + staged).
Use --staged to review only staged changes.
Use --base <branch> to review changes between HEAD and the specified branch.

Examples:
  hive review diff                    # review all uncommitted changes
  hive review diff --staged           # review only staged changes
  hive review diff --base main        # review changes vs main branch
  hive review diff --name my-review   # name the review session`,
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:        "staged",
				Usage:       "review only staged changes",
				Destination: &cmd.diffStaged,
			},
			&cli.StringFlag{
				Name:        "base",
				Aliases:     []string{"b"},
				Usage:       "base branch to compare against (e.g., main)",
				Destination: &cmd.diffBase,
			},
			&cli.StringFlag{
				Name:        "name",
				Aliases:     []string{"n"},
				Usage:       "name for the review session",
				Destination: &cmd.diffName,
			},
		},
		Action: cmd.runDiff,
	}
}

func (cmd *ReviewCmd) runDiff(ctx context.Context, c *cli.Command) error {
	// Get current working directory
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get working directory: %w", err)
	}

	// Determine diff mode from flags
	var diffOpts git.DiffOptions
	if cmd.diffBase != "" {
		diffOpts.Mode = git.DiffBranch
		diffOpts.BaseBranch = cmd.diffBase
	} else if cmd.diffStaged {
		diffOpts.Mode = git.DiffStaged
	} else {
		diffOpts.Mode = git.DiffUncommitted
	}

	// Get git diff
	exec := &executil.RealExecutor{}
	gitExec := git.NewExecutor(cmd.app.Config.GitPath, exec)
	diffContent, err := gitExec.GetDiff(ctx, cwd, diffOpts)
	if err != nil {
		return fmt.Errorf("get git diff: %w", err)
	}

	// Check if there are any changes
	if strings.TrimSpace(diffContent) == "" {
		fmt.Println("No changes to review.")
		return nil
	}

	// Parse diff
	files, _, err := gitdiff.Parse(strings.NewReader(diffContent))
	if err != nil {
		return fmt.Errorf("parse git diff: %w", err)
	}

	if len(files) == 0 {
		fmt.Println("No files changed.")
		return nil
	}

	// Check delta availability (warn if not present)
	if err := diff.CheckDeltaAvailable(); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: %v\n", err)
		fmt.Fprintf(os.Stderr, "Syntax highlighting will not be available.\n\n")
	}

	// Build review context string
	reviewContext := cmd.buildReviewContext(ctx, cwd, diffOpts)

	// Create diff model
	model := diff.NewWithContext(files, cmd.app.Config, reviewContext)

	// Get terminal size and set model size
	// BubbleTea will handle this automatically, but we can set initial size
	model.SetSize(120, 40) // Default size, will be updated by BubbleTea

	// Run TUI
	p := tea.NewProgram(model)
	if _, err := p.Run(); err != nil {
		return fmt.Errorf("run TUI: %w", err)
	}

	return nil
}

// buildReviewContext creates a descriptive title for the review session.
func (cmd *ReviewCmd) buildReviewContext(ctx context.Context, cwd string, opts git.DiffOptions) string {
	// If user provided a name, use it
	if cmd.diffName != "" {
		return cmd.diffName
	}

	// Build context based on diff mode
	exec := &executil.RealExecutor{}
	gitExec := git.NewExecutor(cmd.app.Config.GitPath, exec)

	switch opts.Mode {
	case git.DiffBranch:
		// Get current branch name
		currentBranch, err := gitExec.Branch(ctx, cwd)
		if err == nil && currentBranch != "" {
			return fmt.Sprintf("%s..%s", opts.BaseBranch, currentBranch)
		}
		return fmt.Sprintf("Changes vs %s", opts.BaseBranch)

	case git.DiffStaged:
		return "Staged Changes"

	case git.DiffUncommitted:
		// Get current branch name
		currentBranch, err := gitExec.Branch(ctx, cwd)
		if err == nil && currentBranch != "" {
			return fmt.Sprintf("Uncommitted (%s)", currentBranch)
		}
		return "Uncommitted Changes"

	default:
		return "Diff Review"
	}
}
