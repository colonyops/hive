package lua

import (
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	glua "github.com/yuin/gopher-lua"
)

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
