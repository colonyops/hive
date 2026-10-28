package commands

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/colonyops/hive/cmd/hive/internal/app"
	"github.com/colonyops/hive/internal/domain/multiplexer"
	"github.com/colonyops/hive/internal/domain/session"
	sessionsvc "github.com/colonyops/hive/internal/hive/session"
	"github.com/urfave/cli/v3"
	"golang.org/x/term"
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
func createSessionFromFlags(ctx context.Context, app *app.App, name string, f *createSessionFlags, progress io.Writer) (*session.Session, error) {
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

	opts := sessionsvc.CreateOptions{
		Name:          name,
		Remote:        f.remote,
		Source:        source,
		Background:    f.background,
		CloneStrategy: f.cloneStrategy,
		AgentKey:      f.agent,
		Tags:          f.tags,
		Progress:      progress,
	}
	if err := resolveCreateLocation(ctx, app, &opts); err != nil {
		return nil, err
	}
	sess, err := app.SessionAPI().CreateSession(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("create session: %w", err)
	}
	if !f.background && canAttach() {
		if err := app.Sessions().AttachOrSwitch(ctx, sessionsvc.Target(*sess), multiplexer.AttachStreams{}); err != nil {
			return sess, fmt.Errorf("session %s was created, but attaching to it failed: %w", sess.Name, err)
		}
	}
	return sess, nil
}

// canAttach reports whether a tmux client can show the new session: inside
// tmux the CLI switches the client, and outside it attaches on the terminal.
// A script with neither gets the session created and left in the background.
func canAttach() bool {
	return os.Getenv("TMUX") != "" || term.IsTerminal(int(os.Stdin.Fd())) //nolint:gosec // a file descriptor fits in int
}

// resolveCreateLocation fills in what depends on where the CLI runs, because
// the session API may run in a daemon with another working directory.
func resolveCreateLocation(ctx context.Context, app *app.App, opts *sessionsvc.CreateOptions) error {
	if opts.Remote == "" {
		remote, err := app.Sessions().DetectRemote(ctx, ".")
		if err != nil {
			return fmt.Errorf("detect remote: %w", err)
		}
		opts.Remote = remote
	}
	if opts.Source != "" {
		source, err := filepath.Abs(opts.Source)
		if err != nil {
			return fmt.Errorf("resolve source directory: %w", err)
		}
		opts.Source = source
	}
	return nil
}

type NewCmd struct {
	flags       *Flags
	app         *app.App
	createFlags createSessionFlags
}

// NewNewCmd creates a new new command
func NewNewCmd(flags *Flags, app *app.App) *NewCmd {
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
