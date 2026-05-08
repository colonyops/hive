package lua

import (
	"context"
	"fmt"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/rs/zerolog"
	glua "github.com/yuin/gopher-lua"
)

// tickerMinInterval floors every/after; sub-second intervals raise a
// Lua error rather than clamp silently.
const tickerMinInterval = time.Second

// tickerRegistryKeyPrefix namespaces the per-module sub-table in the Lua
// registry. The instance pointer is appended so tests with multiple
// modules on one LState don't stomp each other.
const tickerRegistryKeyPrefix = "hive.ticker."

// tickerHandleMetatableName keys the metatable shared by every handle.
const tickerHandleMetatableName = "hive.ticker.handle"

// TickerModule exposes hive.ticker.every and hive.ticker.after, returning
// userdata handles with a :cancel() method. Ticker goroutines route
// callbacks through Runtime.Submit; they never touch the LState directly.
type TickerModule struct {
	Runtime    *Runtime
	PluginName string
	Logger     zerolog.Logger

	// rootCtx fans out to every per-handle context; cancelling it during
	// Close stops every running ticker.
	rootCtx    context.Context
	rootCancel context.CancelFunc

	wg sync.WaitGroup

	// nextID is also the per-handle registry sub-key, stringified.
	nextID atomic.Uint64

	mu      sync.Mutex
	handles map[uint64]*tickerHandle

	registryKey string
}

// tickerHandle is the Go-side state behind the userdata returned to Lua.
// Cancel is idempotent; the per-handle ctx stops the goroutine and the
// registry slot is released via Submit.
type tickerHandle struct {
	id     uint64
	cancel context.CancelFunc
	once   sync.Once
	module *TickerModule
}

// Register attaches every/after to a fresh hive.ticker subtable and
// creates this module's registry sub-table for pinning callback functions.
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

// Close stops every ticker goroutine and clears the handle map. Idempotent.
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

// ensureHandleMetatable installs the metatable shared by every handle.
// One metatable per module means cancel allocates only once.
func (m *TickerModule) ensureHandleMetatable(state *glua.LState) {
	mt := state.NewTypeMetatable(tickerHandleMetatableName)
	state.SetField(mt, "__index", state.SetFuncs(state.NewTable(), map[string]glua.LGFunction{
		"cancel": m.luaCancel,
	}))
}

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

// luaAfter fires once then auto-cancels, releasing the registry slot
// and map entry without an explicit cancel from the plugin.
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

// luaCancel is the __index method bound to handle userdata.
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

// checkArgs parses (duration, fn). On bad input it raises a Lua error,
// which longjmps out of the calling C function — there is no error return.
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

// spawn pins fn, allocates a handle, and starts run on a goroutine.
// Runs on the dispatcher; the mutex guards against goroutine-side
// Cancel via the after-fire path.
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

// fire hops from a ticker goroutine onto the dispatcher to run the
// callback. If the runtime is closed, Submit drops the work.
func (m *TickerModule) fire(h *tickerHandle) {
	m.Runtime.Submit(func(state *glua.LState) {
		fn := m.loadFunction(state, h.id)
		if fn == nil {
			// Cancelled between fire and dispatch.
			return
		}
		if err := state.CallByParam(glua.P{
			Fn:      fn,
			NRet:    0,
			Protect: true,
		}); err != nil {
			m.Logger.Warn().
				Uint64("handle", h.id).
				Err(err).
				Msg("ticker callback returned error")
		}
	})
}

// Cancel stops the goroutine, removes the map entry, and releases the
// registry slot via Submit. Idempotent and safe from any goroutine.
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

// newHandleUserData wraps a handle in *LUserData with the shared metatable.
// Must run on the dispatcher.
func (m *TickerModule) newHandleUserData(state *glua.LState, h *tickerHandle) *glua.LUserData {
	ud := state.NewUserData()
	ud.Value = h
	state.SetMetatable(ud, state.GetTypeMetatable(tickerHandleMetatableName))
	return ud
}

// storeFunction pins fn so Lua cannot GC it while the goroutine holds
// a reference. Must run on the dispatcher.
func (m *TickerModule) storeFunction(state *glua.LState, id uint64, fn *glua.LFunction) {
	table := m.registryTable(state)
	if table == nil {
		return
	}
	state.SetField(table, strconv.FormatUint(id, 10), fn)
}

// loadFunction returns the pinned function for a handle, or nil if
// already released. Cancel and load both run on the dispatcher.
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

// releaseFunction drops the registry pin so Lua can GC the LFunction.
// Must run on the dispatcher.
func (m *TickerModule) releaseFunction(state *glua.LState, id uint64) {
	table := m.registryTable(state)
	if table == nil {
		return
	}
	state.SetField(table, strconv.FormatUint(id, 10), glua.LNil)
}

// registryTable returns this module's pinning sub-table, or nil if
// Register has not run.
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
