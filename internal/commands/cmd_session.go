package commands

import (
	"context"
	"fmt"
	"os"

	"github.com/hay-kot/hive/internal/core/git"
	"github.com/hay-kot/hive/internal/hive"
	"github.com/hay-kot/hive/pkg/iojson"
	"github.com/urfave/cli/v3"
)

type SessionCmd struct {
	flags *Flags
	app   *hive.App

	// flags
	jsonOutput bool
}

// NewSessionCmd creates a new session command
func NewSessionCmd(flags *Flags, app *hive.App) *SessionCmd {
	return &SessionCmd{flags: flags, app: app}
}

// Register adds the session command to the application
func (cmd *SessionCmd) Register(app *cli.Command) *cli.Command {
	app.Commands = append(app.Commands, &cli.Command{
		Name:  "session",
		Usage: "Session management commands",
		Description: `Commands for managing and inspecting hive sessions.

Use 'hive session info' to get details about the current session.`,
		Commands: []*cli.Command{
			cmd.infoCmd(),
		},
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
				Destination: &cmd.jsonOutput,
			},
		},
		Action: cmd.runInfo,
	}
}

// sessionInfoOutput is the JSON output format for hive session info.
type sessionInfoOutput struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Repo   string `json:"repo"`
	Remote string `json:"remote"`
	Path   string `json:"path"`
	Inbox  string `json:"inbox"`
	State  string `json:"state"`
}

func (cmd *SessionCmd) runInfo(ctx context.Context, c *cli.Command) error {
	// Detect session from current working directory
	sessionID, err := cmd.app.Sessions.DetectSession(ctx)
	if err != nil {
		return fmt.Errorf("detect session: %w", err)
	}

	if sessionID == "" {
		if cmd.jsonOutput {
			_, _ = fmt.Fprintln(c.Root().Writer, "{\"error\":\"not in a hive session\"}")
			return nil
		}
		fmt.Fprintf(os.Stderr, "Not in a hive session\nRun this command from within a hive session directory\n")
		return nil
	}

	// Get full session details
	sess, err := cmd.app.Sessions.GetSession(ctx, sessionID)
	if err != nil {
		return fmt.Errorf("get session: %w", err)
	}

	out := c.Root().Writer

	if cmd.jsonOutput {
		info := sessionInfoOutput{
			ID:     sess.ID,
			Name:   sess.Name,
			Repo:   git.ExtractRepoName(sess.Remote),
			Remote: sess.Remote,
			Path:   sess.Path,
			Inbox:  sess.InboxTopic(),
			State:  string(sess.State),
		}
		return iojson.WriteLine(out, info)
	}

	// Human-readable output
	_, _ = fmt.Fprintf(out, "Session ID:  %s\n", sess.ID)
	_, _ = fmt.Fprintf(out, "Name:        %s\n", sess.Name)
	_, _ = fmt.Fprintf(out, "Repository:  %s\n", git.ExtractRepoName(sess.Remote))
	_, _ = fmt.Fprintf(out, "Inbox:       %s\n", sess.InboxTopic())
	_, _ = fmt.Fprintf(out, "Path:        %s\n", sess.Path)
	_, _ = fmt.Fprintf(out, "State:       %s\n", sess.State)

	return nil
}
