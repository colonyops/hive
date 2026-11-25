package commands

import (
	"context"
	"fmt"
	"os"

	"github.com/urfave/cli/v3"

	"github.com/colonyops/hive/cmd/hive/internal/app"
	"github.com/colonyops/hive/internal/domain/multiplexer"
	"github.com/colonyops/hive/internal/domain/session"
	sessionsvc "github.com/colonyops/hive/internal/hive/session"
	"github.com/colonyops/hive/internal/hive/sessionapi"
	"github.com/colonyops/hive/internal/ops/sessions"
	"github.com/colonyops/hive/pkg/iojson"
)

// SessionCmd registers the hive session group with the commands that no
// operation can do: info reads the working directory's session, and attach
// takes over the terminal. EngineCmd adds the generated ones.
type SessionCmd struct {
	flags *Flags
	app   *app.App

	server   string
	infoJSON bool
}

func NewSessionCmd(flags *Flags, app *app.App) *SessionCmd {
	return &SessionCmd{flags: flags, app: app}
}

func (cmd *SessionCmd) Register(app *cli.Command) *cli.Command {
	app.Commands = append(app.Commands, &cli.Command{
		Name:  "session",
		Usage: "Session management commands",
		Description: `Commands for managing and inspecting hive sessions on this machine and on
every server under servers: in config.yaml.

A session argument is a bare id on --server (default local) or <server>:<id>.
Use 'hive session list' to list all sessions.
Use 'hive session info' to get details about the current session.`,
		Commands: []*cli.Command{cmd.infoCmd(), cmd.attachCmd()},
	})
	return app
}

func (cmd *SessionCmd) infoCmd() *cli.Command {
	return &cli.Command{
		Name:  "info",
		Usage: "Show current session information",
		Description: `Displays information about the current hive session based on the working directory.

This command is useful for LLMs to discover their session ID and inbox topic.

Example output (--json):
  {"id":"abc123","name":"Fix Auth Bug","inbox":"agent.abc123.inbox",...}`,
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:        "json",
				Usage:       "output as JSON (recommended for LLMs)",
				Destination: &cmd.infoJSON,
			},
		},
		Action: cmd.runInfo,
	}
}

func (cmd *SessionCmd) runInfo(ctx context.Context, c *cli.Command) error {
	caller, err := requireCallingSession(ctx, cmd.app, "",
		"not in a hive session; run this command from within a hive session directory")
	if err != nil {
		return err
	}
	out, err := callEngine(ctx, cmd.app, sessions.SessionGet.Name, map[string]any{"sessionId": caller.ID}, c.Root().ErrWriter)
	if err != nil {
		return err
	}
	sess := out.(sessions.SessionOut)
	if cmd.infoJSON {
		return iojson.WriteLine(c.Root().Writer, sess)
	}
	printSessionHuman(c.Root().Writer, sess)
	return nil
}

func (cmd *SessionCmd) attachCmd() *cli.Command {
	return &cli.Command{
		Name:      "attach",
		Usage:     "Open a session's terminal",
		UsageText: "hive session attach <[server:]id>",
		Description: `Attaches this terminal to the session's tmux session. When none runs, it
starts one first, with the agent.

For a session on a server, the terminal runs over ssh. Inside tmux it opens in
a new window, so you keep the current one.`,
		Flags:  []cli.Flag{serverFlag(&cmd.server)},
		Before: targetServer(cmd.app, &cmd.server),
		Action: cmd.runAttach,
	}
}

func (cmd *SessionCmd) runAttach(ctx context.Context, c *cli.Command) error {
	ref, api, err := cmd.resolve(c.Args().First())
	if err != nil {
		return err
	}
	sess, err := api.GetSession(ctx, ref.ID)
	if err != nil {
		return fmt.Errorf("get session: %w", err)
	}
	if ref.Server != session.LocalServer {
		return cmd.app.AttachServerSession(ctx, ref.Server, sess)
	}
	result, err := api.StartTmuxSession(ctx, sess.ID)
	if err != nil {
		return fmt.Errorf("start session %s: %w", sess.Name, err)
	}
	if result.Completed {
		_, _ = fmt.Fprintf(c.Root().ErrWriter, "session %s: %v\n", sess.Name, app.ErrSessionCompleted)
		return nil
	}
	streams := multiplexer.AttachStreams{Stdin: os.Stdin, Stdout: os.Stdout, Stderr: os.Stderr}
	return cmd.app.Sessions().AttachOrSwitch(ctx, sessionsvc.Target(sess), streams)
}

// resolve reads a session argument, "<server>:<id>" or a bare id on the
// default server, and finds that server's session API.
func (cmd *SessionCmd) resolve(arg string) (session.Ref, sessionapi.API, error) {
	if arg == "" {
		return session.Ref{}, nil, invalidf("session ID required")
	}
	ref, err := session.ParseRef(arg, cmd.app.DefaultServer())
	if err != nil {
		return session.Ref{}, nil, err
	}
	api, err := cmd.app.ServerAPI(ref.Server)
	if err != nil {
		return session.Ref{}, nil, err
	}
	return ref, api, nil
}
