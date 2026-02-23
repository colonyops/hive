package commands

import (
	"context"
	"fmt"

	"github.com/colonyops/hive/internal/core/todo"
	"github.com/colonyops/hive/internal/hive"
	"github.com/colonyops/hive/pkg/iojson"
	"github.com/colonyops/hive/pkg/randid"
	"github.com/rs/zerolog/log"
	"github.com/urfave/cli/v3"
)

// TodoCmd implements the hive todo command group.
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
		Usage: "Manage human operator todo items",
		Description: `Todo commands for creating and managing operator action items.

Agents create todos to request human attention (code review, document review, etc.).
The operator manages them through the TUI action panel or CLI commands.

Session ID is auto-detected from the current working directory.`,
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
		Description: `Creates a new todo item for operator attention.

URI format: scheme://value (e.g., review://doc.md, session://abc123, https://example.com)

If no --uri is provided and a session is detected, auto-generates session://<id>.

Examples:
  hive todo add --title "Review API research" --uri "review://.hive/research/api.md"
  hive todo add --title "Review auth changes" --uri "code-review://pr/123"
  hive todo add --title "Check docs" --uri "https://example.com/docs"
  hive todo add --title "Completed migration"`,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "title",
				Aliases:     []string{"t"},
				Usage:       "todo title",
				Required:    true,
				Destination: &cmd.addTitle,
			},
			&cli.StringFlag{
				Name:        "uri",
				Aliases:     []string{"u"},
				Usage:       "URI in scheme://value format",
				Destination: &cmd.addURI,
			},
			&cli.StringFlag{
				Name:        "source",
				Aliases:     []string{"s"},
				Usage:       "source (agent, human, system)",
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
		Description: `Lists todo items as JSON lines.

Examples:
  hive todo list
  hive todo list --status pending`,
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
		Description: `Updates a todo item's status.

Status values:
  acknowledged - Operator has seen the item
  completed    - Item is done
  dismissed    - Item is not needed

Examples:
  hive todo update abc123 --status acknowledged
  hive todo update abc123 --status completed`,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "status",
				Usage:       "new status (acknowledged, completed, dismissed)",
				Required:    true,
				Destination: &cmd.updateStatus,
			},
		},
		Action: cmd.runUpdate,
	}
}

func (cmd *TodoCmd) runAdd(ctx context.Context, c *cli.Command) error {
	src, err := todo.ParseSource(cmd.addSource)
	if err != nil {
		return fmt.Errorf("invalid source %q: valid values are agent, human, system", cmd.addSource)
	}

	// Auto-detect session ID (best-effort)
	sessionID, err := cmd.app.Sessions.DetectSession(ctx)
	if err != nil {
		log.Debug().Err(err).Msg("failed to detect session for todo")
	}

	// Determine URI
	uri := cmd.addURI
	if uri == "" && sessionID != "" {
		uri = "session://" + sessionID
	}

	var ref todo.Ref
	if uri != "" {
		ref = todo.ParseRef(uri)
		if !ref.Valid() {
			return fmt.Errorf("invalid URI %q: must use scheme://value format", uri)
		}
	}

	td := todo.Todo{
		ID:        randid.Generate(8),
		SessionID: sessionID,
		Source:    src,
		Title:     cmd.addTitle,
		URI:       ref,
	}

	if err := cmd.app.Todos.Add(ctx, td); err != nil {
		return err
	}

	// Re-fetch to get populated timestamps
	created, err := cmd.app.Todos.Get(ctx, td.ID)
	if err != nil {
		return fmt.Errorf("get created todo: %w", err)
	}

	return iojson.WriteLine(c.Root().Writer, created)
}

func (cmd *TodoCmd) runList(ctx context.Context, c *cli.Command) error {
	filter := todo.ListFilter{}

	if cmd.listStatus != "" {
		status, err := todo.ParseStatus(cmd.listStatus)
		if err != nil {
			return fmt.Errorf("invalid status %q: valid values are pending, acknowledged, completed, dismissed", cmd.listStatus)
		}
		filter.Status = &status
	}

	items, err := cmd.app.Todos.List(ctx, filter)
	if err != nil {
		return fmt.Errorf("list todos: %w", err)
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

	id := c.Args().First()
	status, err := todo.ParseStatus(cmd.updateStatus)
	if err != nil {
		return fmt.Errorf("invalid status %q: valid values are pending, acknowledged, completed, dismissed", cmd.updateStatus)
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
		return fmt.Errorf("unsupported status %q", status)
	}

	if err != nil {
		return err
	}

	updated, err := cmd.app.Todos.Get(ctx, id)
	if err != nil {
		return fmt.Errorf("get updated todo: %w", err)
	}

	return iojson.WriteLine(c.Root().Writer, updated)
}
