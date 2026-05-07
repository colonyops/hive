package commands

import (
	"context"
	"fmt"
	"strings"

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

// kebabToPascal converts a kebab-case identifier (e.g. "my-plugin") into
// PascalCase (e.g. "MyPlugin"). Empty segments are skipped. The plugin name
// validator restricts inputs to ASCII lowercase letters, digits, and dashes,
// so byte-level uppercasing is sufficient.
//
//nolint:unused // consumed by hive plugin init runInit
func kebabToPascal(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, segment := range strings.Split(s, "-") {
		if segment == "" {
			continue
		}
		b.WriteByte(byteToUpper(segment[0]))
		b.WriteString(segment[1:])
	}
	return b.String()
}

//nolint:unused // consumed by hive plugin init runInit
func byteToUpper(b byte) byte {
	if b >= 'a' && b <= 'z' {
		return b - ('a' - 'A')
	}
	return b
}

//nolint:unused // consumed by hive plugin init runInit
const initLuaTemplate = `-- {{.Name}}: hive Lua plugin scaffolded by ` + "`hive plugin init`" + `
--
-- This file is the entrypoint. The plugin discovery loader looks for it at
-- ~/.config/hive/plugins/{{.Name}}/init.lua. To activate it, either set
--
--   plugins:
--     lua:
--       entry: ~/.config/hive/plugins/{{.Name}}/init.lua
--
-- in your hive config, or ` + "`" + `require("{{.Name}}.init")` + "`" + ` from your top-level
-- ~/.config/hive/plugins/init.lua.

local hello = require("commands.hello")

return function(hive)
  hive.commands({
    {{.CommandPrefix}}Hello = {
      sh = "echo " .. hello.greet("{{.Name}}"),
      help = "Say hello from the {{.Name}} plugin",
      scope = {"sessions"},
    },
  })
end

-- Future M-table form (planned in #301) -- for reference only:
--
-- local M = {}
-- function M.setup(hive)
--   hive.commands({ ... })
-- end
-- return M
`

//nolint:unused // consumed by hive plugin init runInit
const helloLuaTemplate = `-- {{.Name}}: example helper module loaded via require("commands.hello")

local M = {}

function M.greet(name)
  return "hello from " .. name
end

return M
`
