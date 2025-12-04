package commands

import (
	"context"
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/urfave/cli/v3"

	"github.com/hay-kot/hive/internal/tui"
)

type TuiCmd struct {
	flags *Flags
}

// NewTuiCmd creates a new tui command
func NewTuiCmd(flags *Flags) *TuiCmd {
	return &TuiCmd{flags: flags}
}

// Register adds the tui command to the application
func (cmd *TuiCmd) Register(app *cli.Command) *cli.Command {
	app.Commands = append(app.Commands, &cli.Command{
		Name:      "tui",
		Usage:     "Launch the interactive session manager",
		UsageText: "hive tui",
		Description: `Opens a terminal UI for managing sessions.

Navigate with arrow keys or j/k. Press r to recycle, d to delete.
Custom keybindings can be configured in the config file.

This is the default command when hive is run without arguments.`,
		Action: cmd.run,
	})

	return app
}

// Run executes the TUI. Exported for use as default command.
func (cmd *TuiCmd) Run(ctx context.Context, c *cli.Command) error {
	return cmd.run(ctx, c)
}

func (cmd *TuiCmd) run(_ context.Context, _ *cli.Command) error {
	m := tui.New(cmd.flags.Service, cmd.flags.Config)
	p := tea.NewProgram(m, tea.WithAltScreen())

	if _, err := p.Run(); err != nil {
		return fmt.Errorf("run tui: %w", err)
	}

	return nil
}
