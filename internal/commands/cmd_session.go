package commands

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/hay-kot/hive/internal/core/git"
	"github.com/hay-kot/hive/internal/core/messaging"
	"github.com/hay-kot/hive/internal/printer"
	"github.com/urfave/cli/v3"
)

type SessionCmd struct {
	flags *Flags

	// flags
	jsonOutput bool
}

// NewSessionCmd creates a new session command
func NewSessionCmd(flags *Flags) *SessionCmd {
	return &SessionCmd{flags: flags}
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
	p := printer.Ctx(ctx)

	// Detect session from current working directory
	detector := messaging.NewSessionDetector(cmd.flags.Store)
	sessionID, err := detector.DetectSession(ctx)
	if err != nil {
		return fmt.Errorf("detect session: %w", err)
	}

	if sessionID == "" {
		if cmd.jsonOutput {
			_, _ = fmt.Fprintln(c.Root().Writer, "{\"error\":\"not in a hive session\"}")
			return nil
		}
		p.Warnf("Not in a hive session")
		p.Infof("Run this command from within a hive session directory")
		return nil
	}

	// Get full session details
	sess, err := cmd.flags.Service.GetSession(ctx, sessionID)
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
		enc := json.NewEncoder(out)
		return enc.Encode(info)
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
