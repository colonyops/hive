// Package lua provides Lua-backed Hive plugins.
package lua

import (
	"fmt"
	"path/filepath"
	"strings"

	glua "github.com/yuin/gopher-lua"
)

// Runtime owns a sandboxed Lua state for a Hive plugin. The state is private:
// callers interact through LoadEntrypoint / CallEntrypoint / Close.
type Runtime struct {
	state *glua.LState
}

// NewRuntime constructs a sandboxed Lua runtime, configures `require()` to
// resolve relative to moduleRoot, and registers each HostModule onto the
// global `hive` table in order.
func NewRuntime(moduleRoot string, modules ...HostModule) (*Runtime, error) {
	state := glua.NewState(glua.Options{SkipOpenLibs: true})

	// Opt-in standard libraries: omit io/os/debug so plugins cannot touch the
	// filesystem, spawn processes, or escape via the debug introspection API.
	openLib(state, glua.BaseLibName, glua.OpenBase)
	openLib(state, glua.LoadLibName, glua.OpenPackage)
	openLib(state, glua.TabLibName, glua.OpenTable)
	openLib(state, glua.StringLibName, glua.OpenString)
	openLib(state, glua.MathLibName, glua.OpenMath)
	openLib(state, glua.CoroutineLibName, glua.OpenCoroutine)

	// Disable bytecode and free-form file loaders; require() is the only path
	// for pulling in additional Lua, and configureRequire below pins it to
	// moduleRoot.
	state.SetGlobal("loadfile", glua.LNil)
	state.SetGlobal("dofile", glua.LNil)
	state.SetGlobal("load", glua.LNil)
	configureRequire(state, moduleRoot)

	hive := state.NewTable()
	for _, m := range modules {
		if err := m.Register(state, hive); err != nil {
			state.Close()
			return nil, fmt.Errorf("register host module: %w", err)
		}
	}
	state.SetGlobal("hive", hive)

	return &Runtime{state: state}, nil
}

// LoadEntrypoint executes the plugin entry file and returns the function it
// must yield as its single return value.
func (r *Runtime) LoadEntrypoint(path string) (*glua.LFunction, error) {
	base := r.state.GetTop()
	if err := r.state.DoFile(path); err != nil {
		return nil, fmt.Errorf("load lua plugin %q: %w", path, err)
	}

	returned := r.state.GetTop() - base
	if returned != 1 {
		r.state.Pop(returned)
		return nil, fmt.Errorf("lua plugin %q must return exactly one function", path)
	}

	entrypoint, ok := r.state.Get(-1).(*glua.LFunction)
	r.state.Pop(1)
	if !ok {
		return nil, fmt.Errorf("lua plugin %q must return a function", path)
	}

	return entrypoint, nil
}

// CallEntrypoint invokes the plugin entry function in protected mode, passing
// the global `hive` table as its single argument.
func (r *Runtime) CallEntrypoint(entrypoint *glua.LFunction) error {
	hive, ok := r.state.GetGlobal("hive").(*glua.LTable)
	if !ok {
		return fmt.Errorf("internal error: hive table missing from lua runtime")
	}
	return r.state.CallByParam(glua.P{
		Fn:      entrypoint,
		NRet:    0,
		Protect: true,
	}, hive)
}

// Close releases the underlying Lua state. Safe on a nil receiver and on a
// runtime that has already been closed.
func (r *Runtime) Close() {
	if r == nil || r.state == nil {
		return
	}
	r.state.Close()
	r.state = nil
}

func openLib(state *glua.LState, name string, fn glua.LGFunction) {
	state.Push(state.NewFunction(fn))
	state.Push(glua.LString(name))
	state.Call(1, 0)
}

// configureRequire pins package.path to moduleRoot and wraps require() so
// module names cannot escape that directory via path separators or `..`.
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

// validateModuleName rejects require() arguments that could escape the plugin
// module root, mirroring the path-traversal guards used elsewhere in Hive.
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
