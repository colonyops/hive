package commands

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/colonyops/hive/internal/hive"
	"github.com/urfave/cli/v3"
)

type NewCmd struct {
	flags  *Flags
	app    *hive.App
	remote string
	source string
	agent  string
}

// NewNewCmd creates a new new command
func NewNewCmd(flags *Flags, app *hive.App) *NewCmd {
	return &NewCmd{flags: flags, app: app}
}

// Register adds the new command to the application
func (cmd *NewCmd) Register(app *cli.Command) *cli.Command {
	app.Commands = append(app.Commands, &cli.Command{
		Name:      "new",
		Usage:     "Create a new agent session",
		UsageText: "hive new <name...>",
		Description: `Creates a new isolated git environment for an AI agent session.

If a recyclable session exists for the same remote, it will be reused
(reset, checkout main, pull). Otherwise, a fresh clone is created.

After setup, any matching hooks are executed and the configured spawn
command launches a terminal with the AI tool.

Example:
  hive new Fix Auth Bug
  hive new bugfix --source /some/path`,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "agent",
				Aliases:     []string{"a"},
				Usage:       "agent profile to use (from config agents section)",
				Destination: &cmd.agent,
			},
			&cli.StringFlag{
				Name:        "remote",
				Aliases:     []string{"r"},
				Usage:       "git remote URL (defaults to current directory's origin)",
				Destination: &cmd.remote,
			},
			&cli.StringFlag{
				Name:        "source",
				Aliases:     []string{"s"},
				Usage:       "source directory for file copying (defaults to current directory)",
				Destination: &cmd.source,
			},
		},
		Action: cmd.run,
	})

	return app
}

func (cmd *NewCmd) run(ctx context.Context, c *cli.Command) error {
	args := c.Args().Slice()
	if len(args) == 0 {
		return fmt.Errorf("session name required\n\nUsage: hive new <name...>\n\nExample: hive new Fix Auth Bug")
	}
	name := strings.Join(args, " ")

	source := cmd.source
	if source == "" {
		var err error
		source, err = os.Getwd()
		if err != nil {
			return fmt.Errorf("determine source directory: %w", err)
		}
	}

	if cmd.agent != "" {
		if _, ok := cmd.app.Config.Agents.Profiles[cmd.agent]; !ok {
			return fmt.Errorf("unknown agent profile %q (available: %s)", cmd.agent, strings.Join(agentProfileNames(cmd.app.Config), ", "))
		}
	}

	opts := hive.CreateOptions{
		Name:   name,
		Remote: cmd.remote,
		Source: source,
		Agent:  cmd.agent,
	}

	sess, err := cmd.app.Sessions.CreateSession(ctx, opts)
	if err != nil {
		return fmt.Errorf("create session: %w", err)
	}

	fmt.Fprintf(os.Stderr, "Session created\n  %s\n", sess.Path)
	return nil
}
