package commands

import (
	"context"
	"fmt"

	"github.com/colonyops/hive/internal/core/todo"
	"github.com/colonyops/hive/internal/hive"
	"github.com/colonyops/hive/pkg/iojson"
	"github.com/rs/zerolog/log"
	"github.com/urfave/cli/v3"
)

func (cmd *TodoCmd) todos() *hive.TodoService {
	return cmd.app.Todos
}

// TodoCmd implements the hive todo command group.
type TodoCmd struct {
	flags *Flags
	app   *hive.App

	// create flags
	createTitle       string
	createDescription string
	createRepo        string

	// list flags
	listStatus string

	// dismiss flags (none, uses positional arg)
}

// NewTodoCmd creates a new todo command.
func NewTodoCmd(flags *Flags, app *hive.App) *TodoCmd {
	return &TodoCmd{flags: flags, app: app}
}

// Register adds the todo command to the application.
func (cmd *TodoCmd) Register(app *cli.Command) *cli.Command {
	app.Commands = append(app.Commands, &cli.Command{
		Name:  "todo",
		Usage: "Manage operator TODO items",
		Description: `TODO commands for managing operator action items.

Items are auto-created when files change in context directories,
or created manually by agents via "hive todo create".

Examples:
  hive todo list                              # list pending items
  hive todo list --status completed           # list completed items
  hive todo create --title "Review PR #42"    # create a custom TODO
  hive todo dismiss <id>                      # dismiss an item`,
		Commands: []*cli.Command{
			cmd.listCmd(),
			cmd.createCmd(),
			cmd.dismissCmd(),
			cmd.completeCmd(),
		},
	})

	return app
}

func (cmd *TodoCmd) listCmd() *cli.Command {
	return &cli.Command{
		Name:      "list",
		Aliases:   []string{"ls"},
		Usage:     "List TODO items",
		UsageText: "hive todo list [--status <status>]",
		Description: `Lists TODO items as JSON lines.

Defaults to pending items. Use --status to filter by status.

Examples:
  hive todo list
  hive todo list --status completed
  hive todo list --status dismissed`,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "status",
				Aliases:     []string{"s"},
				Usage:       "filter by status (pending, completed, dismissed)",
				Destination: &cmd.listStatus,
			},
		},
		Action: cmd.runList,
	}
}

func (cmd *TodoCmd) createCmd() *cli.Command {
	return &cli.Command{
		Name:      "create",
		Usage:     "Create a custom TODO item",
		UsageText: "hive todo create --title <title> [--description <desc>] [--repo <remote>]",
		Description: `Creates a custom TODO item for the operator.

The session ID is auto-detected from the current working directory.
Rate limited to prevent abuse (default: 20 per session per hour).

Examples:
  hive todo create --title "Review PR #42"
  hive todo create --title "Check test results" --description "CI failed on main"`,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "title",
				Aliases:     []string{"t"},
				Usage:       "title for the TODO item",
				Required:    true,
				Destination: &cmd.createTitle,
			},
			&cli.StringFlag{
				Name:        "description",
				Aliases:     []string{"d"},
				Usage:       "optional description",
				Destination: &cmd.createDescription,
			},
			&cli.StringFlag{
				Name:        "repo",
				Aliases:     []string{"r"},
				Usage:       "repository remote URL (defaults to \"unknown\" if omitted)",
				Destination: &cmd.createRepo,
			},
		},
		Action: cmd.runCreate,
	}
}

func (cmd *TodoCmd) dismissCmd() *cli.Command {
	return &cli.Command{
		Name:      "dismiss",
		Usage:     "Dismiss a TODO item",
		UsageText: "hive todo dismiss <id>",
		Description: `Dismisses a TODO item without completing it.

Examples:
  hive todo dismiss abc123`,
		Action: cmd.runDismiss,
	}
}

func (cmd *TodoCmd) completeCmd() *cli.Command {
	return &cli.Command{
		Name:      "complete",
		Usage:     "Complete a TODO item",
		UsageText: "hive todo complete <id>",
		Description: `Marks a TODO item as completed.

Examples:
  hive todo complete abc123`,
		Action: cmd.runComplete,
	}
}

func (cmd *TodoCmd) runList(ctx context.Context, c *cli.Command) error {
	filter := todo.ListFilter{}

	if cmd.listStatus != "" {
		status := todo.Status(cmd.listStatus)
		if !status.IsValid() {
			return fmt.Errorf("invalid status %q: must be one of pending, completed, dismissed", cmd.listStatus)
		}
		filter.Status = status
	} else {
		filter.Status = todo.StatusPending
	}

	items, err := cmd.todos().List(ctx, filter)
	if err != nil {
		return fmt.Errorf("list todo items: %w", err)
	}

	for _, item := range items {
		if err := iojson.WriteLine(c.Root().Writer, item); err != nil {
			return err
		}
	}

	return nil
}

func (cmd *TodoCmd) runCreate(ctx context.Context, c *cli.Command) error {
	sessionID, err := cmd.app.Sessions.DetectSession(ctx)
	if err != nil {
		log.Debug().Err(err).Msg("todo: detect session for create")
	}

	repoRemote := cmd.createRepo
	if repoRemote == "" {
		repoRemote = "unknown"
	}

	item := todo.Item{
		Title:       cmd.createTitle,
		Description: cmd.createDescription,
		SessionID:   sessionID,
		RepoRemote:  repoRemote,
	}

	if err := cmd.todos().CreateCustom(ctx, item); err != nil {
		return fmt.Errorf("create todo: %w", err)
	}

	_, _ = fmt.Fprintln(c.Root().Writer, "created")
	return nil
}

func (cmd *TodoCmd) runDismiss(ctx context.Context, c *cli.Command) error {
	if c.NArg() < 1 {
		return fmt.Errorf("usage: hive todo dismiss <id>")
	}

	id := c.Args().Get(0)
	if err := cmd.todos().Dismiss(ctx, id); err != nil {
		return fmt.Errorf("dismiss todo: %w", err)
	}

	_, _ = fmt.Fprintln(c.Root().Writer, "dismissed")
	return nil
}

func (cmd *TodoCmd) runComplete(ctx context.Context, c *cli.Command) error {
	if c.NArg() < 1 {
		return fmt.Errorf("usage: hive todo complete <id>")
	}

	id := c.Args().Get(0)
	if err := cmd.todos().Complete(ctx, id); err != nil {
		return fmt.Errorf("complete todo: %w", err)
	}

	_, _ = fmt.Fprintln(c.Root().Writer, "completed")
	return nil
}
