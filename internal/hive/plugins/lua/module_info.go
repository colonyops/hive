package lua

import glua "github.com/yuin/gopher-lua"

// PluginInfoModule exposes static metadata about the running plugin via
// `hive.plugin.{name,entry,module_root}`.
type PluginInfoModule struct {
	Name       string
	Entry      string
	ModuleRoot string
}

// Register exposes the plugin metadata as a hive.plugin subtable.
func (m *PluginInfoModule) Register(state *glua.LState, hive *glua.LTable) error {
	plugin := state.NewTable()
	state.SetField(plugin, "name", glua.LString(m.Name))
	state.SetField(plugin, "entry", glua.LString(m.Entry))
	state.SetField(plugin, "module_root", glua.LString(m.ModuleRoot))
	state.SetField(hive, "plugin", plugin)
	return nil
}
