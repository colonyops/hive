// Package lua provides Lua-backed Hive plugins.
package lua

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/rs/zerolog/log"
	glua "github.com/yuin/gopher-lua"
)

// Runtime owns a sandboxed Lua state for a Hive plugin.
type Runtime struct {
	L    *glua.LState
	hive *glua.LTable
}

// NewRuntime constructs a sandboxed Lua runtime for a Hive plugin.
func NewRuntime(pluginName string, moduleRoot string) *Runtime {
	L := glua.NewState(glua.Options{SkipOpenLibs: true})

	openLib(L, glua.BaseLibName, glua.OpenBase)
	openLib(L, glua.LoadLibName, glua.OpenPackage)
	openLib(L, glua.TabLibName, glua.OpenTable)
	openLib(L, glua.StringLibName, glua.OpenString)
	openLib(L, glua.MathLibName, glua.OpenMath)
	openLib(L, glua.CoroutineLibName, glua.OpenCoroutine)

	L.SetGlobal("loadfile", glua.LNil)
	L.SetGlobal("dofile", glua.LNil)
	L.SetGlobal("load", glua.LNil)
	configureRequire(L, moduleRoot)

	hiveTable := registerHive(L, pluginName)

	return &Runtime{L: L, hive: hiveTable}
}

// Close releases the underlying Lua state.
func (r *Runtime) Close() {
	if r == nil || r.L == nil {
		return
	}

	r.L.Close()
	r.L = nil
	r.hive = nil
}

// Hive returns the runtime's Hive API table passed to the plugin entrypoint.
func (r *Runtime) Hive() *glua.LTable { return r.hive }

func openLib(state *glua.LState, name string, fn glua.LGFunction) {
	state.Push(state.NewFunction(fn))
	state.Push(glua.LString(name))
	state.Call(1, 0)
}

func registerHive(state *glua.LState, pluginName string) *glua.LTable {
	hiveTable := state.NewTable()
	logTable := state.NewTable()

	state.SetField(logTable, "debug", newLogFn(state, func(msg string) {
		log.Debug().Str("plugin", pluginName).Msg(msg)
	}))
	state.SetField(logTable, "info", newLogFn(state, func(msg string) {
		log.Info().Str("plugin", pluginName).Msg(msg)
	}))
	state.SetField(logTable, "warn", newLogFn(state, func(msg string) {
		log.Warn().Str("plugin", pluginName).Msg(msg)
	}))
	state.SetField(logTable, "error", newLogFn(state, func(msg string) {
		log.Error().Str("plugin", pluginName).Msg(msg)
	}))

	state.SetField(hiveTable, "log", logTable)
	state.SetGlobal("hive", hiveTable)
	return hiveTable
}

func newLogFn(state *glua.LState, fn func(string)) *glua.LFunction {
	return state.NewFunction(func(state *glua.LState) int {
		fn(state.CheckString(1))
		return 0
	})
}

func configureRequire(state *glua.LState, moduleRoot string) {
	requireFn := state.GetGlobal("require")
	pkg, ok := state.GetGlobal(glua.LoadLibName).(*glua.LTable)
	if !ok {
		panic("lua package library is unavailable")
	}

	patterns := []string{
		filepath.Join(moduleRoot, "?.lua"),
		filepath.Join(moduleRoot, "?", "init.lua"),
	}
	state.SetField(pkg, "path", glua.LString(strings.Join(patterns, ";")))
	state.SetField(pkg, "cpath", glua.LString(""))

	state.SetGlobal("require", state.NewFunction(func(state *glua.LState) int {
		name := state.CheckString(1)
		if err := validateModuleName(name); err != nil {
			state.RaiseError("%s", err.Error())
			return 0
		}

		if err := state.CallByParam(glua.P{
			Fn:      requireFn,
			NRet:    1,
			Protect: true,
		}, glua.LString(name)); err != nil {
			state.RaiseError("%s", err.Error())
			return 0
		}

		return 1
	}))
}

func validateModuleName(name string) error {
	if name == "" {
		return fmt.Errorf("module name cannot be empty")
	}
	if strings.ContainsAny(name, `/\`) {
		return fmt.Errorf("module name %q must use dot notation", name)
	}

	for _, segment := range strings.Split(name, ".") {
		if segment == "" || segment == "." || segment == ".." {
			return fmt.Errorf("module name %q is invalid", name)
		}
	}

	return nil
}
