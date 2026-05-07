package commands

import (
	"context"
	"fmt"

	"github.com/colonyops/hive/internal/hive"
	"github.com/urfave/cli/v3"
)

// PluginCmd groups plugin management subcommands.
type PluginCmd struct {
	flags *Flags
	app   *hive.App

	// init subcommand state
	initPath  string
	initForce bool
}

// NewPluginCmd creates a new plugin command.
func NewPluginCmd(flags *Flags, app *hive.App) *PluginCmd {
	return &PluginCmd{flags: flags, app: app}
}

// Register adds the plugin command group to the CLI.
func (cmd *PluginCmd) Register(app *cli.Command) *cli.Command {
	app.Commands = append(app.Commands, &cli.Command{
		Name:  "plugin",
		Usage: "Plugin management commands",
		Commands: []*cli.Command{
			cmd.initCmd(),
		},
	})
	return app
}

func (cmd *PluginCmd) initCmd() *cli.Command {
	return &cli.Command{
		Name:      "init",
		Usage:     "Scaffold a new Lua plugin directory",
		UsageText: "hive plugin init <name> [--path <dir>] [--force]",
		Description: `Creates a new Lua plugin scaffold with init.lua and commands/hello.lua.

By default, the plugin is created at ~/.config/hive/plugins/<name>/.
Use --path to write to a custom location.
Use --force to overwrite the two generated files (init.lua, commands/hello.lua);
other contents in the target directory are preserved.`,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "path",
				Usage:       "target directory (default: ~/.config/hive/plugins/<name>)",
				Destination: &cmd.initPath,
			},
			&cli.BoolFlag{
				Name:        "force",
				Usage:       "overwrite the two generated files if they exist",
				Destination: &cmd.initForce,
			},
		},
		Action: cmd.runInit,
	}
}

// runInit is the body of `hive plugin init`. The implementation is filled in
// by a separate task in this epic (hc-otjdbfl7); this skeleton stub keeps the
// command wiring buildable.
func (cmd *PluginCmd) runInit(ctx context.Context, c *cli.Command) error {
	_ = ctx
	_ = c
	return fmt.Errorf("hive plugin init: not yet implemented")
}
