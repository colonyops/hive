package commands

import (
	"context"
	"fmt"

	"github.com/urfave/cli/v3"

	"github.com/colonyops/hive/internal/core/terminal"
	terminaltmux "github.com/colonyops/hive/internal/core/terminal/tmux"
)

func (cmd *ExperimentalCmd) detectCmd() *cli.Command {
	return &cli.Command{
		Name:  "detect",
		Usage: "Diagnose agent detection across all tmux panes",
		Action: func(ctx context.Context, c *cli.Command) error {
			return cmd.runDetect(ctx, c)
		},
	}
}

func (cmd *ExperimentalCmd) runDetect(ctx context.Context, c *cli.Command) error {
	tmuxI := terminaltmux.New(cmd.app.Config.Tmux.PreviewWindowMatcher)
	if !tmuxI.Available() {
		_, _ = fmt.Fprintln(c.Writer, "tmux not available")
		return nil
	}

	panes, err := tmuxI.DiscoverAllPanes(ctx)
	if err != nil {
		return fmt.Errorf("discover panes: %w", err)
	}

	if len(panes) == 0 {
		_, _ = fmt.Fprintln(c.Writer, "no tmux panes found")
		return nil
	}

	terminal.PrintPaneTree(c.Writer, panes)
	return nil
}
