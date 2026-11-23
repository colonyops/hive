package commands

import (
	"context"
	"fmt"
	"strings"

	"github.com/colonyops/hive/cmd/hive/internal/app"
	"github.com/colonyops/hive/internal/domain/todo"
	"github.com/colonyops/hive/internal/hive/engineops"
	sessionsvc "github.com/colonyops/hive/internal/hive/session"
	"github.com/colonyops/hive/internal/ops/todos"
	"github.com/colonyops/hive/pkg/iojson"
	"github.com/urfave/cli/v3"
)

// TodoCmd implements the hive todo command group.
type TodoCmd struct {
	flags *Flags
	app   *app.App

	addTitle  string
	addURI    string
	addSource string
}

// NewTodoCmd creates a new todo command.
func NewTodoCmd(flags *Flags, app *app.App) *TodoCmd {
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

create, list and update are the todos operations of the catalog, the same
ones Hive Desktop serves over MCP. add is the older spelling of create.

Session ID is auto-detected from the current working directory.`,
		Commands: []*cli.Command{
			cmd.addCmd(),
		},
	})

	return app
}

func (cmd *TodoCmd) addCmd() *cli.Command {
	return &cli.Command{
		Name:      "add",
		Metadata:  alwaysJSON(),
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
				Usage:       fmt.Sprintf("source (%s)", strings.Join(todo.SourceNames(), ", ")),
				Value:       todo.SourceAgent.String(),
				Destination: &cmd.addSource,
			},
		},
		Action: cmd.runAdd,
	}
}

func (cmd *TodoCmd) runAdd(ctx context.Context, c *cli.Command) error {
	if _, err := todo.ParseSource(cmd.addSource); err != nil {
		return invalidf("invalid source %q: valid values are %s", cmd.addSource, strings.Join(todo.SourceNames(), ", "))
	}
	caller := callingSessionID(ctx, cmd.app, "", "cli.todo")
	if caller == "" {
		caller = sessionsvc.CallerGlobal
	}
	created, err := engineops.New(cmd.app.Logger, cmd.app.Engine).TodoCreate(ctx, todos.TodoCreateIn{
		Caller: caller, Title: cmd.addTitle, URI: cmd.addURI, Source: cmd.addSource,
	})
	if err != nil {
		return err
	}
	return iojson.WriteLine(c.Root().Writer, created)
}
