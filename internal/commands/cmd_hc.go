package commands

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/colonyops/hive/internal/core/git"
	"github.com/colonyops/hive/internal/core/hc"
	"github.com/colonyops/hive/internal/hive"
	"github.com/colonyops/hive/pkg/iojson"
	"github.com/colonyops/hive/pkg/randid"
	"github.com/rs/zerolog/log"
	"github.com/urfave/cli/v3"
)

// HCCmd implements the hive hc command group for honeycomb task management.
type HCCmd struct {
	flags *Flags
	app   *hive.App
}

// NewHCCmd creates a new hc command.
func NewHCCmd(flags *Flags, app *hive.App) *HCCmd {
	return &HCCmd{flags: flags, app: app}
}

// Register adds the hc command group to the application.
func (cmd *HCCmd) Register(app *cli.Command) *cli.Command {
	app.Commands = append(app.Commands, &cli.Command{
		Name:  "hc",
		Usage: "Honeycomb multi-agent task coordination",
		Description: `Honeycomb (hc) provides built-in task coordination for multi-agent workflows.

Agents create epics and tasks, claim work, record checkpoints, and signal completion.
A conductor agent can bulk-create a full task tree from a JSON plan.`,
		Commands: []*cli.Command{
			cmd.createCmd(),
			cmd.listCmd(),
			cmd.showCmd(),
			cmd.updateCmd(),
			cmd.nextCmd(),
			cmd.logCmd(),
			cmd.checkpointCmd(),
			cmd.contextCmd(),
			cmd.pruneCmd(),
		},
	})
	return app
}

// detectSession returns the session ID for the current working directory.
// Errors are logged at debug level; the caller receives an empty string on failure.
func (cmd *HCCmd) detectSession(ctx context.Context) string {
	sessionID, err := cmd.app.Sessions.DetectSession(ctx)
	if err != nil {
		log.Debug().Err(err).Msg("could not detect session")
	}
	return sessionID
}

// detectRepoKey returns "owner/repo" for the current working directory.
// Returns "" if not inside a git repository or the remote cannot be parsed.
func (cmd *HCCmd) detectRepoKey(ctx context.Context) string {
	url, _ := cmd.app.Sessions.Git().RemoteURL(ctx, ".")
	owner, repo := git.ExtractOwnerRepo(url)
	if owner == "" || repo == "" {
		return ""
	}
	return owner + "/" + repo
}

func (cmd *HCCmd) createCmd() *cli.Command {
	var itemType string
	var parentID string
	var assignID string
	var desc string
	bulk := &iojson.FileReader[hc.CreateInput]{}
	return &cli.Command{
		Name:      "create",
		Usage:     "Create an hc item or bulk-create a task tree from JSON",
		ArgsUsage: "[title]",
		Flags: []cli.Flag{
			bulk.Flag(),
			&cli.StringFlag{
				Name:        "type",
				Usage:       "Item type: epic or task",
				Value:       "epic",
				Destination: &itemType,
			},
			&cli.StringFlag{
				Name:        "parent",
				Usage:       "Parent item ID",
				Destination: &parentID,
			},
			&cli.StringFlag{
				Name:        "assign",
				Usage:       "Assign to session ID",
				Destination: &assignID,
			},
			&cli.StringFlag{
				Name:        "desc",
				Usage:       "Item description",
				Destination: &desc,
			},
		},
		Action: func(ctx context.Context, c *cli.Command) error {
			sessionID := cmd.detectSession(ctx)
			repoKey := cmd.detectRepoKey(ctx)

			if assignID != "" {
				sessionID = assignID
			}

			// Bulk creation via JSON file or stdin.
			if c.IsSet("file") || c.NArg() == 0 {
				input, err := bulk.Read()
				if err != nil {
					return fmt.Errorf("read bulk input: %w", err)
				}
				items, err := cmd.app.HC.CreateBulk(ctx, input, repoKey, sessionID)
				if err != nil {
					return fmt.Errorf("bulk create hc items: %w", err)
				}
				for _, item := range items {
					if err := iojson.WriteLine(c.Root().Writer, item); err != nil {
						return err
					}
				}
				return nil
			}

			// Single item creation.
			title := c.Args().First()
			if title == "" {
				return fmt.Errorf("title is required (or use --file for bulk creation)")
			}

			typ, err := hc.ParseItemType(itemType)
			if err != nil {
				return fmt.Errorf("invalid item type %q: %w", itemType, err)
			}

			now := time.Now()
			id := "hc-" + randid.Generate(8)

			var epicID string
			var depth int

			if parentID != "" {
				parent, err := cmd.app.HC.GetItem(ctx, parentID)
				if err != nil {
					return fmt.Errorf("get parent item %q: %w", parentID, err)
				}
				if parent.IsEpic() {
					epicID = parent.ID
				} else {
					epicID = parent.EpicID
				}
				depth = parent.Depth + 1
			}

			item := hc.Item{
				ID:        id,
				RepoKey:   repoKey,
				EpicID:    epicID,
				ParentID:  parentID,
				SessionID: sessionID,
				Title:     title,
				Desc:      desc,
				Type:      typ,
				Status:    hc.StatusOpen,
				Depth:     depth,
				CreatedAt: now,
				UpdatedAt: now,
			}
			if err := cmd.app.HC.CreateItem(ctx, item); err != nil {
				return fmt.Errorf("create hc item: %w", err)
			}
			return iojson.WriteLine(c.Root().Writer, item)
		},
	}
}

func (cmd *HCCmd) listCmd() *cli.Command {
	var repo string
	var statusStr string
	var jsonOut bool
	return &cli.Command{
		Name:      "list",
		Usage:     "List hc items",
		ArgsUsage: "[epic-id]",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "repo",
				Usage:       "Filter by repo key (owner/repo)",
				Destination: &repo,
			},
			&cli.StringFlag{
				Name:        "status",
				Usage:       "Filter by status",
				Destination: &statusStr,
			},
			&cli.BoolFlag{
				Name:        "json",
				Usage:       "Output as JSON lines",
				Destination: &jsonOut,
			},
		},
		Action: func(ctx context.Context, c *cli.Command) error {
			filter := hc.ListFilter{
				RepoKey: repo,
				EpicID:  c.Args().First(),
			}
			if statusStr != "" {
				s, err := hc.ParseStatus(statusStr)
				if err != nil {
					return fmt.Errorf("invalid status %q: %w", statusStr, err)
				}
				filter.Status = &s
			}

			items, err := cmd.app.HC.ListItems(ctx, filter)
			if err != nil {
				return fmt.Errorf("list hc items: %w", err)
			}

			if jsonOut {
				for _, item := range items {
					if err := iojson.WriteLine(c.Root().Writer, item); err != nil {
						return err
					}
				}
				return nil
			}

			_, _ = fmt.Fprintf(c.Root().Writer, "%-20s\t%-6s\t%-12s\t%s\n", "ID", "TYPE", "STATUS", "TITLE")
			for _, item := range items {
				_, _ = fmt.Fprintf(c.Root().Writer, "%-20s\t%-6s\t%-12s\t%s\n",
					item.ID, item.Type, item.Status, item.Title)
			}
			return nil
		},
	}
}

func (cmd *HCCmd) showCmd() *cli.Command {
	var jsonOut bool
	return &cli.Command{
		Name:      "show",
		Usage:     "Show an hc item and its activity log",
		ArgsUsage: "<id>",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:        "json",
				Usage:       "Output as JSON lines",
				Destination: &jsonOut,
			},
		},
		Action: func(ctx context.Context, c *cli.Command) error {
			id := c.Args().First()
			if id == "" {
				return fmt.Errorf("item ID is required")
			}
			item, err := cmd.app.HC.GetItem(ctx, id)
			if err != nil {
				return fmt.Errorf("get hc item: %w", err)
			}
			activities, err := cmd.app.HC.ListActivity(ctx, id)
			if err != nil {
				return fmt.Errorf("list hc activity: %w", err)
			}

			if jsonOut {
				if err := iojson.WriteLine(c.Root().Writer, item); err != nil {
					return err
				}
				for _, a := range activities {
					if err := iojson.WriteLine(c.Root().Writer, a); err != nil {
						return err
					}
				}
				return nil
			}

			// Human-readable output.
			w := c.Root().Writer
			_, _ = fmt.Fprintf(w, "ID:      %s\n", item.ID)
			_, _ = fmt.Fprintf(w, "Type:    %s\n", item.Type)
			_, _ = fmt.Fprintf(w, "Status:  %s\n", item.Status)
			_, _ = fmt.Fprintf(w, "Title:   %s\n", item.Title)
			if item.Desc != "" {
				_, _ = fmt.Fprintf(w, "Desc:    %s\n", item.Desc)
			}
			_, _ = fmt.Fprintf(w, "EpicID:  %s\n", item.EpicID)
			_, _ = fmt.Fprintf(w, "Parent:  %s\n", item.ParentID)
			_, _ = fmt.Fprintf(w, "Session: %s\n", item.SessionID)
			_, _ = fmt.Fprintf(w, "Created: %s\n", item.CreatedAt.Format(time.RFC3339))
			_, _ = fmt.Fprintf(w, "Updated: %s\n", item.UpdatedAt.Format(time.RFC3339))
			if len(activities) > 0 {
				_, _ = fmt.Fprintf(w, "\nActivity:\n")
				for _, a := range activities {
					_, _ = fmt.Fprintf(w, "  [%s] %s: %s\n", a.CreatedAt.Format(time.RFC3339), a.Type, a.Message)
				}
			}
			return nil
		},
	}
}

func (cmd *HCCmd) updateCmd() *cli.Command {
	var statusStr string
	var assignID string
	return &cli.Command{
		Name:      "update",
		Usage:     "Update an hc item",
		ArgsUsage: "<id>",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "status",
				Usage:       "New status (open, in_progress, done, cancelled)",
				Destination: &statusStr,
			},
			&cli.StringFlag{
				Name:        "assign",
				Usage:       "Assign to session ID",
				Destination: &assignID,
			},
		},
		Action: func(ctx context.Context, c *cli.Command) error {
			id := c.Args().First()
			if id == "" {
				return fmt.Errorf("item ID is required")
			}
			if statusStr == "" && assignID == "" {
				return fmt.Errorf("at least one of --status or --assign is required")
			}
			update := hc.ItemUpdate{}
			if statusStr != "" {
				s, err := hc.ParseStatus(statusStr)
				if err != nil {
					return fmt.Errorf("invalid status %q: %w", statusStr, err)
				}
				update.Status = &s
			}
			if assignID != "" {
				update.SessionID = &assignID
			}
			item, err := cmd.app.HC.UpdateItem(ctx, id, update)
			if err != nil {
				return fmt.Errorf("update hc item: %w", err)
			}
			return iojson.WriteLine(c.Root().Writer, item)
		},
	}
}

func (cmd *HCCmd) nextCmd() *cli.Command {
	return &cli.Command{
		Name:      "next",
		Usage:     "Get the next available task for this session",
		ArgsUsage: "[epic-id]",
		Action: func(ctx context.Context, c *cli.Command) error {
			sessionID := cmd.detectSession(ctx)
			if sessionID == "" {
				return fmt.Errorf("session not detected: run inside a hive session")
			}
			filter := hc.NextFilter{
				EpicID:    c.Args().First(),
				SessionID: sessionID,
			}
			item, ok, err := cmd.app.HC.Next(ctx, filter)
			if err != nil {
				return fmt.Errorf("next hc item: %w", err)
			}
			if !ok {
				_, _ = fmt.Fprintln(c.Root().Writer, "no ready tasks")
				return nil
			}
			return iojson.WriteLine(c.Root().Writer, item)
		},
	}
}

func (cmd *HCCmd) logCmd() *cli.Command {
	var actTypeStr string
	return &cli.Command{
		Name:      "log",
		Usage:     "Log an activity entry for an hc item",
		ArgsUsage: "<id> <message>",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "type",
				Usage:       "Activity type: update or comment",
				Value:       "update",
				Destination: &actTypeStr,
			},
		},
		Action: func(ctx context.Context, c *cli.Command) error {
			id := c.Args().Get(0)
			message := c.Args().Get(1)
			if id == "" || message == "" {
				return fmt.Errorf("item ID and message are required")
			}
			actType, err := hc.ParseActivityType(actTypeStr)
			if err != nil {
				return fmt.Errorf("invalid activity type %q: %w", actTypeStr, err)
			}
			activity, err := cmd.app.HC.LogActivity(ctx, id, actType, message)
			if err != nil {
				return fmt.Errorf("log hc activity: %w", err)
			}
			return iojson.WriteLine(c.Root().Writer, activity)
		},
	}
}

func (cmd *HCCmd) checkpointCmd() *cli.Command {
	return &cli.Command{
		Name:      "checkpoint",
		Usage:     "Record a checkpoint for an hc item",
		ArgsUsage: "<id> <message>",
		Action: func(ctx context.Context, c *cli.Command) error {
			id := c.Args().Get(0)
			message := c.Args().Get(1)
			if id == "" || message == "" {
				return fmt.Errorf("item ID and message are required")
			}
			activity, err := cmd.app.HC.Checkpoint(ctx, id, message)
			if err != nil {
				return fmt.Errorf("checkpoint hc item: %w", err)
			}
			return iojson.WriteLine(c.Root().Writer, activity)
		},
	}
}

func (cmd *HCCmd) contextCmd() *cli.Command {
	var jsonOut bool
	return &cli.Command{
		Name:      "context",
		Usage:     "Get assembled context for an epic",
		ArgsUsage: "<epic-id>",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:        "json",
				Usage:       "Output as JSON",
				Destination: &jsonOut,
			},
		},
		Action: func(ctx context.Context, c *cli.Command) error {
			epicID := c.Args().First()
			if epicID == "" {
				return fmt.Errorf("epic-id is required")
			}
			sessionID := cmd.detectSession(ctx)
			block, err := cmd.app.HC.Context(ctx, epicID, sessionID)
			if err != nil {
				return fmt.Errorf("get hc context: %w", err)
			}

			if jsonOut {
				return iojson.WriteLine(c.Root().Writer, block)
			}

			w := c.Root().Writer
			_, _ = fmt.Fprintf(w, "# Honeycomb: %s [%s]\n", block.Epic.Title, block.Epic.ID)
			_, _ = fmt.Fprintf(w, "Status: %d/%d open · %d done · %d cancelled\n",
				block.Counts.Open+block.Counts.InProgress,
				block.Counts.Open+block.Counts.InProgress+block.Counts.Done+block.Counts.Cancelled,
				block.Counts.Done, block.Counts.Cancelled)

			if len(block.MyTasks) > 0 {
				_, _ = fmt.Fprintf(w, "\n## Assigned to this session\n")
				for _, t := range block.MyTasks {
					_, _ = fmt.Fprintf(w, "- [%s] %s (%s)\n", t.Item.Status, t.Item.Title, t.Item.ID)
					if t.LatestCheckpoint.Message != "" {
						_, _ = fmt.Fprintf(w, "  Last checkpoint: %s (%s)\n",
							t.LatestCheckpoint.Message,
							timeAgo(t.LatestCheckpoint.CreatedAt))
					} else {
						_, _ = fmt.Fprintf(w, "  Last checkpoint: no checkpoint\n")
					}
				}
			}

			if len(block.AllOpenTasks) > 0 {
				_, _ = fmt.Fprintf(w, "\n## All open tasks\n")
				for _, t := range block.AllOpenTasks {
					_, _ = fmt.Fprintf(w, "- [%s] %s (%s)\n", t.Status, t.Title, t.ID)
				}
			}
			return nil
		},
	}
}

func (cmd *HCCmd) pruneCmd() *cli.Command {
	var olderThan string
	var statusesStr string
	var dryRun bool
	return &cli.Command{
		Name:  "prune",
		Usage: "Remove old done/cancelled hc items",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "older-than",
				Usage:       "Remove items older than this duration (e.g. 720h)",
				Value:       "720h",
				Destination: &olderThan,
			},
			&cli.StringFlag{
				Name:        "status",
				Usage:       "Comma-separated statuses to prune (e.g. done,cancelled)",
				Value:       "done,cancelled",
				Destination: &statusesStr,
			},
			&cli.BoolFlag{
				Name:        "dry-run",
				Usage:       "Show what would be removed without deleting",
				Destination: &dryRun,
			},
		},
		Action: func(ctx context.Context, c *cli.Command) error {
			dur, err := time.ParseDuration(olderThan)
			if err != nil {
				return fmt.Errorf("invalid --older-than duration %q: %w", olderThan, err)
			}

			parts := strings.Split(statusesStr, ",")
			statuses := make([]hc.Status, 0, len(parts))
			for _, p := range parts {
				p = strings.TrimSpace(p)
				if p == "" {
					continue
				}
				s, err := hc.ParseStatus(p)
				if err != nil {
					return fmt.Errorf("invalid status %q: %w", p, err)
				}
				statuses = append(statuses, s)
			}

			n, err := cmd.app.HC.Prune(ctx, hc.PruneOpts{
				OlderThan: dur,
				Statuses:  statuses,
				DryRun:    dryRun,
			})
			if err != nil {
				return fmt.Errorf("prune hc items: %w", err)
			}
			return iojson.WriteLine(c.Root().Writer, map[string]int{"count": n})
		},
	}
}

// timeAgo returns a human-readable description of how long ago t was.
func timeAgo(t time.Time) string {
	if t.IsZero() {
		return "never"
	}
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	}
}
