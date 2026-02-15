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

type LsCmd struct {
	flags *Flags
	app   *hive.App

	// flags
	jsonOutput bool
}

// NewLsCmd creates a new ls command
func NewLsCmd(flags *Flags, app *hive.App) *LsCmd {
	return &LsCmd{flags: flags, app: app}
}

// Register adds the ls command to the application
func (cmd *LsCmd) Register(app *cli.Command) *cli.Command {
	app.Commands = append(app.Commands, &cli.Command{
		Name:      "ls",
		Usage:     "List all sessions",
		UsageText: "hive ls [--json]",
		Description: `Displays a table of all sessions with their repo, name, state, and path.

Use --json for LLM-friendly output with additional fields like inbox topic and unread count.`,
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:        "json",
				Usage:       "output as JSON lines with inbox info",
				Destination: &cmd.jsonOutput,
			},
		},
		Action: cmd.run,
	})

	return app
}

func (cmd *LsCmd) run(ctx context.Context, c *cli.Command) error {
	sessions, err := cmd.app.Sessions.ListSessions(ctx)
	if err != nil {
		return fmt.Errorf("list sessions: %w", err)
	}

	if len(sessions) == 0 {
		if !cmd.jsonOutput {
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
	if cmd.jsonOutput {
		for _, s := range normal {
			info := cmd.buildSessionInfo(ctx, s)
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

// sessionInfo is the JSON output format for hive ls --json.
type sessionInfo struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Repo   string `json:"repo"`
	Inbox  string `json:"inbox"`
	State  string `json:"state"`
	Unread int    `json:"unread"`
}

func (cmd *LsCmd) buildSessionInfo(ctx context.Context, s session.Session) sessionInfo {
	info := sessionInfo{
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
