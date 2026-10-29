package commands

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
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
	server        string
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
		serverFlag(&f.server),
	}
}

// createSessionFromFlags validates the shared create flags and creates the
// session. Progress may be nil (service output then goes to the service's
// default writers).
func createSessionFromFlags(ctx context.Context, app *app.App, name string, f *createSessionFlags, progress io.Writer) (*session.Session, error) {
	if err := checkAgent(ctx, app, f.agent); err != nil {
		return nil, err
	}

	// A server cannot read a path on this machine, so a create there gets no
	// source and its copy: rules copy nothing.
	source := f.source
	switch {
	case source != "" && app.TargetsServer():
		return nil, fmt.Errorf("--source names a directory on this machine, which the server cannot read")
	case source == "" && !app.TargetsServer():
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
	if app.TargetsServer() && f.remote == "" && isLocalPath(opts.Remote) {
		return nil, fmt.Errorf("this checkout's remote %q is a path on this machine, which the server cannot clone; pass --remote with a URL the server can reach", opts.Remote)
	}
	sess, err := app.SessionAPI().CreateSession(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("create session: %w", err)
	}
	switch {
	case f.background:
	case app.TargetsServer() && canAttach():
		if err := app.AttachServerSession(ctx, app.DefaultServer(), *sess); err != nil {
			return sess, fmt.Errorf("session %s was created, but attaching to it failed: %w", sess.Name, err)
		}
	case app.TargetsServer():
		showRemoteAttach(app, *sess, progress)
	case canAttach():
		if err := app.Sessions().AttachOrSwitch(ctx, sessionsvc.Target(*sess), multiplexer.AttachStreams{}); err != nil {
			return sess, fmt.Errorf("session %s was created, but attaching to it failed: %w", sess.Name, err)
		}
	}
	return sess, nil
}

// checkAgent refuses an agent profile that the machine which spawns the
// session does not have.
func checkAgent(ctx context.Context, app *app.App, agent string) error {
	if agent == "" {
		return nil
	}
	if !app.TargetsServer() {
		if _, ok := app.Config.Agents.Profiles[agent]; !ok {
			return fmt.Errorf("unknown agent %q", agent)
		}
		return nil
	}
	options, err := app.SessionAPI().SessionLaunchOptions(ctx)
	if err != nil {
		return fmt.Errorf("read the agent profiles of server %s: %w", app.DefaultServer(), err)
	}
	if !slices.Contains(options.Agents, agent) {
		return fmt.Errorf("server %s has no agent %q; its agents are %s", app.DefaultServer(), agent, strings.Join(options.Agents, ", "))
	}
	return nil
}

// showRemoteAttach prints how to reach a new session on a server when there
// is no terminal to attach.
func showRemoteAttach(app *app.App, sess session.Session, progress io.Writer) {
	if progress == nil {
		progress = os.Stderr
	}
	ref := session.Ref{Server: app.DefaultServer(), ID: sess.ID}
	_, _ = fmt.Fprintf(progress, "Attach with: hive session attach %s\n", ref)
}

// isLocalPath reports whether a git remote names a path on this machine
// instead of a URL another machine can clone.
func isLocalPath(remote string) bool {
	return strings.HasPrefix(remote, "/") || strings.HasPrefix(remote, ".") ||
		strings.HasPrefix(remote, "~") || strings.HasPrefix(remote, "file://")
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
  hive new bugfix --source /some/path
  hive new --server box --remote git@github.com:acme/site.git Fix Auth Bug`,
		Flags:  sessionCreateFlags(&cmd.createFlags),
		Before: targetServer(cmd.app, &cmd.createFlags.server),
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
