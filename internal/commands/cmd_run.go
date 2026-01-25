package commands

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/hay-kot/hive/internal/core/history"
	"github.com/hay-kot/hive/internal/printer"
	"github.com/urfave/cli/v3"
)

type RunCmd struct {
	flags *Flags

	// Command-specific flags
	replay       string
	listHistory  bool
	clearHistory bool
}

// NewRunCmd creates a new run command
func NewRunCmd(flags *Flags) *RunCmd {
	return &RunCmd{flags: flags}
}

// Register adds the run command to the application
func (cmd *RunCmd) Register(app *cli.Command) *cli.Command {
	app.Commands = append(app.Commands, &cli.Command{
		Name:      "run",
		Usage:     "Replay commands from history",
		UsageText: "hive run [options]",
		Description: `Replay previously executed 'new' commands or manage command history.

Use --replay to re-run a command from history:
  hive run --replay         # Replay the last failed 'new' command
  hive run --replay <id>    # Replay a specific command by ID

Use --list-history to view recent 'new' commands.
Use --clear-history to remove all history.`,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "replay",
				Aliases:     []string{"r"},
				Usage:       "replay a command (last failed if no ID given)",
				Destination: &cmd.replay,
			},
			&cli.BoolFlag{
				Name:        "list-history",
				Aliases:     []string{"l"},
				Usage:       "list command history",
				Destination: &cmd.listHistory,
			},
			&cli.BoolFlag{
				Name:        "clear-history",
				Usage:       "clear all command history",
				Destination: &cmd.clearHistory,
			},
		},
		Action: cmd.run,
	})

	return app
}

func (cmd *RunCmd) run(ctx context.Context, c *cli.Command) error {
	p := printer.Ctx(ctx)

	// Handle mutually exclusive flags
	flagCount := 0
	if cmd.replay != "" || c.IsSet("replay") {
		flagCount++
	}
	if cmd.listHistory {
		flagCount++
	}
	if cmd.clearHistory {
		flagCount++
	}

	if flagCount == 0 {
		// Default to showing help if no flags provided
		return cli.ShowSubcommandHelp(c)
	}

	if flagCount > 1 {
		return fmt.Errorf("only one of --replay, --list-history, or --clear-history can be used")
	}

	if cmd.listHistory {
		return cmd.runListHistory(ctx, c)
	}

	if cmd.clearHistory {
		return cmd.runClearHistory(ctx, p)
	}

	// Handle replay
	return cmd.runReplay(ctx, c, p)
}

func (cmd *RunCmd) runListHistory(ctx context.Context, c *cli.Command) error {
	entries, err := cmd.flags.HistoryStore.List(ctx)
	if err != nil {
		return fmt.Errorf("list history: %w", err)
	}

	if len(entries) == 0 {
		printer.Ctx(ctx).Infof("No command history")
		return nil
	}

	out := c.Root().Writer
	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "ID\tCOMMAND\tSTATUS\tTIME")

	for _, e := range entries {
		status := "ok"
		if e.Failed() {
			status = fmt.Sprintf("failed (%d)", e.ExitCode)
		}

		cmdStr := e.Command
		if len(e.Args) > 0 {
			cmdStr += " " + strings.Join(e.Args, " ")
		}

		// Truncate long commands
		if len(cmdStr) > 50 {
			cmdStr = cmdStr[:47] + "..."
		}

		_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\n",
			e.ID,
			cmdStr,
			status,
			e.Timestamp.Format("2006-01-02 15:04:05"),
		)
	}

	return w.Flush()
}

func (cmd *RunCmd) runClearHistory(ctx context.Context, p *printer.Printer) error {
	if err := cmd.flags.HistoryStore.Clear(ctx); err != nil {
		return fmt.Errorf("clear history: %w", err)
	}

	p.Successf("Command history cleared")
	return nil
}

func (cmd *RunCmd) runReplay(ctx context.Context, c *cli.Command, p *printer.Printer) error {
	var entry history.Entry
	var err error

	if cmd.replay == "" {
		// Replay last failed command
		entry, err = cmd.flags.HistoryStore.LastFailed(ctx)
		if errors.Is(err, history.ErrNotFound) {
			p.Infof("No failed commands in history")
			return nil
		}
		if err != nil {
			return fmt.Errorf("get last failed: %w", err)
		}
	} else {
		// Replay specific command by ID
		entry, err = cmd.flags.HistoryStore.Get(ctx, cmd.replay)
		if errors.Is(err, history.ErrNotFound) {
			return fmt.Errorf("command %q not found in history", cmd.replay)
		}
		if err != nil {
			return fmt.Errorf("get command: %w", err)
		}
	}

	// Build args for replay
	args := []string{os.Args[0], entry.Command}
	args = append(args, entry.Args...)

	cmdStr := entry.Command
	if len(entry.Args) > 0 {
		cmdStr += " " + strings.Join(entry.Args, " ")
	}
	p.Infof("Replaying: hive %s", cmdStr)

	// Run the command through the root
	return c.Root().Run(ctx, args)
}
