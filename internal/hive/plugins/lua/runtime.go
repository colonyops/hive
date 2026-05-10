// Package lua provides Lua-backed Hive plugins.
package lua

import (
	"context"
	"fmt"
	"path/filepath"
	"runtime/debug"
	"strings"
	"sync"

	"github.com/rs/zerolog"
	glua "github.com/yuin/gopher-lua"
)

// Runtime owns a sandboxed Lua state. Exactly one goroutine — the
// dispatcher started in NewRuntime — touches state. Schedule work via
// Submit, LoadEntrypoint, or CallEntrypoint; tear down with Close.
type Runtime struct {
	state  *glua.LState
	logger zerolog.Logger

	work   chan func(*glua.LState)
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	mu     sync.Mutex
	closed bool
}

// NewRuntime constructs a sandboxed Lua runtime, configures `require()` to
// resolve relative to moduleRoot, and registers each HostModule onto the
// global `hive` table. queueSize bounds the dispatcher's work-channel buffer;
// producers that exceed it drop with a warn-level log via Submit. Setup is
// synchronous; the dispatcher goroutine takes over LState ownership when
// this returns.
func NewRuntime(logger zerolog.Logger, moduleRoot string, queueSize int, modules ...HostModule) (*Runtime, error) {
	if queueSize < 1 {
		return nil, fmt.Errorf("dispatcher queue size must be >= 1, got %d", queueSize)
	}

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

	ctx, cancel := context.WithCancel(context.Background())
	r := &Runtime{
		state:  state,
		logger: logger,
		work:   make(chan func(*glua.LState), queueSize),
		ctx:    ctx,
		cancel: cancel,
	}

	r.wg.Add(1)
	go r.dispatch()

	return r, nil
}

// dispatch drains the work channel on the goroutine that owns r.state.
// Returns once ctx is cancelled and the channel is drained.
func (r *Runtime) dispatch() {
	defer r.wg.Done()
	for {
		select {
		case fn := <-r.work:
			if fn != nil {
				fn(r.state)
			}
		case <-r.ctx.Done():
			// Drain so submitSync callers that won the race against
			// Close still get their result.
			for {
				select {
				case fn := <-r.work:
					if fn != nil {
						fn(r.state)
					}
				default:
					return
				}
			}
		}
	}
}

// Submit schedules fn on the dispatcher. Fire-and-forget, never blocks.
// Drops and logs if the queue is full or the runtime is closed.
func (r *Runtime) Submit(fn func(*glua.LState)) {
	if r == nil || fn == nil {
		return
	}

	r.mu.Lock()
	closed := r.closed
	r.mu.Unlock()
	if closed {
		r.logger.Debug().Msg("dropped lua work item: runtime closed")
		return
	}
	// The work channel is never closed (ctx signals shutdown), so a
	// non-blocking send is always safe.
	select {
	case r.work <- fn:
	default:
		r.logger.Warn().Msg("dropped lua work item: dispatcher queue full")
	}
}

// submitSync runs fn on the dispatcher and blocks until it finishes.
// Panics inside fn surface as errors so the dispatcher cannot crash.
func (r *Runtime) submitSync(fn func(*glua.LState) error) error {
	if r == nil {
		return fmt.Errorf("lua runtime is nil")
	}

	type result struct {
		err error
	}
	// Buffered so wrapped completes even if the caller times out below.
	done := make(chan result, 1)

	wrapped := func(state *glua.LState) {
		defer func() {
			if rec := recover(); rec != nil {
				done <- result{err: fmt.Errorf("lua dispatcher panic: %v\n%s", rec, debug.Stack())}
			}
		}()
		done <- result{err: fn(state)}
	}

	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return fmt.Errorf("lua runtime is closed")
	}
	r.mu.Unlock()

	// Block (don't drop) until the dispatcher accepts or shutdown wins.
	select {
	case r.work <- wrapped:
	case <-r.ctx.Done():
		return fmt.Errorf("lua runtime is closed")
	}

	select {
	case res := <-done:
		return res.err
	case <-r.ctx.Done():
		return fmt.Errorf("lua runtime closed before work completed")
	}
}

// LoadEntrypoint executes the plugin entry file on the dispatcher and
// returns the function it must yield as its single return value.
func (r *Runtime) LoadEntrypoint(path string) (*glua.LFunction, error) {
	var entrypoint *glua.LFunction
	err := r.submitSync(func(state *glua.LState) error {
		base := state.GetTop()
		if err := state.DoFile(path); err != nil {
			return fmt.Errorf("load lua plugin %q: %w", path, err)
		}

		returned := state.GetTop() - base
		if returned != 1 {
			state.Pop(returned)
			return fmt.Errorf("lua plugin %q must return exactly one function", path)
		}

		fn, ok := state.Get(-1).(*glua.LFunction)
		state.Pop(1)
		if !ok {
			return fmt.Errorf("lua plugin %q must return a function", path)
		}

		entrypoint = fn
		return nil
	})
	if err != nil {
		return nil, err
	}
	return entrypoint, nil
}

// CallEntrypoint invokes the plugin entry function on the dispatcher in
// protected mode, passing the global `hive` table as its single argument.
func (r *Runtime) CallEntrypoint(entrypoint *glua.LFunction) error {
	return r.submitSync(func(state *glua.LState) error {
		hive, ok := state.GetGlobal("hive").(*glua.LTable)
		if !ok {
			return fmt.Errorf("internal error: hive table missing from lua runtime")
		}
		return state.CallByParam(glua.P{
			Fn:      entrypoint,
			NRet:    0,
			Protect: true,
		}, hive)
	})
}

// Close stops the dispatcher and releases the LState. Idempotent and
// safe across goroutines.
func (r *Runtime) Close() {
	if r == nil {
		return
	}

	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return
	}
	r.closed = true
	r.mu.Unlock()

	// ctx signals shutdown — the work channel is never closed because
	// late producers can still race a send (see Submit).
	r.cancel()

	r.wg.Wait()

	if r.state != nil {
		r.state.Close()
		r.state = nil
	}
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
