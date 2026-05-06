package lua

import (
	"fmt"

	glua "github.com/yuin/gopher-lua"
)

// LoadEntrypoint executes a Lua plugin entry file and returns the exported entrypoint function.
func (r *Runtime) LoadEntrypoint(path string) (*glua.LFunction, error) {
	base := r.L.GetTop()
	if err := r.L.DoFile(path); err != nil {
		return nil, fmt.Errorf("load lua plugin %q: %w", path, err)
	}

	returned := r.L.GetTop() - base
	if returned != 1 {
		r.L.Pop(returned)
		return nil, fmt.Errorf("lua plugin %q must return exactly one function", path)
	}

	entrypoint, ok := r.L.Get(-1).(*glua.LFunction)
	r.L.Pop(1)
	if !ok {
		return nil, fmt.Errorf("lua plugin %q must return a function", path)
	}

	return entrypoint, nil
}

func (r *Runtime) CallEntrypoint(entrypoint *glua.LFunction) error {
	return r.L.CallByParam(glua.P{
		Fn:      entrypoint,
		NRet:    0,
		Protect: true,
	}, r.Hive())
}
