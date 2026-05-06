package lua

import (
	"os"
	"path/filepath"
	"runtime"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	glua "github.com/yuin/gopher-lua"
)

// bumpModule is a test-only HostModule that exposes `hive.test_bump()` for
// observing how many times a Lua callback executed on the dispatcher.
type bumpModule struct {
	counter *atomic.Int64
}

func (b *bumpModule) Register(state *glua.LState, hive *glua.LTable) error {
	state.SetField(hive, "test_bump", state.NewFunction(func(state *glua.LState) int {
		b.counter.Add(1)
		return 0
	}))
	return nil
}

// tickerHarness wires a Runtime + TickerModule + bumpModule together and runs
// the supplied Lua script as the entrypoint so the call to
// `hive.ticker.every` / `hive.ticker.after` happens on the dispatcher
// goroutine. The returned counter is bumped each time the Lua callback runs.
type tickerHarness struct {
	runtime *Runtime
	module  *TickerModule
	counter *atomic.Int64
}

func newTickerHarness(t *testing.T, script string) *tickerHarness {
	t.Helper()

	root := t.TempDir()
	entry := filepath.Join(root, "init.lua")
	require.NoError(t, os.WriteFile(entry, []byte(script), 0o644))

	counter := &atomic.Int64{}
	tickerModule := &TickerModule{PluginName: "lua-test"}
	bump := &bumpModule{counter: counter}

	rt, err := NewRuntime(
		root,
		&LogModule{PluginName: "lua-test"},
		&PluginInfoModule{Name: "lua-test", Entry: entry, ModuleRoot: root},
		&CommandsModule{},
		tickerModule,
		bump,
	)
	require.NoError(t, err)
	tickerModule.Runtime = rt

	fn, err := rt.LoadEntrypoint(entry)
	require.NoError(t, err)
	require.NoError(t, rt.CallEntrypoint(fn))

	return &tickerHarness{runtime: rt, module: tickerModule, counter: counter}
}

func (h *tickerHarness) Close() {
	// Mirror Plugin.shutdown(): close modules in reverse order before runtime.
	_ = h.module.Close()
	h.runtime.Close()
}

// closeWithCleanup wires harness shutdown into t.Cleanup so each test releases
// goroutines and the LState even on early failures.
func (h *tickerHarness) closeWithCleanup(t *testing.T) {
	t.Helper()
	t.Cleanup(h.Close)
}

func TestTickerEveryFiresRepeatedly(t *testing.T) {
	t.Parallel()

	h := newTickerHarness(t, `
return function(hive)
  hive.ticker.every("1s", function() hive.test_bump() end)
end
`)
	h.closeWithCleanup(t)

	time.Sleep(2500 * time.Millisecond)

	got := h.counter.Load()
	assert.GreaterOrEqual(t, got, int64(2), "ticker should fire at least twice in 2.5s")
}

func TestTickerAfterFiresOnce(t *testing.T) {
	t.Parallel()

	h := newTickerHarness(t, `
return function(hive)
  hive.ticker.after("1s", function() hive.test_bump() end)
end
`)
	h.closeWithCleanup(t)

	time.Sleep(2500 * time.Millisecond)

	got := h.counter.Load()
	assert.Equal(t, int64(1), got, "after should fire exactly once")
}

func TestTickerCancelStopsFurtherFires(t *testing.T) {
	t.Parallel()

	// The Lua callback cancels its own handle on the first fire, so we should
	// see exactly one bump regardless of how long we sleep afterwards.
	h := newTickerHarness(t, `
return function(hive)
  local handle
  handle = hive.ticker.every("1s", function()
    hive.test_bump()
    handle:cancel()
  end)
end
`)
	h.closeWithCleanup(t)

	// Wait long enough for the first fire plus several intervals afterwards
	// so a missed cancel would surface as additional bumps.
	time.Sleep(3 * time.Second)

	got := h.counter.Load()
	assert.Equal(t, int64(1), got, "cancel from inside callback must stop further fires")
}

func TestTickerCallbackErrorsKeepFiring(t *testing.T) {
	t.Parallel()

	// The callback bumps the counter and then raises a Lua error. The runtime
	// should log it and keep the ticker alive.
	h := newTickerHarness(t, `
return function(hive)
  hive.ticker.every("1s", function()
    hive.test_bump()
    error("boom")
  end)
end
`)
	h.closeWithCleanup(t)

	time.Sleep(2500 * time.Millisecond)

	got := h.counter.Load()
	assert.GreaterOrEqual(t, got, int64(2), "ticker must keep firing after callback errors")
}

func TestTickerCloseCancelsAllOutstandingTickers(t *testing.T) {
	// Intentionally NOT t.Parallel(): runtime.NumGoroutine is process-wide so
	// concurrent ticker tests would corrupt the before/after delta.

	// Allow the test process a moment to settle background goroutines from
	// the harness setup before we capture the baseline.
	time.Sleep(100 * time.Millisecond)
	before := runtime.NumGoroutine()

	h := newTickerHarness(t, `
return function(hive)
  hive.ticker.every("1s", function() hive.test_bump() end)
  hive.ticker.every("1s", function() hive.test_bump() end)
  hive.ticker.after("1s", function() hive.test_bump() end)
end
`)

	// Confirm tickers are actually running before we shut them down.
	time.Sleep(1200 * time.Millisecond)
	require.Positive(t, h.counter.Load(), "tickers should have fired before close")

	h.Close()

	// Give the runtime a tick to fully unwind goroutines.
	time.Sleep(200 * time.Millisecond)
	after := runtime.NumGoroutine()

	// runtime.NumGoroutine is inherently noisy with -parallel; allow a small
	// margin but flag any obvious leak (the 3 ticker goroutines + dispatcher).
	assert.LessOrEqual(t, after, before+2,
		"goroutine count should not grow appreciably after Close (before=%d, after=%d)", before, after)
}

func TestTickerEveryRejectsSubSecondDuration(t *testing.T) {
	t.Parallel()

	// The Lua entrypoint uses pcall and stashes the error message in a global
	// so we can inspect it from Go via a follow-up bump-style probe. If the
	// duration is rejected, no ticker is registered and the counter must stay
	// at zero even after we sleep past the would-be interval.
	root := t.TempDir()
	entry := filepath.Join(root, "init.lua")
	script := `
return function(hive)
  local ok, err = pcall(hive.ticker.every, "500ms", function() hive.test_bump() end)
  if ok then
    error("expected sub-second duration to be rejected")
  end
  if not string.find(tostring(err), "duration must be at least") then
    error("unexpected error: " .. tostring(err))
  end
end
`
	require.NoError(t, os.WriteFile(entry, []byte(script), 0o644))

	counter := &atomic.Int64{}
	tickerModule := &TickerModule{PluginName: "lua-test"}
	rt, err := NewRuntime(
		root,
		&LogModule{PluginName: "lua-test"},
		&PluginInfoModule{Name: "lua-test", Entry: entry, ModuleRoot: root},
		&CommandsModule{},
		tickerModule,
		&bumpModule{counter: counter},
	)
	require.NoError(t, err)
	tickerModule.Runtime = rt
	t.Cleanup(func() {
		_ = tickerModule.Close()
		rt.Close()
	})

	fn, err := rt.LoadEntrypoint(entry)
	require.NoError(t, err)
	require.NoError(t, rt.CallEntrypoint(fn))

	// Wait past 500ms to confirm no ticker was registered.
	time.Sleep(1200 * time.Millisecond)
	assert.Equal(t, int64(0), counter.Load(), "no ticker should have been registered")
}
