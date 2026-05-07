package commands

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"text/template"

	"github.com/colonyops/hive/internal/hive"
	"github.com/colonyops/hive/pkg/pathutil"
	"github.com/urfave/cli/v3"
)

// pluginNameRe enforces that plugin names start with a lowercase letter and
// contain only lowercase letters, digits, and dashes. This rejects path
// separators, dots (including ".."), uppercase, and other special characters.
var pluginNameRe = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)

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

// runInit is the body of `hive plugin init`. It validates the plugin name,
// resolves the target directory (default ~/.config/hive/plugins/<name>),
// refuses to overwrite existing files unless --force is set, and writes a
// two-file Lua scaffold: init.lua and commands/hello.lua.
func (cmd *PluginCmd) runInit(ctx context.Context, c *cli.Command) error {
	_ = ctx

	if c.Args().Len() != 1 {
		return fmt.Errorf("hive plugin init: requires exactly one argument: <name>\n\nUsage: hive plugin init <name> [--path <dir>] [--force]")
	}
	name := c.Args().First()

	if !pluginNameRe.MatchString(name) {
		return fmt.Errorf("invalid plugin name %q: name must match [a-z][a-z0-9-]* (start with lowercase letter; only lowercase letters, digits, dashes)", name)
	}

	var target string
	if cmd.initPath != "" {
		target = pathutil.ExpandHome(cmd.initPath)
	} else {
		target = filepath.Join(defaultPluginsDir(), name)
	}

	if _, err := os.Stat(target); err == nil {
		if !cmd.initForce {
			return fmt.Errorf("%s already exists; pass --force to overwrite the two generated files", target)
		}
	} else if !os.IsNotExist(err) {
		if !cmd.initForce {
			return fmt.Errorf("%s already exists; pass --force to overwrite the two generated files", target)
		}
	}

	if err := os.MkdirAll(target, 0o755); err != nil {
		return fmt.Errorf("create plugin directory: %w", err)
	}
	commandsDir := filepath.Join(target, "commands")
	if err := os.MkdirAll(commandsDir, 0o755); err != nil {
		return fmt.Errorf("create commands directory: %w", err)
	}

	data := struct {
		Name          string
		CommandPrefix string
	}{
		Name:          name,
		CommandPrefix: kebabToPascal(name),
	}

	initTmpl, err := template.New("init.lua").Parse(initLuaTemplate)
	if err != nil {
		return fmt.Errorf("parse init template: %w", err)
	}
	helloTmpl, err := template.New("hello.lua").Parse(helloLuaTemplate)
	if err != nil {
		return fmt.Errorf("parse hello template: %w", err)
	}

	var initBuf bytes.Buffer
	if err := initTmpl.Execute(&initBuf, data); err != nil {
		return fmt.Errorf("render init template: %w", err)
	}
	var helloBuf bytes.Buffer
	if err := helloTmpl.Execute(&helloBuf, data); err != nil {
		return fmt.Errorf("render hello template: %w", err)
	}

	initPath := filepath.Join(target, "init.lua")
	helloPath := filepath.Join(commandsDir, "hello.lua")

	if err := os.WriteFile(initPath, initBuf.Bytes(), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", initPath, err)
	}
	if err := os.WriteFile(helloPath, helloBuf.Bytes(), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", helloPath, err)
	}

	fmt.Fprintf(os.Stderr, `Scaffolded plugin %q at %s

Files written:
  %s
  %s

To activate the plugin, do one of:

  1. Set in your hive config:
         plugins:
           lua:
             entry: %s

  2. require() it from your top-level ~/.config/hive/plugins/init.lua:
         local %s = require("%s.init")

--force only overwrites init.lua and commands/hello.lua; other files in the
directory are preserved.
`, name, target, initPath, helloPath, initPath, name, name)

	return nil
}

// kebabToPascal converts a kebab-case identifier (e.g. "my-plugin") into
// PascalCase (e.g. "MyPlugin"). Empty segments are skipped. The plugin name
// validator restricts inputs to ASCII lowercase letters, digits, and dashes,
// so byte-level uppercasing is sufficient.
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

func byteToUpper(b byte) byte {
	if b >= 'a' && b <= 'z' {
		return b - ('a' - 'A')
	}
	return b
}

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

const helloLuaTemplate = `-- {{.Name}}: example helper module loaded via require("commands.hello")

local M = {}

function M.greet(name)
  return "hello from " .. name
end

return M
`
