package commands

import (
	"context"
	"fmt"
	"os"
	"slices"
	"strings"
	"text/tabwriter"

	"github.com/colonyops/hive/internal/core/git"
	"github.com/colonyops/hive/internal/core/session"
	"github.com/colonyops/hive/internal/hive"
	"github.com/colonyops/hive/pkg/iojson"
	"github.com/urfave/cli/v3"
)

type SessionCmd struct {
	flags *Flags
	app   *hive.App

	// per-subcommand flags
	infoJSON bool
	lsJSON   bool
}

// NewSessionCmd creates a new session command
func NewSessionCmd(flags *Flags, app *hive.App) *SessionCmd {
	return &SessionCmd{flags: flags, app: app}
}

// Register adds the session command group and a top-level "ls" alias.
func (cmd *SessionCmd) Register(app *cli.Command) *cli.Command {
	lsCommand := cmd.lsCmd()

	app.Commands = append(app.Commands,
		&cli.Command{
			Name:  "session",
			Usage: "Session management commands",
			Description: `Commands for managing and inspecting hive sessions.

Use 'hive session ls' to list all sessions.
Use 'hive session info' to get details about the current session.`,
			Commands: []*cli.Command{
				lsCommand,
				cmd.infoCmd(),
			},
		},
		// Top-level alias: "hive ls" -> "hive session ls"
		&cli.Command{
			Name:      "ls",
			Usage:     "List all sessions (alias for 'session ls')",
			UsageText: "hive ls [--json]",
			Hidden:    true,
			Flags:     lsCommand.Flags,
			Action:    lsCommand.Action,
		},
	)

	return app
}

func (cmd *SessionCmd) lsCmd() *cli.Command {
	return &cli.Command{
		Name:      "ls",
		Usage:     "List all sessions",
		UsageText: "hive session ls [--json]",
		Description: `Displays a table of all sessions with their repo, name, state, and path.

Use --json for LLM-friendly output with additional fields like inbox topic and unread count.`,
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:        "json",
				Usage:       "output as JSON lines with inbox info",
				Destination: &cmd.lsJSON,
			},
		},
		Action: cmd.runLs,
	}
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
		if cmd.infoJSON {
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

	if cmd.infoJSON {
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

// lsSessionInfo is the JSON output format for hive session ls --json.
type lsSessionInfo struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Repo   string `json:"repo"`
	Inbox  string `json:"inbox"`
	State  string `json:"state"`
	Unread int    `json:"unread"`
}

func (cmd *SessionCmd) runLs(ctx context.Context, c *cli.Command) error {
	sessions, err := cmd.app.Sessions.ListSessions(ctx)
	if err != nil {
		return fmt.Errorf("list sessions: %w", err)
	}

	if len(sessions) == 0 {
		if !cmd.lsJSON {
			fmt.Fprintf(os.Stderr, "No sessions found\n")
		}
		return nil
	}

	// Separate normal and corrupted sessions
	var normal, corrupted []session.Session
	for _, s := range sessions {
		if s.State == session.StateCorrupted {
			corrupted = append(corrupted, s)
		} else {
			normal = append(normal, s)
		}
	}

	// Sort by repository name
	slices.SortFunc(normal, func(a, b session.Session) int {
		return strings.Compare(git.ExtractRepoName(a.Remote), git.ExtractRepoName(b.Remote))
	})

	out := c.Root().Writer

	// JSON output mode
	if cmd.lsJSON {
		for _, s := range normal {
			info := cmd.buildLsSessionInfo(ctx, s)
			if err := iojson.WriteLine(out, info); err != nil {
				return fmt.Errorf("encode session: %w", err)
			}
		}
		return nil
	}

	// Table output mode
	if len(normal) > 0 {
		w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
		_, _ = fmt.Fprintln(w, "REPO\tNAME\tSTATE\tPATH")

		for _, s := range normal {
			repo := git.ExtractRepoName(s.Remote)
			_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", repo, s.Name, s.State, s.Path)
		}

		_ = w.Flush()
	}

	if len(corrupted) > 0 {
		_, _ = fmt.Fprintln(out)
		fmt.Fprintf(os.Stderr, "Found %d corrupted session(s) with invalid git repositories:\n", len(corrupted))
		for _, s := range corrupted {
			repo := git.ExtractRepoName(s.Remote)
			fmt.Fprintf(os.Stderr, "  %s (%s)\n", repo, s.Path)
		}
		_, _ = fmt.Fprintln(out)
		fmt.Fprintf(os.Stderr, "Run 'hive prune' to clean up\n")
	}

	return nil
}

func (cmd *SessionCmd) buildLsSessionInfo(ctx context.Context, s session.Session) lsSessionInfo {
	info := lsSessionInfo{
		ID:     s.ID,
		Name:   s.Name,
		Repo:   git.ExtractRepoName(s.Remote),
		Inbox:  s.InboxTopic(),
		State:  string(s.State),
		Unread: 0,
	}

	// Count unread inbox messages
	if msgs, err := cmd.app.Messages.GetUnread(ctx, s.ID, s.InboxTopic()); err == nil {
		info.Unread = len(msgs)
	}

	return info
}
