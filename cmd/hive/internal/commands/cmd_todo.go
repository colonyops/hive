package commands

import (
	"github.com/colonyops/hive/cmd/hive/internal/app"
	"github.com/urfave/cli/v3"
)

// TodoCmd implements the hive todo command group.
type TodoCmd struct {
	flags *Flags
	app   *app.App
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
ones Hive Desktop serves over MCP.

Session ID is auto-detected from the current working directory.`,
	})

	return app
}
