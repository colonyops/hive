package commands

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/colonyops/hive/internal/core/session"
	"github.com/colonyops/hive/internal/hive"
	"github.com/urfave/cli/v3"
)

// createSessionFlags holds the flag values shared by 'hive new' and
// 'hive session create'.
type createSessionFlags struct {
	remote        string
	source        string
	background    bool
	cloneStrategy string
	agent         string
	tags          []string
}

// sessionCreateFlags returns the flag set shared by 'hive new' and
// 'hive session create', bound to f.
func sessionCreateFlags(f *createSessionFlags) []cli.Flag {
	return []cli.Flag{
		&cli.StringFlag{
			Name:        "remote",
			Aliases:     []string{"r"},
			Usage:       "git remote URL (defaults to current directory's origin)",
			Destination: &f.remote,
		},
		&cli.StringFlag{
			Name:        "source",
			Aliases:     []string{"s"},
			Usage:       "source directory for file copying (defaults to current directory)",
			Destination: &f.source,
		},
		&cli.BoolFlag{
			Name:        "background",
			Aliases:     []string{"bg"},
			Usage:       "create session without attaching to tmux",
			Destination: &f.background,
		},
		&cli.StringFlag{
			Name:        "clone-strategy",
			Usage:       "clone strategy: full or worktree",
			Destination: &f.cloneStrategy,
		},
		&cli.StringFlag{
			Name:        "agent",
			Aliases:     []string{"a"},
			Usage:       "agent profile key from agents config",
			Destination: &f.agent,
		},
		&cli.StringSliceFlag{
			Name:        "tags",
			Aliases:     []string{"t"},
			Usage:       "tags to attach to the session (repeatable)",
			Destination: &f.tags,
		},
	}
}

// createSessionFromFlags validates the shared create flags and creates the
// session. Progress may be nil (service output then goes to the service's
// default writers).
func createSessionFromFlags(ctx context.Context, app *hive.App, name string, f *createSessionFlags, progress io.Writer) (*session.Session, error) {
	if f.agent != "" {
		if _, ok := app.Config.Agents.Profiles[f.agent]; !ok {
			return nil, fmt.Errorf("unknown agent %q", f.agent)
		}
	}

	source := f.source
	if source == "" {
		var err error
		source, err = os.Getwd()
		if err != nil {
			return nil, fmt.Errorf("determine source directory: %w", err)
		}
	}

	sess, err := app.Sessions.CreateSession(ctx, hive.CreateOptions{
		Name:          name,
		Remote:        f.remote,
		Source:        source,
		Background:    f.background,
		CloneStrategy: f.cloneStrategy,
		AgentKey:      f.agent,
		Tags:          f.tags,
		Progress:      progress,
	})
	if err != nil {
		return nil, fmt.Errorf("create session: %w", err)
	}
	return sess, nil
}

type NewCmd struct {
	flags       *Flags
	app         *hive.App
	createFlags createSessionFlags
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
  hive new --agent claude Refactor Utils
  hive new bugfix --source /some/path`,
		Flags:  sessionCreateFlags(&cmd.createFlags),
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

	sess, err := createSessionFromFlags(ctx, cmd.app, name, &cmd.createFlags, nil)
	if err != nil {
		return err
	}

	fmt.Fprintf(os.Stderr, "Session created\n  %s\n", sess.Path)
	return nil
}
