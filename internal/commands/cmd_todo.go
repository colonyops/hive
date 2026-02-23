package commands

import (
	"context"
	"fmt"

	"github.com/colonyops/hive/internal/core/todo"
	"github.com/colonyops/hive/internal/hive"
	"github.com/colonyops/hive/pkg/iojson"
	"github.com/colonyops/hive/pkg/randid"
	"github.com/urfave/cli/v3"
)

// TodoCmd implements the hive todo subcommands.
type TodoCmd struct {
	flags *Flags
	app   *hive.App

	// add flags
	addTitle  string
	addURI    string
	addSource string

	// update flags
	updateStatus string

	// list flags
	listStatus string
}

// NewTodoCmd creates a new todo command.
func NewTodoCmd(flags *Flags, app *hive.App) *TodoCmd {
	return &TodoCmd{flags: flags, app: app}
}

// Register adds the todo command to the application.
func (cmd *TodoCmd) Register(app *cli.Command) *cli.Command {
	app.Commands = append(app.Commands, &cli.Command{
		Name:  "todo",
		Usage: "Manage operator todo items",
		Commands: []*cli.Command{
			cmd.addCmd(),
			cmd.listCmd(),
			cmd.updateCmd(),
		},
	})
	return app
}

func (cmd *TodoCmd) addCmd() *cli.Command {
	return &cli.Command{
		Name:      "add",
		Usage:     "Create a new todo item",
		UsageText: "hive todo add --title <title> [--uri <uri>] [--source <source>]",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "title",
				Aliases:     []string{"t"},
				Usage:       "todo title (required)",
				Required:    true,
				Destination: &cmd.addTitle,
			},
			&cli.StringFlag{
				Name:        "uri",
				Aliases:     []string{"u"},
				Usage:       "URI in scheme://value format (e.g., session://abc, review://doc.md)",
				Destination: &cmd.addURI,
			},
			&cli.StringFlag{
				Name:        "source",
				Aliases:     []string{"s"},
				Usage:       "who created this todo (agent, human, system)",
				Value:       "agent",
				Destination: &cmd.addSource,
			},
		},
		Action: cmd.runAdd,
	}
}

func (cmd *TodoCmd) listCmd() *cli.Command {
	return &cli.Command{
		Name:      "list",
		Usage:     "List todo items",
		UsageText: "hive todo list [--status <status>]",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "status",
				Usage:       "filter by status (pending, acknowledged, completed, dismissed)",
				Destination: &cmd.listStatus,
			},
		},
		Action: cmd.runList,
	}
}

func (cmd *TodoCmd) updateCmd() *cli.Command {
	return &cli.Command{
		Name:      "update",
		Usage:     "Update a todo item's status",
		UsageText: "hive todo update <id> --status <status>",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "status",
				Usage:       "new status (pending, acknowledged, completed, dismissed)",
				Required:    true,
				Destination: &cmd.updateStatus,
			},
		},
		Action: cmd.runUpdate,
	}
}

func (cmd *TodoCmd) runAdd(ctx context.Context, c *cli.Command) error {
	source, err := todo.ParseSource(cmd.addSource)
	if err != nil {
		return fmt.Errorf("invalid source: %w", err)
	}

	uri := cmd.addURI
	sessionID, _ := cmd.app.Sessions.DetectSession(ctx)
	if uri == "" && sessionID != "" {
		uri = "session://" + sessionID
	}

	var parsedURI todo.URI
	if uri != "" {
		parsedURI = todo.ParseURI(uri)
		if !parsedURI.Valid() {
			return fmt.Errorf("invalid URI %q: must use scheme://value format", uri)
		}
	}

	t := todo.Todo{
		ID:        randid.Generate(8),
		SessionID: sessionID,
		Source:    source,
		Title:     cmd.addTitle,
		URI:       parsedURI,
	}

	if err := cmd.app.Todos.Add(ctx, t); err != nil {
		return err
	}

	// Re-fetch to get the final state (timestamps set by service)
	result, err := cmd.app.Todos.Get(ctx, t.ID)
	if err != nil {
		return err
	}

	return iojson.WriteLine(c.Root().Writer, result)
}

func (cmd *TodoCmd) runList(ctx context.Context, c *cli.Command) error {
	var filter todo.ListFilter
	if cmd.listStatus != "" {
		status, err := todo.ParseStatus(cmd.listStatus)
		if err != nil {
			return fmt.Errorf("invalid status: %w", err)
		}
		filter.Status = &status
	}

	items, err := cmd.app.Todos.List(ctx, filter)
	if err != nil {
		return err
	}

	for _, item := range items {
		if err := iojson.WriteLine(c.Root().Writer, item); err != nil {
			return err
		}
	}
	return nil
}

func (cmd *TodoCmd) runUpdate(ctx context.Context, c *cli.Command) error {
	if c.NArg() < 1 {
		return fmt.Errorf("todo ID required as argument")
	}
	id := c.Args().Get(0)

	status, err := todo.ParseStatus(cmd.updateStatus)
	if err != nil {
		return fmt.Errorf("invalid status: %w", err)
	}

	switch status {
	case todo.StatusAcknowledged:
		err = cmd.app.Todos.Acknowledge(ctx, id)
	case todo.StatusCompleted:
		err = cmd.app.Todos.Complete(ctx, id)
	case todo.StatusDismissed:
		err = cmd.app.Todos.Dismiss(ctx, id)
	case todo.StatusPending:
		return fmt.Errorf("cannot set status back to pending")
	default:
		return fmt.Errorf("unsupported status: %s", status)
	}

	if err != nil {
		return err
	}

	result, err := cmd.app.Todos.Get(ctx, id)
	if err != nil {
		return err
	}

	return iojson.WriteLine(c.Root().Writer, result)
}
