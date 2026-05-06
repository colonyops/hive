package lua

import (
	glua "github.com/yuin/gopher-lua"
)

// HostModule registers part of the `hive` host API on the Lua state. Each
// module owns its own state and the field(s) it attaches to the hive table;
// adding a new API surface is "implement Register and pass an instance to
// NewRuntime."
type HostModule interface {
	Register(state *glua.LState, hive *glua.LTable) error
}
