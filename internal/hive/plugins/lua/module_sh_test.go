package lua

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	glua "github.com/yuin/gopher-lua"

	"github.com/colonyops/hive/internal/hive/plugins"
)

// fakeExecutor implements ShellExecutor with caller-provided behaviour.
type fakeExecutor struct {
	mu sync.Mutex

	respond func(ctx context.Context, cmd string, opts shOptions) shResult

	lastCmd  string
	lastOpts shOptions
	lastCtx  context.Context
	calls    int
}

func (f *fakeExecutor) Exec(ctx context.Context, cmd string, opts shOptions) shResult {
	respond := f.respond
	f.mu.Lock()
	f.lastCmd = cmd
	f.lastOpts = opts
	f.lastCtx = ctx
	f.calls++
	f.mu.Unlock()
	if respond == nil {
		return shResult{}
	}
	return respond(ctx, cmd, opts)
}

func (f *fakeExecutor) snapshot() (cmd string, opts shOptions, ctx context.Context, calls int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.lastCmd, f.lastOpts, f.lastCtx, f.calls
}

// shHarness wires a Runtime + ShModule + a result-capturing module into a
// Lua entry script. Tests inject a fakeExecutor to drive the executor side
// without spawning subprocesses.
type shHarness struct {
	module   *ShModule
	executor *fakeExecutor
	captured *shCaptureModule
}

// shCaptureModule exposes hive.test_capture(value) so Lua can hand
// arbitrary values back to the test for assertion. Distinct from the
// keyed captureModule used by module_json_test.go because hive.sh tests
// only need a positional list.
type shCaptureModule struct {
	mu     sync.Mutex
	values []glua.LValue
}

func (c *shCaptureModule) Register(state *glua.LState, hive *glua.LTable) error {
	state.SetField(hive, "test_capture", state.NewFunction(func(state *glua.LState) int {
		v := state.CheckAny(1)
		c.mu.Lock()
		c.values = append(c.values, v)
		c.mu.Unlock()
		return 0
	}))
	return nil
}

func (c *shCaptureModule) snapshot() []glua.LValue {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]glua.LValue, len(c.values))
	copy(out, c.values)
	return out
}

func newShHarness(t *testing.T, script string, executor *fakeExecutor, defaultTimeout time.Duration) *shHarness {
	t.Helper()

	root := t.TempDir()
	entry := filepath.Join(root, "init.lua")
	require.NoError(t, os.WriteFile(entry, []byte(script), 0o644))

	pool := plugins.NewWorkerPool(1)
	module := &ShModule{
		PluginName:     "lua-test",
		Pool:           pool,
		Executor:       executor,
		DefaultTimeout: defaultTimeout,
	}
	captured := &shCaptureModule{}

	rt, err := NewRuntime(
		root,
		&LogModule{PluginName: "lua-test"},
		&PluginInfoModule{Name: "lua-test", Entry: entry, ModuleRoot: root},
		&CommandsModule{},
		module,
		captured,
	)
	require.NoError(t, err)

	fn, err := rt.LoadEntrypoint(entry)
	require.NoError(t, err)
	require.NoError(t, rt.CallEntrypoint(fn))

	t.Cleanup(func() {
		_ = module.Close()
		rt.Close()
	})

	return &shHarness{
		module:   module,
		executor: executor,
		captured: captured,
	}
}

func TestShRun_ExitCodes(t *testing.T) {
	t.Parallel()

	exec := &fakeExecutor{
		respond: func(_ context.Context, cmd string, _ shOptions) shResult {
			switch cmd {
			case "ok":
				return shResult{Code: 0}
			case "fail":
				return shResult{Code: 1}
			case "weird":
				return shResult{Code: 7}
			}
			t.Fatalf("unexpected cmd %q", cmd)
			return shResult{}
		},
	}

	h := newShHarness(t, `
return function(hive)
  hive.test_capture(hive.sh.run("ok"))
  hive.test_capture(hive.sh.run("fail"))
  hive.test_capture(hive.sh.run("weird"))
end
`, exec, 0)

	values := h.captured.snapshot()
	require.Len(t, values, 3)
	assert.Equal(t, 0, asLuaInt(t, values[0]))
	assert.Equal(t, 1, asLuaInt(t, values[1]))
	assert.Equal(t, 7, asLuaInt(t, values[2]))
}

// asLuaInt unwraps an LNumber to int for exact comparison without
// triggering the testifylint float-compare rule.
func asLuaInt(t *testing.T, v glua.LValue) int {
	t.Helper()
	n, ok := v.(glua.LNumber)
	require.True(t, ok, "expected LNumber, got %T", v)
	return int(n)
}

func TestShOutput_StripsTrailingNewline(t *testing.T) {
	t.Parallel()

	exec := &fakeExecutor{
		respond: func(_ context.Context, _ string, _ shOptions) shResult {
			return shResult{Stdout: "hello\n", Code: 0}
		},
	}

	h := newShHarness(t, `
return function(hive)
  hive.test_capture(hive.sh.output("anything"))
end
`, exec, 0)

	values := h.captured.snapshot()
	require.Len(t, values, 1)
	assert.Equal(t, glua.LString("hello"), values[0])
}

func TestShOutput_NonZeroRaises(t *testing.T) {
	t.Parallel()

	exec := &fakeExecutor{
		respond: func(_ context.Context, _ string, _ shOptions) shResult {
			return shResult{Code: 1, Stderr: "boom\n"}
		},
	}

	root := t.TempDir()
	entry := filepath.Join(root, "init.lua")
	require.NoError(t, os.WriteFile(entry, []byte(`
return function(hive)
  hive.sh.output("bad")
end
`), 0o644))

	pool := plugins.NewWorkerPool(1)
	module := &ShModule{
		PluginName:     "lua-test",
		Pool:           pool,
		Executor:       exec,
		DefaultTimeout: 5 * time.Second,
	}

	rt, err := NewRuntime(
		root,
		&LogModule{PluginName: "lua-test"},
		&PluginInfoModule{Name: "lua-test", Entry: entry, ModuleRoot: root},
		&CommandsModule{},
		module,
	)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = module.Close()
		rt.Close()
	})

	fn, err := rt.LoadEntrypoint(entry)
	require.NoError(t, err)

	err = rt.CallEntrypoint(fn)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "boom")
	assert.Contains(t, err.Error(), "exit 1")
}

func TestShExec_TableShape(t *testing.T) {
	t.Parallel()

	exec := &fakeExecutor{
		respond: func(_ context.Context, _ string, _ shOptions) shResult {
			return shResult{Stdout: "out", Stderr: "err", Code: 2}
		},
	}

	h := newShHarness(t, `
return function(hive)
  local r = hive.sh.exec("anything")
  hive.test_capture(r.stdout)
  hive.test_capture(r.stderr)
  hive.test_capture(r.code)
  hive.test_capture(r.err)
end
`, exec, 0)

	values := h.captured.snapshot()
	require.Len(t, values, 4)
	assert.Equal(t, glua.LString("out"), values[0])
	assert.Equal(t, glua.LString("err"), values[1])
	assert.Equal(t, 2, asLuaInt(t, values[2]))
	assert.Equal(t, glua.LNil, values[3])
}

func TestShExec_TableShape_WithErr(t *testing.T) {
	t.Parallel()

	exec := &fakeExecutor{
		respond: func(_ context.Context, _ string, _ shOptions) shResult {
			return shResult{Code: -1, Err: errors.New("timeout")}
		},
	}

	h := newShHarness(t, `
return function(hive)
  local r = hive.sh.exec("anything")
  hive.test_capture(r.err)
end
`, exec, 0)

	values := h.captured.snapshot()
	require.Len(t, values, 1)
	assert.Equal(t, glua.LString("timeout"), values[0])
}

func TestShExec_OptionsThreaded(t *testing.T) {
	t.Parallel()

	exec := &fakeExecutor{
		respond: func(_ context.Context, _ string, _ shOptions) shResult {
			return shResult{Code: 0}
		},
	}

	newShHarness(t, `
return function(hive)
  hive.sh.exec("anything", { cwd = "/tmp", timeout = 5 })
end
`, exec, 0)

	_, opts, _, calls := exec.snapshot()
	assert.Equal(t, 1, calls)
	assert.Equal(t, "/tmp", opts.Cwd)
	assert.Equal(t, 5*time.Second, opts.Timeout)
}

func TestShExec_DefaultTimeout(t *testing.T) {
	t.Parallel()

	exec := &fakeExecutor{
		respond: func(_ context.Context, _ string, _ shOptions) shResult {
			return shResult{Code: 0}
		},
	}

	newShHarness(t, `
return function(hive)
  hive.sh.exec("anything")
end
`, exec, 11*time.Second)

	_, opts, _, calls := exec.snapshot()
	assert.Equal(t, 1, calls)
	assert.Equal(t, 11*time.Second, opts.Timeout)
}

func TestShModule_ShutdownCancelsInflight(t *testing.T) {
	t.Parallel()

	released := make(chan struct{})
	captured := make(chan context.Context, 1)

	exec := &fakeExecutor{
		respond: func(ctx context.Context, _ string, _ shOptions) shResult {
			captured <- ctx
			<-ctx.Done()
			close(released)
			return shResult{Code: -1, Err: ctx.Err()}
		},
	}

	root := t.TempDir()
	entry := filepath.Join(root, "init.lua")
	require.NoError(t, os.WriteFile(entry, []byte(`
return function(hive)
  hive.commands({ Trigger = { sh = "true", help = "noop" } })
end
`), 0o644))

	pool := plugins.NewWorkerPool(1)
	module := &ShModule{
		PluginName:     "lua-test",
		Pool:           pool,
		Executor:       exec,
		DefaultTimeout: 0,
	}

	rt, err := NewRuntime(
		root,
		&LogModule{PluginName: "lua-test"},
		&PluginInfoModule{Name: "lua-test", Entry: entry, ModuleRoot: root},
		&CommandsModule{},
		module,
	)
	require.NoError(t, err)

	fn, err := rt.LoadEntrypoint(entry)
	require.NoError(t, err)
	require.NoError(t, rt.CallEntrypoint(fn))

	// Kick off a blocked exec from a goroutine; it will park inside
	// the fake executor until rootCtx is cancelled.
	done := make(chan shResult, 1)
	go func() {
		done <- module.runViaPool("blocking", shOptions{})
	}()

	var execCtx context.Context
	select {
	case execCtx = <-captured:
	case <-time.After(2 * time.Second):
		t.Fatalf("executor never received a context")
	}

	require.NoError(t, module.Close())

	select {
	case <-released:
	case <-time.After(2 * time.Second):
		t.Fatalf("executor never released after Close")
	}

	require.Error(t, execCtx.Err(), "executor's context should be cancelled by Close")

	select {
	case res := <-done:
		assert.Equal(t, -1, res.Code)
	case <-time.After(2 * time.Second):
		t.Fatalf("runViaPool never returned after Close")
	}

	rt.Close()
}

func TestShModule_RealExecutor_EndToEnd(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	entry := filepath.Join(root, "init.lua")
	require.NoError(t, os.WriteFile(entry, []byte(`
return function(hive)
  hive.test_capture(hive.sh.output("echo hello"))
end
`), 0o644))

	pool := plugins.NewWorkerPool(1)
	module := &ShModule{
		PluginName:     "lua-test",
		Pool:           pool,
		DefaultTimeout: 5 * time.Second,
	}
	captured := &shCaptureModule{}

	rt, err := NewRuntime(
		root,
		&LogModule{PluginName: "lua-test"},
		&PluginInfoModule{Name: "lua-test", Entry: entry, ModuleRoot: root},
		&CommandsModule{},
		module,
		captured,
	)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = module.Close()
		rt.Close()
	})

	fn, err := rt.LoadEntrypoint(entry)
	require.NoError(t, err)
	require.NoError(t, rt.CallEntrypoint(fn))

	values := captured.snapshot()
	require.Len(t, values, 1)
	assert.Equal(t, glua.LString("hello"), values[0])
}

// TestShModule_NilPoolRunsInline guards against a nil Pool panic so the
// module remains usable in tests that don't care about pool semantics.
func TestShModule_NilPoolRunsInline(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32
	exec := &fakeExecutor{
		respond: func(_ context.Context, _ string, _ shOptions) shResult {
			calls.Add(1)
			return shResult{Code: 0}
		},
	}

	root := t.TempDir()
	entry := filepath.Join(root, "init.lua")
	require.NoError(t, os.WriteFile(entry, []byte(`
return function(hive)
  hive.sh.run("anything")
end
`), 0o644))

	module := &ShModule{
		PluginName:     "lua-test",
		Pool:           nil,
		Executor:       exec,
		DefaultTimeout: time.Second,
	}

	rt, err := NewRuntime(
		root,
		&LogModule{PluginName: "lua-test"},
		&PluginInfoModule{Name: "lua-test", Entry: entry, ModuleRoot: root},
		&CommandsModule{},
		module,
	)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = module.Close()
		rt.Close()
	})

	fn, err := rt.LoadEntrypoint(entry)
	require.NoError(t, err)
	require.NoError(t, rt.CallEntrypoint(fn))

	assert.Equal(t, int32(1), calls.Load())
}
