package lua

import (
	"context"
	"fmt"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/rs/zerolog/log"
	glua "github.com/yuin/gopher-lua"
)

// tickerMinInterval is the smallest interval a plugin may request via
// hive.ticker.every / hive.ticker.after. The floor exists to keep runaway
// plugins from spamming the dispatcher queue; we surface the violation as a
// Lua error rather than silently clamping so authors notice.
const tickerMinInterval = time.Second

// tickerRegistryKeyPrefix namespaces the per-module sub-table inside the Lua
// registry. The instance pointer is appended so multiple TickerModules on the
// same LState (in tests, etc.) cannot stomp each other.
const tickerRegistryKeyPrefix = "hive.ticker."

// tickerHandleMetatableName is the registry key for the metatable shared by
// every handle userdata produced by this module.
const tickerHandleMetatableName = "hive.ticker.handle"

// TickerModule exposes `hive.ticker.every` and `hive.ticker.after`, returning
// userdata handles with a `:cancel()` method. All goroutines spawned by the
// module bottom out at the runtime's dispatcher via Runtime.Submit so they
// never touch the LState directly.
type TickerModule struct {
	Runtime    *Runtime
	PluginName string

	// rootCtx fans out to every per-handle context; cancelling it during Close
	// stops every running ticker without per-handle bookkeeping.
	rootCtx    context.Context
	rootCancel context.CancelFunc

	// wg tracks live ticker/timer goroutines so Close can join them.
	wg sync.WaitGroup

	// nextID hands out monotonically increasing handle IDs (also used as the
	// registry sub-key, stringified).
	nextID atomic.Uint64

	mu      sync.Mutex
	handles map[uint64]*tickerHandle

	// registryKey is the key under which this module's function-pinning
	// sub-table lives inside the Lua registry. Populated in Register.
	registryKey string
}

// tickerHandle is the Go-side state behind the userdata returned to Lua.
// Cancel is idempotent via once; the per-handle context cancel is what stops
// the goroutine, and the registry slot is cleared via Submit so registry
// mutations stay on the dispatcher goroutine.
type tickerHandle struct {
	id     uint64
	cancel context.CancelFunc
	once   sync.Once
	module *TickerModule
}

// Register attaches `every` and `after` to a fresh `hive.ticker` subtable and
// installs the per-instance registry sub-table used to pin Lua callback
// functions for the lifetime of their handles.
func (m *TickerModule) Register(state *glua.LState, hive *glua.LTable) error {
	m.handles = map[uint64]*tickerHandle{}
	m.rootCtx, m.rootCancel = context.WithCancel(context.Background())
	m.registryKey = fmt.Sprintf("%s%p", tickerRegistryKeyPrefix, m)

	registry, ok := state.Get(glua.RegistryIndex).(*glua.LTable)
	if !ok {
		return fmt.Errorf("ticker module: lua registry is not a table")
	}
	state.SetField(registry, m.registryKey, state.NewTable())

	m.ensureHandleMetatable(state)

	ticker := state.NewTable()
	state.SetField(ticker, "every", state.NewFunction(m.luaEvery))
	state.SetField(ticker, "after", state.NewFunction(m.luaAfter))
	state.SetField(hive, "ticker", ticker)
	return nil
}

// Close stops every running ticker/timer goroutine and clears the handle map.
// It is safe to call multiple times; the second call is a no-op because
// rootCancel is idempotent and the map is already empty.
func (m *TickerModule) Close() error {
	if m.rootCancel != nil {
		m.rootCancel()
	}
	m.wg.Wait()

	m.mu.Lock()
	m.handles = map[uint64]*tickerHandle{}
	m.mu.Unlock()
	return nil
}

// ensureHandleMetatable installs the metatable used by every handle userdata.
// Sharing one metatable across handles keeps construction cheap and means the
// `cancel` Go closure only allocates once per module.
func (m *TickerModule) ensureHandleMetatable(state *glua.LState) {
	mt := state.NewTypeMetatable(tickerHandleMetatableName)
	state.SetField(mt, "__index", state.SetFuncs(state.NewTable(), map[string]glua.LGFunction{
		"cancel": m.luaCancel,
	}))
}

// luaEvery implements hive.ticker.every(duration, fn).
func (m *TickerModule) luaEvery(state *glua.LState) int {
	d, fn := m.checkArgs(state)
	handle := m.spawn(state, fn, func(ctx context.Context, h *tickerHandle) {
		t := time.NewTicker(d)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				m.fire(h)
			}
		}
	})
	state.Push(m.newHandleUserData(state, handle))
	return 1
}

// luaAfter implements hive.ticker.after(duration, fn). The goroutine fires
// exactly once, then auto-cancels so the registry slot and map entry are
// released without the plugin having to call cancel explicitly.
func (m *TickerModule) luaAfter(state *glua.LState) int {
	d, fn := m.checkArgs(state)
	handle := m.spawn(state, fn, func(ctx context.Context, h *tickerHandle) {
		t := time.NewTimer(d)
		defer t.Stop()
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			m.fire(h)
			h.Cancel()
		}
	})
	state.Push(m.newHandleUserData(state, handle))
	return 1
}

// luaCancel is the Lua-callable __index method on handle userdata.
func (m *TickerModule) luaCancel(state *glua.LState) int {
	ud := state.CheckUserData(1)
	handle, ok := ud.Value.(*tickerHandle)
	if !ok {
		state.ArgError(1, "expected ticker handle")
		return 0
	}
	handle.Cancel()
	return 0
}

// checkArgs validates and parses the (duration, fn) argument pair shared by
// every and after. It raises a Lua error on bad input, which longjmps out of
// the calling C function — callers should treat its return as the only path
// that survives.
func (m *TickerModule) checkArgs(state *glua.LState) (time.Duration, *glua.LFunction) {
	durationStr := state.CheckString(1)
	fn := state.CheckFunction(2)

	d, err := time.ParseDuration(durationStr)
	if err != nil {
		state.ArgError(1, fmt.Sprintf("invalid duration %q: %s", durationStr, err.Error()))
	}
	if d < tickerMinInterval {
		state.ArgError(1, fmt.Sprintf("duration must be at least %s, got %s", tickerMinInterval, d))
	}
	return d, fn
}

// spawn pins fn in the registry, allocates a handle, and starts the supplied
// run loop. It runs on the dispatcher goroutine, so registry and map writes
// here are race-free with each other (but still need the mutex against
// goroutine-side reads via Cancel-from-after).
func (m *TickerModule) spawn(state *glua.LState, fn *glua.LFunction, run func(context.Context, *tickerHandle)) *tickerHandle {
	id := m.nextID.Add(1)
	ctx, cancel := context.WithCancel(m.rootCtx)
	h := &tickerHandle{id: id, cancel: cancel, module: m}

	m.storeFunction(state, id, fn)

	m.mu.Lock()
	m.handles[id] = h
	m.mu.Unlock()

	m.wg.Go(func() {
		run(ctx, h)
	})
	return h
}

// fire is invoked from a ticker/timer goroutine. It hops onto the dispatcher
// before touching the LState; if the runtime is closed Submit drops the work
// and the goroutine exits via ctx cancellation on the next select.
func (m *TickerModule) fire(h *tickerHandle) {
	m.Runtime.Submit(func(state *glua.LState) {
		fn := m.loadFunction(state, h.id)
		if fn == nil {
			// Handle was cancelled between fire and dispatch; nothing to do.
			return
		}
		if err := state.CallByParam(glua.P{
			Fn:      fn,
			NRet:    0,
			Protect: true,
		}); err != nil {
			log.Warn().
				Str("plugin", m.PluginName).
				Uint64("handle", h.id).
				Err(err).
				Msg("ticker callback returned error")
		}
	})
}

// Cancel stops the goroutine, removes the map entry, and clears the registry
// slot. Safe to call from any goroutine and idempotent under sync.Once. The
// registry release hops onto the dispatcher because the registry must only be
// touched by the dispatcher goroutine.
func (h *tickerHandle) Cancel() {
	h.once.Do(func() {
		h.cancel()

		h.module.mu.Lock()
		delete(h.module.handles, h.id)
		h.module.mu.Unlock()

		id := h.id
		mod := h.module
		mod.Runtime.Submit(func(state *glua.LState) {
			mod.releaseFunction(state, id)
		})
	})
}

// newHandleUserData wraps a handle in *LUserData with the shared metatable
// installed in Register. It must be called on the dispatcher goroutine.
func (m *TickerModule) newHandleUserData(state *glua.LState, h *tickerHandle) *glua.LUserData {
	ud := state.NewUserData()
	ud.Value = h
	state.SetMetatable(ud, state.GetTypeMetatable(tickerHandleMetatableName))
	return ud
}

// storeFunction pins fn under the handle ID inside this module's registry
// sub-table, preventing Lua GC from collecting it while the goroutine is
// alive. Must run on the dispatcher goroutine.
func (m *TickerModule) storeFunction(state *glua.LState, id uint64, fn *glua.LFunction) {
	table := m.registryTable(state)
	if table == nil {
		return
	}
	state.SetField(table, strconv.FormatUint(id, 10), fn)
}

// loadFunction retrieves the pinned LFunction for a handle. Returns nil when
// the handle has already been released, which races naturally with cancel
// because both run on the dispatcher.
func (m *TickerModule) loadFunction(state *glua.LState, id uint64) *glua.LFunction {
	table := m.registryTable(state)
	if table == nil {
		return nil
	}
	value := state.GetField(table, strconv.FormatUint(id, 10))
	fn, ok := value.(*glua.LFunction)
	if !ok {
		return nil
	}
	return fn
}

// releaseFunction drops the registry pin so the LFunction becomes eligible for
// Lua GC. Must run on the dispatcher goroutine.
func (m *TickerModule) releaseFunction(state *glua.LState, id uint64) {
	table := m.registryTable(state)
	if table == nil {
		return
	}
	state.SetField(table, strconv.FormatUint(id, 10), glua.LNil)
}

// registryTable returns this module's pinning sub-table from the Lua
// registry, or nil if Register has not been called yet. Must run on the
// dispatcher goroutine.
func (m *TickerModule) registryTable(state *glua.LState) *glua.LTable {
	registry, ok := state.Get(glua.RegistryIndex).(*glua.LTable)
	if !ok {
		return nil
	}
	table, ok := state.GetField(registry, m.registryKey).(*glua.LTable)
	if !ok {
		return nil
	}
	return table
}
