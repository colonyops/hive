package commands

import (
	"context"
	"fmt"
	"os"

	"github.com/colonyops/hive/internal/hive"
	claudeplugin "github.com/colonyops/hive/internal/hive/plugins/claude"
	"github.com/urfave/cli/v3"
)

// ClaudeCmd implements `hive claude` subcommands.
type ClaudeCmd struct {
	flags *Flags
	app   *hive.App
}

// NewClaudeCmd creates a new claude command.
func NewClaudeCmd(flags *Flags, app *hive.App) *ClaudeCmd {
	return &ClaudeCmd{flags: flags, app: app}
}

// Register adds the claude command tree to the CLI application.
func (cmd *ClaudeCmd) Register(app *cli.Command) *cli.Command {
	app.Commands = append(app.Commands, &cli.Command{
		Name:  "claude",
		Usage: "Claude Code integration commands",
		Commands: []*cli.Command{
			cmd.hooksCmd(),
		},
	})
	return app
}

func (cmd *ClaudeCmd) hooksCmd() *cli.Command {
	return &cli.Command{
		Name:  "hooks",
		Usage: "Manage Claude Code hook integrations",
		Commands: []*cli.Command{
			cmd.hooksInstallCmd(),
		},
	}
}

func (cmd *ClaudeCmd) hooksInstallCmd() *cli.Command {
	var path string
	return &cli.Command{
		Name:  "install",
		Usage: "Install hive status hooks into a session's Claude Code settings",
		Description: `Writes hook entries into <path>/.claude/settings.json so that Claude Code
reports agent status (active / ready) to hive's terminal integration.

Existing hooks and settings are preserved — the command is idempotent.

Example:
  hive claude hooks install
  hive claude hooks install --path /path/to/session`,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "path",
				Aliases:     []string{"p"},
				Usage:       "session directory path (defaults to current directory)",
				Destination: &path,
			},
		},
		Action: func(ctx context.Context, c *cli.Command) error {
			sessionPath := path
			if sessionPath == "" {
				var err error
				sessionPath, err = os.Getwd()
				if err != nil {
					return fmt.Errorf("determine working directory: %w", err)
				}
			}

			if err := claudeplugin.InstallHooks(sessionPath); err != nil {
				return fmt.Errorf("install Claude hooks: %w", err)
			}

			_, _ = fmt.Fprintf(c.Root().Writer, "Claude hooks installed in %s/.claude/settings.json\n", sessionPath)
			return nil
		},
	}
}
