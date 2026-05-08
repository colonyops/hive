package lua

import (
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
	glua "github.com/yuin/gopher-lua"
)

// testPluginName is the plugin name registered with LogModule and
// PluginInfoModule in test harnesses. Tests that exercise kv.Scoped
// namespacing pass their own name to KVModule.
const testPluginName = "lua-test"

// captureModule exposes test-only capture helpers in the Lua hive table:
//
//	hive.test_capture(value)        — appends value to a positional list
//	hive.test_capture(key, value)   — stores value under key
//	hive.test_bump()                — increments a counter
//
// All three are safe to call from any goroutine; the unified type replaces
// the per-test capture/bump modules that used to live alongside each
// module's tests.
type captureModule struct {
	mu      sync.Mutex
	list    []glua.LValue
	keyed   map[string]glua.LValue
	counter atomic.Int64
}

func newCaptureModule() *captureModule {
	return &captureModule{keyed: map[string]glua.LValue{}}
}

func (c *captureModule) Register(state *glua.LState, hive *glua.LTable) error {
	state.SetField(hive, "test_capture", state.NewFunction(c.luaCapture))
	state.SetField(hive, "test_bump", state.NewFunction(c.luaBump))
	return nil
}

func (c *captureModule) luaCapture(state *glua.LState) int {
	if state.GetTop() == 1 {
		v := state.CheckAny(1)
		c.mu.Lock()
		c.list = append(c.list, v)
		c.mu.Unlock()
		return 0
	}
	key := state.CheckString(1)
	val := state.CheckAny(2)
	c.mu.Lock()
	c.keyed[key] = val
	c.mu.Unlock()
	return 0
}

func (c *captureModule) luaBump(_ *glua.LState) int {
	c.counter.Add(1)
	return 0
}

// Snapshot returns a copy of the positional capture list.
func (c *captureModule) Snapshot() []glua.LValue {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]glua.LValue, len(c.list))
	copy(out, c.list)
	return out
}

// Get returns the value captured under key, or LNil if the key is unset.
func (c *captureModule) Get(key string) glua.LValue {
	c.mu.Lock()
	defer c.mu.Unlock()
	if v, ok := c.keyed[key]; ok {
		return v
	}
	return glua.LNil
}

// Has reports whether key was captured.
func (c *captureModule) Has(key string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	_, ok := c.keyed[key]
	return ok
}

// String returns the keyed capture as a Go string. Returns "" if the key
// is absent or holds a non-string value.
func (c *captureModule) String(key string) string {
	if v, ok := c.Get(key).(glua.LString); ok {
		return string(v)
	}
	return ""
}

// Bool returns the keyed capture as a Go bool. Returns false if the key
// is absent or holds a non-boolean value.
func (c *captureModule) Bool(key string) bool {
	if v, ok := c.Get(key).(glua.LBool); ok {
		return bool(v)
	}
	return false
}

// Number returns the keyed capture as a float64. Returns 0 if the key
// is absent or holds a non-numeric value.
func (c *captureModule) Number(key string) float64 {
	if v, ok := c.Get(key).(glua.LNumber); ok {
		return float64(v)
	}
	return 0
}

// Counter returns the current bump counter.
func (c *captureModule) Counter() int64 {
	return c.counter.Load()
}

// luaHarness wraps a Runtime configured with the standard test module set
// (LogModule, PluginInfoModule, CommandsModule, captureModule) plus any
// caller-supplied modules. The Lua entry script runs during construction;
// runtime and module shutdown are registered with t.Cleanup.
type luaHarness struct {
	runtime *Runtime
	capture *captureModule
	modules []HostModule
	root    string
	entry   string
	closed  atomic.Bool
}

// newLuaHarness writes script to t.TempDir()/init.lua, builds a Runtime
// with the standard test module set plus extras, runs the entrypoint, and
// registers cleanup on t.
//
// Modules requiring a Runtime reference (TickerModule.Runtime) are wired
// after NewRuntime returns and before LoadEntrypoint runs, mirroring
// Plugin.Init.
func newLuaHarness(t *testing.T, script string, extras ...HostModule) *luaHarness {
	t.Helper()

	root := t.TempDir()
	entry := filepath.Join(root, "init.lua")
	require.NoError(t, os.WriteFile(entry, []byte(script), 0o644))

	capture := newCaptureModule()
	modules := append([]HostModule{
		&LogModule{PluginName: testPluginName, Logger: zerolog.Nop()},
		&PluginInfoModule{Name: testPluginName, Entry: entry, ModuleRoot: root},
		&CommandsModule{},
		capture,
	}, extras...)

	rt, err := NewRuntime(root, zerolog.Nop(), modules...)
	require.NoError(t, err)

	for _, m := range extras {
		switch m := m.(type) {
		case *TickerModule:
			m.Runtime = rt
		case *ShModule:
			m.Runtime = rt
		}
	}

	h := &luaHarness{
		runtime: rt,
		capture: capture,
		modules: modules,
		root:    root,
		entry:   entry,
	}
	t.Cleanup(h.Close)

	fn, err := rt.LoadEntrypoint(entry)
	require.NoError(t, err)
	require.NoError(t, rt.CallEntrypoint(fn))

	return h
}

// Close shuts down host modules in reverse-registration order, then the
// runtime. Idempotent; safe to call from t.Cleanup and from tests that
// want explicit shutdown ordering.
func (h *luaHarness) Close() {
	if !h.closed.CompareAndSwap(false, true) {
		return
	}
	for i := len(h.modules) - 1; i >= 0; i-- {
		if c, ok := h.modules[i].(HostModuleCloser); ok {
			_ = c.Close()
		}
	}
	h.runtime.Close()
}
