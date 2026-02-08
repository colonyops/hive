package commands

import (
	"context"
	"fmt"

	"github.com/hay-kot/hive/internal/hive"
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
	// TODO: Implement diff TUI launch
	// 1. Get git diff using appropriate mode (uncommitted, staged, or branch comparison)
	// 2. Verify delta is available
	// 3. Launch TUI with diff viewer
	return fmt.Errorf("not implemented yet")
}
