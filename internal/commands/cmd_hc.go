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
	"github.com/rs/zerolog/log"
	"github.com/urfave/cli/v3"
)

// HCCmd implements the hive hc command group.
type HCCmd struct {
	flags *Flags
	app   *hive.App

	// create flags
	createType     string
	createDesc     string
	createParentID string
	createAssign   bool

	// list flags
	listStatus  string
	listSession string

	// show flags
	showJSON bool

	// update flags
	updateStatus   string
	updateAssign   bool
	updateUnassign bool

	// next flags
	nextAssign bool

	// context flags
	contextJSON bool

	// prune flags
	pruneOlderThan string
	pruneStatuses  []string
	pruneDryRun    bool
}

// NewHCCmd creates a new hc command.
func NewHCCmd(flags *Flags, app *hive.App) *HCCmd {
	return &HCCmd{flags: flags, app: app}
}

// Register adds the hc command to the application.
func (cmd *HCCmd) Register(app *cli.Command) *cli.Command {
	app.Commands = append(app.Commands, &cli.Command{
		Name:  "hc",
		Usage: "Manage multi-agent task coordination (Honeycomb)",
		Description: `Honeycomb (hc) is a built-in multi-agent task coordination system.

A conductor agent creates epics and tasks; worker agents claim leaf items
via 'hive hc next', record progress with 'hive hc checkpoint', and mark
items done with 'hive hc update'.

Session ID and repo key are auto-detected from the working directory.`,
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

func (cmd *HCCmd) detectSession(ctx context.Context) string {
	sessionID, err := cmd.app.Sessions.DetectSession(ctx)
	if err != nil {
		log.Debug().Err(err).Msg("failed to detect session for hc")
	}
	return sessionID
}

func (cmd *HCCmd) detectRepoKey(ctx context.Context) string {
	url, err := cmd.app.Sessions.Git().RemoteURL(ctx, ".")
	if err != nil {
		log.Debug().Err(err).Msg("failed to get remote URL for hc")
		return ""
	}
	owner, repoName := git.ExtractOwnerRepo(url)
	if owner == "" || repoName == "" {
		return ""
	}
	return owner + "/" + repoName
}

func (cmd *HCCmd) createCmd() *cli.Command {
	bulk := iojson.FileReader[hc.CreateInput]{}
	return &cli.Command{
		Name:      "create",
		Usage:     "Create a task or epic",
		UsageText: "hive hc create [title] [--type epic|task] [--desc <desc>] [--parent <id>]",
		Description: `Creates a single item from flags, or a bulk tree from JSON (--file or stdin).

Bulk JSON format:
  {"title":"My Epic","type":"epic","children":[{"title":"Task 1","type":"task"}]}

Examples:
  hive hc create "Implement auth" --type task --parent hc-abc123
  echo '{"title":"Epic","type":"epic","children":[...]}' | hive hc create`,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "type",
				Aliases:     []string{"t"},
				Usage:       "item type (epic, task)",
				Value:       "task",
				Destination: &cmd.createType,
			},
			&cli.StringFlag{
				Name:        "desc",
				Aliases:     []string{"d"},
				Usage:       "item description",
				Destination: &cmd.createDesc,
			},
			&cli.StringFlag{
				Name:        "parent",
				Aliases:     []string{"p"},
				Usage:       "parent item ID",
				Destination: &cmd.createParentID,
			},
			&cli.BoolFlag{
				Name:        "assign",
				Usage:       "assign item to current session",
				Destination: &cmd.createAssign,
			},
			bulk.Flag(),
		},
		Action: func(ctx context.Context, c *cli.Command) error {
			repoKey := cmd.detectRepoKey(ctx)

			// bulk mode: --file set OR no positional args
			if c.IsSet("file") || c.NArg() == 0 {
				input, err := bulk.Read()
				if err != nil {
					return fmt.Errorf("read input: %w", err)
				}
				items, err := cmd.app.Honeycomb.CreateBulk(ctx, repoKey, input)
				if err != nil {
					return fmt.Errorf("create bulk: %w", err)
				}
				for _, item := range items {
					if err := iojson.WriteLine(c.Root().Writer, item); err != nil {
						return err
					}
				}
				return nil
			}

			// single-item mode
			itemType, err := hc.ParseItemType(cmd.createType)
			if err != nil {
				return fmt.Errorf("invalid type %q: valid values are epic, task", cmd.createType)
			}

			input := hc.CreateItemInput{
				Title:    c.Args().First(),
				Desc:     cmd.createDesc,
				Type:     itemType,
				ParentID: cmd.createParentID,
			}

			item, err := cmd.app.Honeycomb.CreateItem(ctx, repoKey, input)
			if err != nil {
				return fmt.Errorf("create item: %w", err)
			}

			if cmd.createAssign {
				sessionID := cmd.detectSession(ctx)
				if sessionID != "" {
					updated, err := cmd.app.Honeycomb.UpdateItem(ctx, item.ID, hc.ItemUpdate{SessionID: &sessionID})
					if err != nil {
						return fmt.Errorf("assign item: %w", err)
					}
					item = updated
				}
			}

			return iojson.WriteLine(c.Root().Writer, item)
		},
	}
}

func (cmd *HCCmd) listCmd() *cli.Command {
	return &cli.Command{
		Name:      "list",
		Usage:     "List items",
		UsageText: "hive hc list [epic-id] [--status <status>] [--session <id>]",
		Description: `Lists items as JSON lines. Optional positional arg filters by epic ID.

Examples:
  hive hc list
  hive hc list hc-abc123
  hive hc list --status open
  hive hc list --session mysession`,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "status",
				Usage:       "filter by status (open, in_progress, done, cancelled)",
				Destination: &cmd.listStatus,
			},
			&cli.StringFlag{
				Name:        "session",
				Usage:       "filter by session ID",
				Destination: &cmd.listSession,
			},
		},
		Action: func(ctx context.Context, c *cli.Command) error {
			filter := hc.ListFilter{
				EpicID:    c.Args().First(),
				SessionID: cmd.listSession,
			}

			if cmd.listStatus != "" {
				s, err := hc.ParseStatus(cmd.listStatus)
				if err != nil {
					return fmt.Errorf("invalid status %q: valid values are open, in_progress, done, cancelled", cmd.listStatus)
				}
				filter.Status = &s
			}

			items, err := cmd.app.Honeycomb.ListItems(ctx, filter)
			if err != nil {
				return fmt.Errorf("list items: %w", err)
			}

			for _, item := range items {
				if err := iojson.WriteLine(c.Root().Writer, item); err != nil {
					return err
				}
			}
			return nil
		},
	}
}

func (cmd *HCCmd) showCmd() *cli.Command {
	return &cli.Command{
		Name:      "show",
		Usage:     "Show an item and its comments",
		UsageText: "hive hc show <id> [--json]",
		Description: `Displays an item and all associated comments as JSON lines.

The item is written first, followed by each comment.

Examples:
  hive hc show hc-abc123
  hive hc show hc-abc123 --json`,
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:        "json",
				Usage:       "output as JSON lines (default)",
				Destination: &cmd.showJSON,
			},
		},
		Action: func(ctx context.Context, c *cli.Command) error {
			if c.NArg() < 1 {
				return fmt.Errorf("item ID required as argument")
			}
			id := c.Args().First()

			item, err := cmd.app.Honeycomb.GetItem(ctx, id)
			if err != nil {
				return fmt.Errorf("get item %q: %w", id, err)
			}

			if err := iojson.WriteLine(c.Root().Writer, item); err != nil {
				return err
			}

			comments, err := cmd.app.Honeycomb.ListComments(ctx, id)
			if err != nil {
				return fmt.Errorf("list comments for %q: %w", id, err)
			}

			for _, comment := range comments {
				if err := iojson.WriteLine(c.Root().Writer, comment); err != nil {
					return err
				}
			}

			return nil
		},
	}
}

func (cmd *HCCmd) updateCmd() *cli.Command {
	return &cli.Command{
		Name:      "update",
		Usage:     "Update an item",
		UsageText: "hive hc update <id> [--status <status>] [--assign] [--unassign]",
		Description: `Updates an item's status or session assignment.

Examples:
  hive hc update hc-abc123 --status in_progress
  hive hc update hc-abc123 --status done
  hive hc update hc-abc123 --assign
  hive hc update hc-abc123 --unassign`,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "status",
				Aliases:     []string{"s"},
				Usage:       "new status (open, in_progress, done, cancelled)",
				Destination: &cmd.updateStatus,
			},
			&cli.BoolFlag{
				Name:        "assign",
				Usage:       "assign item to current session",
				Destination: &cmd.updateAssign,
			},
			&cli.BoolFlag{
				Name:        "unassign",
				Usage:       "remove session assignment from item",
				Destination: &cmd.updateUnassign,
			},
		},
		Action: func(ctx context.Context, c *cli.Command) error {
			if c.NArg() < 1 {
				return fmt.Errorf("item ID required as argument")
			}
			id := c.Args().First()

			update := hc.ItemUpdate{}

			if cmd.updateStatus != "" {
				s, err := hc.ParseStatus(cmd.updateStatus)
				if err != nil {
					return fmt.Errorf("invalid status %q: valid values are open, in_progress, done, cancelled", cmd.updateStatus)
				}
				update.Status = &s
			}

			if cmd.updateAssign && cmd.updateUnassign {
				return fmt.Errorf("--assign and --unassign are mutually exclusive")
			}

			if cmd.updateAssign {
				sessionID := cmd.detectSession(ctx)
				if sessionID == "" {
					return fmt.Errorf("could not detect current session; use 'hive session' to verify")
				}
				update.SessionID = &sessionID
			}

			if cmd.updateUnassign {
				empty := ""
				update.SessionID = &empty
			}

			item, err := cmd.app.Honeycomb.UpdateItem(ctx, id, update)
			if err != nil {
				return fmt.Errorf("update item %q: %w", id, err)
			}

			return iojson.WriteLine(c.Root().Writer, item)
		},
	}
}

func (cmd *HCCmd) nextCmd() *cli.Command {
	return &cli.Command{
		Name:      "next",
		Usage:     "Get the next actionable task",
		UsageText: "hive hc next [epic-id] [--assign]",
		Description: `Returns the next actionable leaf task for the current session or epic.

Actionable means the task has status open or in_progress and no open/in_progress children.

If --assign is set, the task is assigned to the current session and its status
is set to in_progress.

Examples:
  hive hc next
  hive hc next hc-epic123
  hive hc next --assign`,
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:        "assign",
				Usage:       "assign item to current session and set status to in_progress",
				Destination: &cmd.nextAssign,
			},
		},
		Action: func(ctx context.Context, c *cli.Command) error {
			filter := hc.NextFilter{
				EpicID: c.Args().First(),
			}

			item, found, err := cmd.app.Honeycomb.Next(ctx, filter)
			if err != nil {
				return fmt.Errorf("next item: %w", err)
			}
			if !found {
				return fmt.Errorf("no actionable tasks found")
			}

			if cmd.nextAssign {
				sessionID := cmd.detectSession(ctx)
				if sessionID == "" {
					return fmt.Errorf("could not detect current session; use 'hive session' to verify")
				}
				statusInProgress := hc.StatusInProgress
				updated, err := cmd.app.Honeycomb.UpdateItem(ctx, item.ID, hc.ItemUpdate{
					Status:    &statusInProgress,
					SessionID: &sessionID,
				})
				if err != nil {
					return fmt.Errorf("assign item: %w", err)
				}
				item = updated
			}

			return iojson.WriteLine(c.Root().Writer, item)
		},
	}
}

func (cmd *HCCmd) logCmd() *cli.Command {
	return &cli.Command{
		Name:      "log",
		Usage:     "Add a log comment to an item",
		UsageText: "hive hc log <id> <message>",
		Description: `Attaches a general log comment to an item. Use for recording observations,
decisions, or progress notes during implementation.

Examples:
  hive hc log hc-abc123 "Decided to use JWT for auth"
  hive hc log hc-abc123 "Added rate limiting middleware"`,
		Action: func(ctx context.Context, c *cli.Command) error {
			if c.NArg() < 2 {
				return fmt.Errorf("item ID and message required as arguments")
			}
			id := c.Args().Get(0)
			message := strings.Join(c.Args().Slice()[1:], " ")

			comment, err := cmd.app.Honeycomb.AddComment(ctx, id, message)
			if err != nil {
				return fmt.Errorf("add comment to %q: %w", id, err)
			}

			return iojson.WriteLine(c.Root().Writer, comment)
		},
	}
}

func (cmd *HCCmd) checkpointCmd() *cli.Command {
	return &cli.Command{
		Name:      "checkpoint",
		Usage:     "Record a handoff checkpoint comment",
		UsageText: "hive hc checkpoint <id> <message>",
		Description: `Attaches a checkpoint comment to an item. Use when stopping mid-implementation
to leave context for the next agent picking up the work.

The message is prefixed with "CHECKPOINT:" to make it easy to identify in context views.

Examples:
  hive hc checkpoint hc-abc123 "Implemented login endpoint, auth middleware pending"
  hive hc checkpoint hc-abc123 "DB schema done, need to wire up API handlers"`,
		Action: func(ctx context.Context, c *cli.Command) error {
			if c.NArg() < 2 {
				return fmt.Errorf("item ID and message required as arguments")
			}
			id := c.Args().Get(0)
			message := "CHECKPOINT: " + strings.Join(c.Args().Slice()[1:], " ")

			comment, err := cmd.app.Honeycomb.AddComment(ctx, id, message)
			if err != nil {
				return fmt.Errorf("add checkpoint to %q: %w", id, err)
			}

			return iojson.WriteLine(c.Root().Writer, comment)
		},
	}
}

func (cmd *HCCmd) contextCmd() *cli.Command {
	return &cli.Command{
		Name:      "context",
		Usage:     "Show context block for an epic",
		UsageText: "hive hc context <epic-id> [--json]",
		Description: `Assembles and displays the context block for an epic.

The context block contains:
  - Epic title and description
  - Task counts by status
  - Tasks assigned to the current session (with latest comment)
  - All open/in-progress tasks

Without --json, outputs a markdown representation suitable for AI agent consumption.
With --json, outputs a single JSON object.

Examples:
  hive hc context hc-epic123
  hive hc context hc-epic123 --json`,
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:        "json",
				Usage:       "output as JSON",
				Destination: &cmd.contextJSON,
			},
		},
		Action: func(ctx context.Context, c *cli.Command) error {
			if c.NArg() < 1 {
				return fmt.Errorf("epic ID required as argument")
			}
			epicID := c.Args().First()
			sessionID := cmd.detectSession(ctx)

			cb, err := cmd.app.Honeycomb.Context(ctx, epicID, sessionID)
			if err != nil {
				return fmt.Errorf("get context for epic %q: %w", epicID, err)
			}

			if cmd.contextJSON {
				return iojson.WriteWith(c.Root().Writer, c.Root().ErrWriter, cb)
			}

			_, err = fmt.Fprint(c.Root().Writer, cb.String())
			return err
		},
	}
}

func (cmd *HCCmd) pruneCmd() *cli.Command {
	return &cli.Command{
		Name:      "prune",
		Usage:     "Remove old completed items",
		UsageText: "hive hc prune [--older-than <duration>] [--status <status>...] [--dry-run]",
		Description: `Removes items older than the specified duration with matching statuses.

Duration format: Go duration string (e.g. 24h, 7d, 168h).

Examples:
  hive hc prune --older-than 168h
  hive hc prune --older-than 24h --status done --status cancelled
  hive hc prune --dry-run`,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "older-than",
				Usage:       "remove items older than this duration (e.g. 24h, 168h)",
				Value:       "168h",
				Destination: &cmd.pruneOlderThan,
			},
			&cli.StringSliceFlag{
				Name:        "status",
				Usage:       "statuses to prune (default: done, cancelled)",
				Destination: &cmd.pruneStatuses,
			},
			&cli.BoolFlag{
				Name:        "dry-run",
				Usage:       "show what would be pruned without removing",
				Destination: &cmd.pruneDryRun,
			},
		},
		Action: func(ctx context.Context, c *cli.Command) error {
			olderThan, err := time.ParseDuration(cmd.pruneOlderThan)
			if err != nil {
				return fmt.Errorf("invalid duration %q: %w", cmd.pruneOlderThan, err)
			}

			statusStrings := cmd.pruneStatuses
			statuses := make([]hc.Status, 0, len(statusStrings))
			if len(statusStrings) == 0 {
				statuses = []hc.Status{hc.StatusDone, hc.StatusCancelled}
			} else {
				for _, s := range statusStrings {
					status, err := hc.ParseStatus(s)
					if err != nil {
						return fmt.Errorf("invalid status %q: valid values are open, in_progress, done, cancelled", s)
					}
					statuses = append(statuses, status)
				}
			}

			opts := hc.PruneOpts{
				OlderThan: olderThan,
				Statuses:  statuses,
				DryRun:    cmd.pruneDryRun,
			}

			count, err := cmd.app.Honeycomb.Prune(ctx, opts)
			if err != nil {
				return fmt.Errorf("prune: %w", err)
			}

			action := "pruned"
			if cmd.pruneDryRun {
				action = "would prune"
			}
			_, err = fmt.Fprintf(c.Root().Writer, `{"action":%q,"count":%d}`+"\n", action, count)
			return err
		},
	}
}
