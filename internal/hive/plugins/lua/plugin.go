package lua

import (
	"context"
	"fmt"
	"maps"
	"os"

	"github.com/colonyops/hive/internal/core/config"
	"github.com/colonyops/hive/internal/hive/plugins"
	glua "github.com/yuin/gopher-lua"
)

const pluginName = "lua"

// Plugin adapts a Lua entry file to Hive's plugin interface.
type Plugin struct {
	cfg      config.LuaPluginConfig
	runtime  *Runtime
	commands map[string]config.UserCommand
}

// New creates a new Lua-backed Hive plugin.
func New(cfg config.LuaPluginConfig) *Plugin {
	return &Plugin{
		cfg:      cfg,
		commands: map[string]config.UserCommand{},
	}
}

func (p *Plugin) Name() string { return pluginName }

func (p *Plugin) Available() bool {
	info, err := os.Stat(p.cfg.ResolvedEntry())
	return err == nil && !info.IsDir()
}

func (p *Plugin) Init(_ context.Context) error {
	if p.runtime != nil {
		p.runtime.Close()
		p.runtime = nil
	}
	p.commands = map[string]config.UserCommand{}

	runtime := NewRuntime(p.Name())
	p.installHiveAPI(runtime.L, runtime.Hive())

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
	return nil
}

func (p *Plugin) Close() error {
	if p.runtime != nil {
		p.runtime.Close()
		p.runtime = nil
	}
	p.commands = map[string]config.UserCommand{}
	return nil
}

func (p *Plugin) Commands() map[string]config.UserCommand {
	commands := make(map[string]config.UserCommand, len(p.commands))
	for name, cmd := range p.commands {
		commands[name] = cmd
	}
	return commands
}

func (p *Plugin) StatusProvider() plugins.StatusProvider {
	return nil
}

func (p *Plugin) installHiveAPI(state *glua.LState, hive *glua.LTable) {
	state.SetField(hive, "commands", p.commandsFn(state))
	state.SetField(hive, "plugin", p.pluginTable(state))
}

func (p *Plugin) pluginTable(state *glua.LState) *glua.LTable {
	plugin := state.NewTable()
	state.SetField(plugin, "name", glua.LString(p.Name()))
	state.SetField(plugin, "entry", glua.LString(p.cfg.ResolvedEntry()))
	state.SetField(plugin, "module_root", glua.LString(p.cfg.ModuleRoot()))
	return plugin
}

func (p *Plugin) commandsFn(state *glua.LState) *glua.LFunction {
	return state.NewFunction(func(state *glua.LState) int {
		commandsTable := state.CheckTable(1)
		commands, err := commandsFromTable(commandsTable)
		if err != nil {
			state.RaiseError("%s", err.Error())
			return 0
		}
		maps.Copy(p.commands, commands)
		return 0
	})
}

func commandsFromTable(commandsTable *glua.LTable) (map[string]config.UserCommand, error) {
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

		commandTable, ok := value.(*glua.LTable)
		if !ok {
			parseErr = fmt.Errorf("command %q must be a table", key.String())
			return
		}

		cmd, err := userCommandFromLua(commandTable)
		if err != nil {
			parseErr = fmt.Errorf("command %q: %w", key.String(), err)
			return
		}

		commands[string(name)] = cmd
	})

	return commands, parseErr
}

func userCommandFromLua(commandTable *glua.LTable) (config.UserCommand, error) {
	sh, err := luaStringField(commandTable, "sh")
	if err != nil {
		return config.UserCommand{}, err
	}
	if sh == "" {
		return config.UserCommand{}, fmt.Errorf("field %q is required", "sh")
	}

	help, err := luaStringField(commandTable, "help")
	if err != nil {
		return config.UserCommand{}, err
	}

	confirm, err := luaStringField(commandTable, "confirm")
	if err != nil {
		return config.UserCommand{}, err
	}

	silent, err := luaBoolField(commandTable, "silent")
	if err != nil {
		return config.UserCommand{}, err
	}

	scope, err := luaStringSliceField(commandTable, "scope")
	if err != nil {
		return config.UserCommand{}, err
	}

	return config.UserCommand{
		Sh:      sh,
		Help:    help,
		Confirm: confirm,
		Silent:  silent,
		Scope:   scope,
	}, nil
}

func luaStringField(table *glua.LTable, field string) (string, error) {
	value := table.RawGetString(field)
	if value == glua.LNil {
		return "", nil
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
