package lua

import (
	"context"
	"fmt"
	"maps"
	"os"
	"strconv"

	"github.com/colonyops/hive/internal/core/config"
	"github.com/colonyops/hive/internal/hive/plugins"
	glua "github.com/yuin/gopher-lua"
)

// Plugin adapts a Lua entry file to Hive's plugin interface.
type Plugin struct {
	cfg      config.LuaPluginConfig
	runtime  *Runtime
	commands map[string]config.UserCommand
}

// New creates a Lua-backed Hive plugin.
func New(cfg config.LuaPluginConfig) *Plugin {
	return &Plugin{cfg: cfg}
}

func (p *Plugin) Name() string { return "lua" }

func (p *Plugin) Available() bool {
	info, err := os.Stat(p.cfg.ResolvedEntry())
	return err == nil && !info.IsDir()
}

func (p *Plugin) Init(_ context.Context) error {
	if p.runtime != nil {
		p.runtime.Close()
		p.runtime = nil
	}
	p.commands = nil

	// Build into a local map so a partial init (failure during entry-file
	// load or while calling the entrypoint) cannot leave stale commands
	// reachable from MergedCommands.
	commands := map[string]config.UserCommand{}
	runtime := NewRuntime(HostMetadata{
		Name:       p.Name(),
		Entry:      p.cfg.ResolvedEntry(),
		ModuleRoot: p.cfg.ModuleRoot(),
	}, func(table *glua.LTable) error {
		next, err := commandsFromTable(table, commands)
		if err != nil {
			return err
		}
		maps.Copy(commands, next)
		return nil
	})

	entrypoint, err := runtime.LoadEntrypoint(p.cfg.ResolvedEntry())
	if err != nil {
		runtime.Close()
		return err
	}

	if err := runtime.CallEntrypoint(entrypoint); err != nil {
		runtime.Close()
		return fmt.Errorf("initialize lua plugin %q: %w", p.cfg.ResolvedEntry(), err)
	}

	p.runtime = runtime
	p.commands = commands
	return nil
}

func (p *Plugin) Close() error {
	if p.runtime != nil {
		p.runtime.Close()
		p.runtime = nil
	}
	p.commands = nil
	return nil
}

func (p *Plugin) Commands() map[string]config.UserCommand {
	return p.commands
}

func (p *Plugin) StatusProvider() plugins.StatusProvider {
	return nil
}

func commandsFromTable(commandsTable *glua.LTable, existing map[string]config.UserCommand) (map[string]config.UserCommand, error) {
	commands := make(map[string]config.UserCommand)
	var parseErr error

	commandsTable.ForEach(func(key, value glua.LValue) {
		if parseErr != nil {
			return
		}

		name, ok := key.(glua.LString)
		if !ok {
			parseErr = fmt.Errorf("command names must be strings")
			return
		}
		if _, dup := existing[string(name)]; dup {
			parseErr = fmt.Errorf("duplicate command %q", string(name))
			return
		}

		commandTable, ok := value.(*glua.LTable)
		if !ok {
			parseErr = fmt.Errorf("command %q must be a table", key.String())
			return
		}

		cmd, err := userCommandFromLua(string(name), commandTable)
		if err != nil {
			parseErr = err
			return
		}

		commands[string(name)] = cmd
	})

	return commands, parseErr
}

// Adding a new field requires updating both UserCommand in config and the
// switch below; the explicit cases for action/windows/options/form keep them
// rejected at the v1 boundary.
func userCommandFromLua(name string, commandTable *glua.LTable) (config.UserCommand, error) {
	var cmd config.UserCommand
	var parseErr error

	commandTable.ForEach(func(key, _ glua.LValue) {
		if parseErr != nil {
			return
		}

		fieldName, ok := key.(glua.LString)
		if !ok {
			parseErr = fmt.Errorf("command %q field names must be strings", name)
			return
		}

		field := string(fieldName)
		switch field {
		case "sh":
			sh, err := luaStringField(commandTable, field)
			if err != nil {
				parseErr = fmt.Errorf("command %q: %w", name, err)
				return
			}
			cmd.Sh = sh
		case "help":
			help, err := luaStringField(commandTable, field)
			if err != nil {
				parseErr = fmt.Errorf("command %q: %w", name, err)
				return
			}
			cmd.Help = help
		case "confirm":
			confirm, err := luaStringField(commandTable, field)
			if err != nil {
				parseErr = fmt.Errorf("command %q: %w", name, err)
				return
			}
			cmd.Confirm = confirm
		case "silent":
			silent, err := luaBoolField(commandTable, field)
			if err != nil {
				parseErr = fmt.Errorf("command %q: %w", name, err)
				return
			}
			cmd.Silent = silent
		case "scope":
			scope, err := luaStringSliceField(commandTable, field)
			if err != nil {
				parseErr = fmt.Errorf("command %q: %w", name, err)
				return
			}
			cmd.Scope = scope
		case "exit":
			exit, err := luaExitField(commandTable, field)
			if err != nil {
				parseErr = fmt.Errorf("command %q: %w", name, err)
				return
			}
			cmd.Exit = exit
		case "action", "windows", "options", "form":
			parseErr = fmt.Errorf("command %q: field %q is not supported by lua plugins (supported: sh, help, scope, confirm, silent, exit)", name, field)
		default:
			parseErr = fmt.Errorf("command %q: unsupported field %q (supported: sh, help, scope, confirm, silent, exit)", name, field)
		}
	})

	if parseErr != nil {
		return config.UserCommand{}, parseErr
	}

	field := fmt.Sprintf("command %q", name)
	if err := config.ValidateUserCommandBasic(field, name, cmd).ToError(); err != nil {
		return config.UserCommand{}, err
	}
	if err := config.ValidateUserCommandTemplates(field, cmd).ToError(); err != nil {
		return config.UserCommand{}, err
	}

	return cmd, nil
}

func luaStringField(table *glua.LTable, field string) (string, error) {
	value := table.RawGetString(field)
	if value == glua.LNil {
		return "", nil
	}
	if value.Type() == glua.LTFunction {
		return "", fmt.Errorf("field %q does not support callback values", field)
	}

	str, ok := value.(glua.LString)
	if !ok {
		return "", fmt.Errorf("field %q must be a string", field)
	}

	return string(str), nil
}

func luaBoolField(table *glua.LTable, field string) (bool, error) {
	value := table.RawGetString(field)
	if value == glua.LNil {
		return false, nil
	}
	if value.Type() == glua.LTFunction {
		return false, fmt.Errorf("field %q does not support callback values", field)
	}

	boolean, ok := value.(glua.LBool)
	if !ok {
		return false, fmt.Errorf("field %q must be a boolean", field)
	}

	return bool(boolean), nil
}

func luaStringSliceField(table *glua.LTable, field string) ([]string, error) {
	value := table.RawGetString(field)
	if value == glua.LNil {
		return nil, nil
	}
	if value.Type() == glua.LTFunction {
		return nil, fmt.Errorf("field %q does not support callback values", field)
	}

	list, ok := value.(*glua.LTable)
	if !ok {
		return nil, fmt.Errorf("field %q must be a table", field)
	}

	var values []string
	var parseErr error
	list.ForEach(func(_ glua.LValue, value glua.LValue) {
		if parseErr != nil {
			return
		}

		str, ok := value.(glua.LString)
		if !ok {
			parseErr = fmt.Errorf("field %q entries must be strings", field)
			return
		}

		values = append(values, string(str))
	})

	return values, parseErr
}

func luaExitField(table *glua.LTable, field string) (string, error) {
	value := table.RawGetString(field)
	if value == glua.LNil {
		return "", nil
	}
	if value.Type() == glua.LTFunction {
		return "", fmt.Errorf("field %q does not support callback values", field)
	}

	switch v := value.(type) {
	case glua.LString:
		return string(v), nil
	case glua.LBool:
		return strconv.FormatBool(bool(v)), nil
	default:
		return "", fmt.Errorf("field %q must be a string or boolean", field)
	}
}
