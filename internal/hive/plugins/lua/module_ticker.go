package lua

import (
	"context"
	"fmt"
	"time"

	"github.com/rs/zerolog"
	glua "github.com/yuin/gopher-lua"
)

// tickerMinInterval floors every/after; sub-second intervals raise a
// Lua error rather than clamp silently.
const tickerMinInterval = time.Second

// TickerModule exposes hive.ticker.every and hive.ticker.after, returning
// userdata handles with a :cancel() method. Ticker goroutines route
// callbacks through Runtime.Submit; they never touch the LState directly.
type TickerModule struct {
	Runtime *Runtime
	Logger  zerolog.Logger

	registry asyncRegistry
}

// Register attaches every/after to a fresh hive.ticker subtable and
// initialises the per-module async registry.
func (m *TickerModule) Register(state *glua.LState, hive *glua.LTable) error {
	if err := m.registry.init(state, m, asyncRegistryConfig{
		KeyPrefix:     "hive.ticker.",
		MetatableName: "hive.ticker.handle",
	}); err != nil {
		return fmt.Errorf("ticker module: %w", err)
	}

	ticker := state.NewTable()
	state.SetField(ticker, "every", state.NewFunction(m.luaEvery))
	state.SetField(ticker, "after", state.NewFunction(m.luaAfter))
	state.SetField(hive, "ticker", ticker)
	return nil
}

// Close stops every ticker goroutine and clears the handle map. Idempotent.
func (m *TickerModule) Close() error {
	m.registry.shutdown()
	return nil
}

func (m *TickerModule) luaEvery(state *glua.LState) int {
	d, fn := m.checkArgs(state)
	handle := m.spawn(state, fn, func(ctx context.Context, h *asyncHandle) {
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
	m.Logger.Debug().Uint64("handle", handle.id).Str("kind", "every").Dur("interval", d).Msg("hive.ticker: handle registered")
	state.Push(m.registry.handleUserData(state, handle))
	return 1
}

// luaAfter fires once then auto-cancels, releasing the registry slot
// and map entry without an explicit cancel from the plugin.
func (m *TickerModule) luaAfter(state *glua.LState) int {
	d, fn := m.checkArgs(state)
	handle := m.spawn(state, fn, func(ctx context.Context, h *asyncHandle) {
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
	m.Logger.Debug().Uint64("handle", handle.id).Str("kind", "after").Dur("delay", d).Msg("hive.ticker: handle registered")
	state.Push(m.registry.handleUserData(state, handle))
	return 1
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
// Runs on the dispatcher.
func (m *TickerModule) spawn(state *glua.LState, fn *glua.LFunction, run func(context.Context, *asyncHandle)) *asyncHandle {
	id, ctx := m.registry.allocate(state, fn)
	h := m.registry.newHandle(id, m.Runtime)
	m.registry.Go(func() { run(ctx, h) })
	return h
}

// fire hops from a ticker goroutine onto the dispatcher to run the
// callback. If the runtime is closed, Submit drops the work.
func (m *TickerModule) fire(h *asyncHandle) {
	m.Runtime.Submit(func(state *glua.LState) {
		fn := m.registry.loadFunction(state, h.id)
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
