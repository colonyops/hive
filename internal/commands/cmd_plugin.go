package commands

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/colonyops/hive/internal/hive"
	"github.com/colonyops/hive/pkg/pathutil"
	"github.com/urfave/cli/v3"
)

// PluginCmd groups plugin management subcommands.
type PluginCmd struct {
	flags *Flags
	app   *hive.App

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
		Usage:     "Scaffold the Lua plugin entry file",
		UsageText: "hive plugin init [--path <dir>] [--force]",
		Description: `Writes init.lua and commands/hello.lua into the Lua plugin directory.

By default the scaffold lands at ~/.config/hive/plugins/, where the Lua loader
auto-discovers init.lua. Use --path to write to a custom directory; you'll then
need to point ` + "`plugins.lua.entry`" + ` at <path>/init.lua in your hive config.

--force overwrites the two generated files (init.lua, commands/hello.lua);
other contents in the target directory are preserved.`,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "path",
				Usage:       "target directory (default: ~/.config/hive/plugins)",
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

func (cmd *PluginCmd) runInit(ctx context.Context, c *cli.Command) error {
	_ = ctx

	if c.Args().Len() != 0 {
		return fmt.Errorf("hive plugin init: takes no positional arguments\n\nUsage: hive plugin init [--path <dir>] [--force]")
	}

	target := defaultPluginsDir()
	if cmd.initPath != "" {
		target = pathutil.ExpandHome(cmd.initPath)
	}

	initPath := filepath.Join(target, "init.lua")
	helloPath := filepath.Join(target, "commands", "hello.lua")

	for _, p := range []string{initPath, helloPath} {
		if _, err := os.Stat(p); err == nil {
			if !cmd.initForce {
				return fmt.Errorf("%s already exists; pass --force to overwrite", p)
			}
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("stat %s: %w", p, err)
		}
	}

	if err := os.MkdirAll(filepath.Dir(helloPath), 0o755); err != nil {
		return fmt.Errorf("create commands directory: %w", err)
	}

	if err := os.WriteFile(initPath, []byte(initLuaTemplate), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", initPath, err)
	}
	if err := os.WriteFile(helloPath, []byte(helloLuaTemplate), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", helloPath, err)
	}

	_, _ = fmt.Fprintf(c.Root().ErrWriter, `Scaffolded Lua plugin at %s

Files written:
  %s
  %s

The Lua loader auto-discovers ~/.config/hive/plugins/init.lua. To use a
different location, set `+"`plugins.lua.entry`"+` in your hive config.

--force only overwrites init.lua and commands/hello.lua; other files in the
directory are preserved.
`, target, initPath, helloPath)

	return nil
}

const initLuaTemplate = `-- hive Lua plugin scaffolded by ` + "`hive plugin init`" + `.
--
-- The Lua loader auto-discovers this file at ~/.config/hive/plugins/init.lua.
-- Set ` + "`plugins.lua.entry`" + ` in your hive config to use a different location.

local hello = require("commands.hello")

return function(hive)
  hive.commands({
    LuaHello = {
      sh = "echo " .. hello.greet("hive"),
      help = "Example command from the Lua plugin scaffold",
      scope = {"sessions"},
    },
  })
end
`

const helloLuaTemplate = `-- example helper module loaded via require("commands.hello")

local M = {}

function M.greet(name)
  return "hello from " .. name
end

return M
`
