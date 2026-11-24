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
	"github.com/colonyops/hive/internal/ops"
	"github.com/colonyops/hive/internal/ops/sessions"
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
			Usage:       "tags to attach to the session (repeatable)",
			Destination: &f.tags,
		},
		serverFlag(&f.server),
	}
}

// createSessionFromFlags creates the session hive new names, then attaches
// to it unless --background is set.
func createSessionFromFlags(ctx context.Context, a *app.App, name string, f *createSessionFlags, progress io.Writer) (*sessions.SessionOut, error) {
	args := map[string]any{"name": name, "background": f.background, "server": a.DefaultServer()}
	for key, value := range map[string]string{"remote": f.remote, "source": f.source, "cloneStrategy": f.cloneStrategy, "agent": f.agent} {
		if value != "" {
			args[key] = value
		}
	}
	if len(f.tags) > 0 {
		args["tags"] = f.tags
	}
	created, err := createSession(ctx, a, args, progress)
	if err != nil {
		return nil, err
	}
	if f.background {
		return &created, nil
	}
	return &created, attachCreated(ctx, a, created, progress)
}

// createSession runs session.create with the CLI's defaults: the working
// directory is the caller, never HIVE_AGENT_SESSION, and a create on this
// machine copies from it.
func createSession(ctx context.Context, a *app.App, args map[string]any, progress io.Writer) (sessions.SessionOut, error) {
	wd, err := os.Getwd()
	if err != nil {
		return sessions.SessionOut{}, fmt.Errorf("determine the current directory: %w", err)
	}
	if _, ok := args[ops.CallerField]; !ok {
		args[ops.CallerField] = wd
	}
	if server, _ := args["server"].(string); server == "" || server == session.LocalServer {
		source, _ := args["source"].(string)
		if source == "" {
			source = wd
		}
		abs, err := filepath.Abs(source)
		if err != nil {
			return sessions.SessionOut{}, fmt.Errorf("resolve source directory: %w", err)
		}
		args["source"] = abs
	}
	out, err := callEngine(ctx, a, sessions.SessionCreate.Name, args, progress)
	if err != nil {
		return sessions.SessionOut{}, err
	}
	return out.(sessions.SessionOut), nil
}

// attachCreated shows a new session: it switches or attaches this terminal
// to it, or, with no terminal for a server's session, says how to attach.
func attachCreated(ctx context.Context, a *app.App, created sessions.SessionOut, progress io.Writer) error {
	onServer := created.Server != session.LocalServer
	if !onServer && !canAttach() {
		return nil
	}
	if onServer && !canAttach() {
		showRemoteAttach(created, progress)
		return nil
	}
	api, err := a.ServerAPI(created.Server)
	if err != nil {
		return err
	}
	sess, err := api.GetSession(ctx, created.ID)
	if err != nil {
		return fmt.Errorf("session %s was created, but reading it back failed: %w", created.Name, err)
	}
	if onServer {
		err = a.AttachServerSession(ctx, created.Server, sess)
	} else {
		err = a.Sessions().AttachOrSwitch(ctx, sessionsvc.Target(sess), multiplexer.AttachStreams{})
	}
	if err != nil {
		return fmt.Errorf("session %s was created, but attaching to it failed: %w", sess.Name, err)
	}
	return nil
}

// showRemoteAttach prints how to reach a new session on a server when there
// is no terminal to attach.
func showRemoteAttach(created sessions.SessionOut, progress io.Writer) {
	if progress == nil {
		progress = os.Stderr
	}
	ref := session.Ref{Server: created.Server, ID: created.ID}
	_, _ = fmt.Fprintf(progress, "Attach with: hive session attach %s\n", ref)
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
		return invalidf("session name required\n\nUsage: hive new <name...>\n\nExample: hive new Fix Auth Bug")
	}
	name := strings.Join(args, " ")

	sess, err := createSessionFromFlags(ctx, cmd.app, name, &cmd.createFlags, c.Root().ErrWriter)
	if err != nil {
		return err
	}

	_, _ = fmt.Fprintf(c.Root().ErrWriter, "Session created\n  %s\n", sess.Path)
	return nil
}
