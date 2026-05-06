package lua

import (
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	glua "github.com/yuin/gopher-lua"
)

// HostModule registers part of the `hive` host API on the Lua state. Each
// module owns its own state and the field(s) it attaches to the hive table;
// adding a new API surface is "implement Register and pass an instance to
// NewRuntime."
type HostModule interface {
	Register(state *glua.LState, hive *glua.LTable) error
}

// LogModule exposes `hive.log.{debug,info,warn,error}` functions that emit to
// the standard Hive log with a "plugin" field tagged with PluginName.
type LogModule struct {
	PluginName string
}

func (m *LogModule) Register(state *glua.LState, hive *glua.LTable) error {
	logs := state.NewTable()
	state.SetField(logs, "debug", m.fn(state, log.Debug))
	state.SetField(logs, "info", m.fn(state, log.Info))
	state.SetField(logs, "warn", m.fn(state, log.Warn))
	state.SetField(logs, "error", m.fn(state, log.Error))
	state.SetField(hive, "log", logs)
	return nil
}

func (m *LogModule) fn(state *glua.LState, level func() *zerolog.Event) *glua.LFunction {
	return state.NewFunction(func(state *glua.LState) int {
		level().Str("plugin", m.PluginName).Msg(state.CheckString(1))
		return 0
	})
}

// PluginInfoModule exposes static metadata about the running plugin via
// `hive.plugin.{name,entry,module_root}`.
type PluginInfoModule struct {
	Name       string
	Entry      string
	ModuleRoot string
}

func (m *PluginInfoModule) Register(state *glua.LState, hive *glua.LTable) error {
	plugin := state.NewTable()
	state.SetField(plugin, "name", glua.LString(m.Name))
	state.SetField(plugin, "entry", glua.LString(m.Entry))
	state.SetField(plugin, "module_root", glua.LString(m.ModuleRoot))
	state.SetField(hive, "plugin", plugin)
	return nil
}

// CommandsHandler is invoked when a Lua plugin calls `hive.commands(map)`. It
// receives the raw Lua table; returning an error aborts the entrypoint via
// RaiseError and surfaces the message to the Go caller.
type CommandsHandler func(*glua.LTable) error

// CommandsModule exposes `hive.commands(map)` and routes registered commands
// through Handler.
type CommandsModule struct {
	Handler CommandsHandler
}

func (m *CommandsModule) Register(state *glua.LState, hive *glua.LTable) error {
	state.SetField(hive, "commands", state.NewFunction(func(state *glua.LState) int {
		table := state.CheckTable(1)
		if err := m.Handler(table); err != nil {
			state.RaiseError("%s", err.Error())
			return 0
		}
		return 0
	}))
	return nil
}
