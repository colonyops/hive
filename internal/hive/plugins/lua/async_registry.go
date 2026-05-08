package lua

import (
	"context"
	"fmt"
	"strconv"
	"sync"
	"sync/atomic"

	glua "github.com/yuin/gopher-lua"
)

// asyncRegistry holds the Lua-side and Go-side bookkeeping shared by
// every async host module (hive.ticker, hive.sh, ...). Each module owns
// one registry, initialises it during Register, and tears it down via
// shutdown.
//
// The registry handles:
//
//   - A per-module sub-table in the Lua registry that pins LFunctions
//     so callbacks survive Lua GC while a goroutine still references
//     them.
//   - A handle map keyed by id, storing the per-handle context.CancelFunc
//     so any goroutine can cancel a specific running operation.
//   - A root context whose cancellation fans out to every per-handle
//     context, used for module shutdown.
//   - A WaitGroup that lets shutdown block until in-flight goroutines
//     finish.
//   - The shared __index metatable installed on every handle userdata.
//
// Module code stays small: allocate to start a new operation, loadFunction
// to invoke its callback on the dispatcher, release after a successful
// dispatch, and stop or asyncHandle.Cancel for any-goroutine cancellation.
type asyncRegistry struct {
	cfg asyncRegistryConfig

	// registryKey namespaces the per-module sub-table in the Lua
	// registry. The instance pointer is appended so multiple modules of
	// the same kind on one LState don't stomp each other.
	registryKey string

	// nextID is also the per-handle registry sub-key, stringified.
	nextID atomic.Uint64

	mu      sync.Mutex
	handles map[uint64]context.CancelFunc

	rootCtx    context.Context
	rootCancel context.CancelFunc

	wg sync.WaitGroup

	closeOnce sync.Once
}

// asyncRegistryConfig configures the namespacing and Lua-facing names
// for an asyncRegistry. KeyPrefix should end in a "." for readability.
type asyncRegistryConfig struct {
	// KeyPrefix prefixes the per-module Lua registry sub-table key.
	// Example: "hive.ticker."
	KeyPrefix string
	// MetatableName names the Lua metatable installed on every handle
	// userdata. Example: "hive.ticker.handle"
	MetatableName string
}

// asyncHandle is the Go-side state behind every async userdata returned
// to Lua. Cancel is idempotent, safe from any goroutine, and routes
// through the registry to delete the map entry, cancel the per-handle
// context, and release the registry pin.
type asyncHandle struct {
	id       uint64
	once     sync.Once
	registry *asyncRegistry
	runtime  *Runtime
}

// init prepares the registry for use. modulePtr is fmt-formatted with
// %p to make registryKey unique per module instance (so two modules of
// the same kind on one LState don't collide). Must run on the
// dispatcher.
func (r *asyncRegistry) init(state *glua.LState, modulePtr any, cfg asyncRegistryConfig) error {
	r.cfg = cfg
	r.handles = map[uint64]context.CancelFunc{}
	r.rootCtx, r.rootCancel = context.WithCancel(context.Background())
	r.registryKey = fmt.Sprintf("%s%p", cfg.KeyPrefix, modulePtr)

	registry, ok := state.Get(glua.RegistryIndex).(*glua.LTable)
	if !ok {
		return fmt.Errorf("%slua registry is not a table", cfg.KeyPrefix)
	}
	state.SetField(registry, r.registryKey, state.NewTable())

	mt := state.NewTypeMetatable(cfg.MetatableName)
	state.SetField(mt, "__index", state.SetFuncs(state.NewTable(), map[string]glua.LGFunction{
		"cancel": r.luaCancel,
	}))
	return nil
}

// shutdown cancels rootCtx, waits for goroutines to drain, and clears
// the handle map. Idempotent.
func (r *asyncRegistry) shutdown() {
	r.closeOnce.Do(func() {
		if r.rootCancel != nil {
			r.rootCancel()
		}
		r.wg.Wait()

		r.mu.Lock()
		r.handles = map[uint64]context.CancelFunc{}
		r.mu.Unlock()
	})
}

// Go starts fn in a goroutine tracked by the registry's WaitGroup so
// shutdown blocks until fn completes. Mirrors sync.WaitGroup.Go;
// capitalised because lowercase `go` is a reserved keyword.
func (r *asyncRegistry) Go(fn func()) { r.wg.Go(fn) }

// allocate pins fn under a fresh handle id, derives a per-handle context
// from rootCtx, and records the cancel func in the handle map. Returns
// the id and ctx for the goroutine to bind to. Must run on the
// dispatcher.
func (r *asyncRegistry) allocate(state *glua.LState, fn *glua.LFunction) (uint64, context.Context) {
	id := r.nextID.Add(1)
	ctx, cancel := context.WithCancel(r.rootCtx)

	r.storeFunction(state, id, fn)

	r.mu.Lock()
	r.handles[id] = cancel
	r.mu.Unlock()

	return id, ctx
}

// loadFunction returns the pinned function for id, or nil if already
// released. Must run on the dispatcher.
func (r *asyncRegistry) loadFunction(state *glua.LState, id uint64) *glua.LFunction {
	table := r.registryTable(state)
	if table == nil {
		return nil
	}
	fn, ok := state.GetField(table, strconv.FormatUint(id, 10)).(*glua.LFunction)
	if !ok {
		return nil
	}
	return fn
}

// release drops the handle map entry and registry pin for id. Used after
// a dispatch path has consumed the callback. Safe even if id is unknown.
// Must run on the dispatcher.
func (r *asyncRegistry) release(state *glua.LState, id uint64) {
	r.mu.Lock()
	delete(r.handles, id)
	r.mu.Unlock()

	r.releaseFunction(state, id)
}

// stop cancels the per-handle context, removes the map entry, and
// schedules the registry pin release on the dispatcher. Safe from any
// goroutine; a missing id is treated as already released.
func (r *asyncRegistry) stop(rt *Runtime, id uint64) {
	r.mu.Lock()
	cancel, ok := r.handles[id]
	if ok {
		delete(r.handles, id)
	}
	r.mu.Unlock()
	if !ok {
		return
	}
	cancel()

	rt.Submit(func(state *glua.LState) {
		r.releaseFunction(state, id)
	})
}

// handleUserData wraps value in *LUserData with the registry's metatable.
// Must run on the dispatcher.
func (r *asyncRegistry) handleUserData(state *glua.LState, value any) *glua.LUserData {
	ud := state.NewUserData()
	ud.Value = value
	state.SetMetatable(ud, state.GetTypeMetatable(r.cfg.MetatableName))
	return ud
}

// newHandle returns a fresh asyncHandle wired to this registry and the
// supplied runtime. Pair with allocate to get the id.
func (r *asyncRegistry) newHandle(id uint64, rt *Runtime) *asyncHandle {
	return &asyncHandle{id: id, registry: r, runtime: rt}
}

// luaCancel is the __index method bound to handle userdata. Type-asserts
// the userdata as *asyncHandle (which both ticker and sh use) and routes
// to Cancel.
func (r *asyncRegistry) luaCancel(state *glua.LState) int {
	ud := state.CheckUserData(1)
	h, ok := ud.Value.(*asyncHandle)
	if !ok {
		state.ArgError(1, "expected "+r.cfg.MetatableName+" handle")
		return 0
	}
	h.Cancel()
	return 0
}

// storeFunction pins fn so Lua cannot GC it while the goroutine holds a
// reference. Must run on the dispatcher.
func (r *asyncRegistry) storeFunction(state *glua.LState, id uint64, fn *glua.LFunction) {
	table := r.registryTable(state)
	if table == nil {
		return
	}
	state.SetField(table, strconv.FormatUint(id, 10), fn)
}

// releaseFunction drops the registry pin so Lua can GC the LFunction.
// Must run on the dispatcher.
func (r *asyncRegistry) releaseFunction(state *glua.LState, id uint64) {
	table := r.registryTable(state)
	if table == nil {
		return
	}
	state.SetField(table, strconv.FormatUint(id, 10), glua.LNil)
}

// registryTable returns this registry's pinning sub-table, or nil if
// init has not run.
func (r *asyncRegistry) registryTable(state *glua.LState) *glua.LTable {
	registry, ok := state.Get(glua.RegistryIndex).(*glua.LTable)
	if !ok {
		return nil
	}
	table, ok := state.GetField(registry, r.registryKey).(*glua.LTable)
	if !ok {
		return nil
	}
	return table
}

// Cancel stops the per-handle goroutine, removes the map entry, and
// schedules the registry pin release on the dispatcher. Idempotent and
// safe from any goroutine.
func (h *asyncHandle) Cancel() {
	h.once.Do(func() {
		h.registry.stop(h.runtime, h.id)
	})
}

// poison consumes the once so a later Cancel is a true no-op. Used by
// post-dispatch cleanup paths that already released via the dispatcher
// and want to suppress redundant Cancel work.
func (h *asyncHandle) poison() {
	h.once.Do(func() {})
}
