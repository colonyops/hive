package lua

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/rs/zerolog"
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

// shHarness wraps luaHarness with the ShModule reference so tests can
// reach the module for explicit shutdown ordering or to drive the
// dispatcher for handle inspection.
type shHarness struct {
	*luaHarness
	module   *ShModule
	executor *fakeExecutor
}

func newShHarness(t *testing.T, script string, executor *fakeExecutor, defaultTimeout time.Duration) *shHarness {
	t.Helper()
	module := &ShModule{
		Pool:           plugins.NewWorkerPool(1),
		Executor:       executor,
		DefaultTimeout: defaultTimeout,
		Logger:         zerolog.Nop(),
	}
	return &shHarness{
		luaHarness: newLuaHarness(t, script, module),
		module:     module,
		executor:   executor,
	}
}

// asLuaInt unwraps an LNumber to int for exact comparison without
// triggering the testifylint float-compare rule.
func asLuaInt(t *testing.T, v glua.LValue) int {
	t.Helper()
	n, ok := v.(glua.LNumber)
	require.True(t, ok, "expected LNumber, got %T", v)
	return int(n)
}

// waitForCaptures polls the capture list until it reaches n values, with
// a generous timeout for CI. Returns the final snapshot.
func waitForCaptures(t *testing.T, capture *captureModule, n int) []glua.LValue {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		values := capture.Snapshot()
		if len(values) >= n {
			return values
		}
		if time.Now().After(deadline) {
			t.Fatalf("expected %d captures, got %d", n, len(values))
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestShRun_CallbackReceivesCode(t *testing.T) {
	t.Parallel()

	exec := &fakeExecutor{
		respond: func(_ context.Context, cmd string, _ shOptions) shResult {
			switch cmd {
			case "ok":
				return shResult{Code: 0}
			case "weird":
				return shResult{Code: 7}
			}
			t.Fatalf("unexpected cmd %q", cmd)
			return shResult{}
		},
	}

	h := newShHarness(t, `
return function(hive)
  hive.sh.run("ok", function(code) hive.test_capture(code) end)
  hive.sh.run("weird", function(code) hive.test_capture(code) end)
end
`, exec, 0)

	values := waitForCaptures(t, h.capture, 2)
	require.Len(t, values, 2)
	// Two async runs race; assert as a multiset.
	codes := []int{asLuaInt(t, values[0]), asLuaInt(t, values[1])}
	assert.ElementsMatch(t, []int{0, 7}, codes)
}

func TestShRun_RequiresCallback(t *testing.T) {
	t.Parallel()

	h := newShHarness(t, `
return function(hive)
  local ok, err = pcall(hive.sh.run, "anything")
  if ok then
    error("expected hive.sh.run to require a callback")
  end
  hive.test_capture("err", tostring(err))
end
`, &fakeExecutor{}, 0)

	got := h.capture.String("err")
	assert.NotEmpty(t, got, "callback-missing pcall should have captured an err")
}

func TestShOutput_SuccessPassesStdoutNilErr(t *testing.T) {
	t.Parallel()

	exec := &fakeExecutor{
		respond: func(_ context.Context, _ string, _ shOptions) shResult {
			return shResult{Stdout: "hi\n", Code: 0}
		},
	}

	h := newShHarness(t, `
return function(hive)
  hive.sh.output("anything", function(stdout, err)
    hive.test_capture(stdout)
    hive.test_capture(err)
  end)
end
`, exec, 0)

	values := waitForCaptures(t, h.capture, 2)
	require.Len(t, values, 2)
	assert.Equal(t, glua.LString("hi"), values[0])
	assert.Equal(t, glua.LNil, values[1])
}

func TestShOutput_FailurePassesNilStdoutAndErr(t *testing.T) {
	t.Parallel()

	exec := &fakeExecutor{
		respond: func(_ context.Context, _ string, _ shOptions) shResult {
			return shResult{Stderr: "boom\n", Code: 1}
		},
	}

	h := newShHarness(t, `
return function(hive)
  hive.sh.output("bad", function(stdout, err)
    hive.test_capture(stdout)
    hive.test_capture(err)
  end)
end
`, exec, 0)

	values := waitForCaptures(t, h.capture, 2)
	require.Len(t, values, 2)
	assert.Equal(t, glua.LNil, values[0])
	errStr, ok := values[1].(glua.LString)
	require.True(t, ok, "expected LString err, got %T", values[1])
	assert.Contains(t, string(errStr), "exit 1")
	assert.Contains(t, string(errStr), "boom")
}

func TestShExec_CallbackReceivesTable(t *testing.T) {
	t.Parallel()

	exec := &fakeExecutor{
		respond: func(_ context.Context, _ string, _ shOptions) shResult {
			return shResult{Stdout: "out", Stderr: "err", Code: 2, Err: errors.New("explained")}
		},
	}

	h := newShHarness(t, `
return function(hive)
  hive.sh.exec("anything", function(r)
    hive.test_capture(r.stdout)
    hive.test_capture(r.stderr)
    hive.test_capture(r.code)
    hive.test_capture(r.err)
  end)
end
`, exec, 0)

	values := waitForCaptures(t, h.capture, 4)
	require.Len(t, values, 4)
	assert.Equal(t, glua.LString("out"), values[0])
	assert.Equal(t, glua.LString("err"), values[1])
	assert.Equal(t, 2, asLuaInt(t, values[2]))
	assert.Equal(t, glua.LString("explained"), values[3])
}

func TestShExec_OptsThreaded(t *testing.T) {
	t.Parallel()

	exec := &fakeExecutor{
		respond: func(_ context.Context, _ string, _ shOptions) shResult {
			return shResult{Code: 0}
		},
	}

	h := newShHarness(t, `
return function(hive)
  hive.sh.exec("anything", { cwd = "/tmp", timeout = 5 }, function(r)
    hive.test_capture(r.code)
  end)
end
`, exec, 0)

	_ = waitForCaptures(t, h.capture, 1)

	_, opts, _, calls := exec.snapshot()
	assert.Equal(t, 1, calls)
	assert.Equal(t, "/tmp", opts.Cwd)
	assert.Equal(t, 5*time.Second, opts.Timeout)
}

func TestShExec_DefaultTimeoutAppliedWhenOptsOmitted(t *testing.T) {
	t.Parallel()

	exec := &fakeExecutor{
		respond: func(_ context.Context, _ string, _ shOptions) shResult {
			return shResult{Code: 0}
		},
	}

	h := newShHarness(t, `
return function(hive)
  hive.sh.exec("anything", function(r) hive.test_capture(r.code) end)
end
`, exec, 11*time.Second)

	_ = waitForCaptures(t, h.capture, 1)

	_, opts, _, calls := exec.snapshot()
	assert.Equal(t, 1, calls)
	assert.Equal(t, 11*time.Second, opts.Timeout)
}

func TestShAsync_HandleCancelKillsSubprocess(t *testing.T) {
	t.Parallel()

	executorCtx := make(chan context.Context, 1)
	released := make(chan struct{})

	exec := &fakeExecutor{
		respond: func(ctx context.Context, _ string, _ shOptions) shResult {
			executorCtx <- ctx
			<-ctx.Done()
			close(released)
			return shResult{Code: -1, Err: ctx.Err()}
		},
	}

	h := newShHarness(t, `
return function(hive)
  HANDLE = hive.sh.run("blocking", function(code) hive.test_capture(code) end)
end
`, exec, 0)

	// Wait for the executor to receive its ctx.
	var ctx context.Context
	select {
	case ctx = <-executorCtx:
	case <-time.After(2 * time.Second):
		t.Fatalf("executor never received a context")
	}

	// Cancel the handle via the dispatcher.
	cancelDone := make(chan struct{})
	h.module.Runtime.Submit(func(state *glua.LState) {
		defer close(cancelDone)
		ud, ok := state.GetGlobal("HANDLE").(*glua.LUserData)
		require.True(t, ok)
		handle, ok := ud.Value.(*asyncHandle)
		require.True(t, ok)
		handle.Cancel()
	})

	select {
	case <-cancelDone:
	case <-time.After(2 * time.Second):
		t.Fatalf("cancel submit never ran")
	}

	select {
	case <-released:
	case <-time.After(2 * time.Second):
		t.Fatalf("executor never released after cancel")
	}

	require.Error(t, ctx.Err(), "executor's per-handle ctx should be cancelled")

	// The callback should never fire because the handle was cancelled
	// before dispatch had a chance to load the registry pin.
	time.Sleep(50 * time.Millisecond)
	values := h.capture.Snapshot()
	assert.Empty(t, values, "callback should not fire after cancel")
}

func TestShAsync_ShutdownDrainsInflight(t *testing.T) {
	t.Parallel()

	executorCtx := make(chan context.Context, 1)
	released := make(chan struct{})

	exec := &fakeExecutor{
		respond: func(ctx context.Context, _ string, _ shOptions) shResult {
			executorCtx <- ctx
			<-ctx.Done()
			close(released)
			return shResult{Code: -1, Err: ctx.Err()}
		},
	}

	h := newShHarness(t, `
return function(hive)
  hive.sh.run("blocking", function(code) hive.test_capture(code) end)
end
`, exec, 0)

	var ctx context.Context
	select {
	case ctx = <-executorCtx:
	case <-time.After(2 * time.Second):
		t.Fatalf("executor never received a context")
	}

	require.NoError(t, h.module.Close())

	select {
	case <-released:
	case <-time.After(2 * time.Second):
		t.Fatalf("executor never released after Close")
	}

	require.Error(t, ctx.Err(), "executor's per-handle ctx should be cancelled by Close")

	// Close drained the goroutine cleanly; the dispatcher remains alive
	// here (rt.Close runs in t.Cleanup), so any queued dispatch may
	// still run. The contract is: no panic, and any callback that does
	// fire receives the cancellation result (Code: -1).
	time.Sleep(50 * time.Millisecond)
	for _, v := range h.capture.Snapshot() {
		assert.Equal(t, -1, asLuaInt(t, v),
			"post-shutdown callback should observe cancellation result")
	}
}

func TestShAsync_CancelAfterCompletionIsNoop(t *testing.T) {
	t.Parallel()

	exec := &fakeExecutor{
		respond: func(_ context.Context, _ string, _ shOptions) shResult {
			return shResult{Code: 0}
		},
	}

	h := newShHarness(t, `
return function(hive)
  HANDLE = hive.sh.run("ok", function(code) hive.test_capture(code) end)
end
`, exec, 0)

	values := waitForCaptures(t, h.capture, 1)
	require.Len(t, values, 1)

	cancelDone := make(chan struct{})
	h.module.Runtime.Submit(func(state *glua.LState) {
		defer close(cancelDone)
		ud, ok := state.GetGlobal("HANDLE").(*glua.LUserData)
		require.True(t, ok)
		handle, ok := ud.Value.(*asyncHandle)
		require.True(t, ok)
		handle.Cancel() // post-completion cancel — should be a true no-op
	})

	select {
	case <-cancelDone:
	case <-time.After(2 * time.Second):
		t.Fatalf("cancel submit never ran")
	}

	// Still exactly one capture; no panic in the dispatcher.
	values = h.capture.Snapshot()
	assert.Len(t, values, 1)
}

func TestShModule_RealExecutor_EndToEnd(t *testing.T) {
	t.Parallel()

	module := &ShModule{
		Pool:           plugins.NewWorkerPool(1),
		DefaultTimeout: 5 * time.Second,
		Logger:         zerolog.Nop(),
	}

	h := newLuaHarness(t, `
return function(hive)
  hive.sh.output("echo hello", function(stdout, err)
    hive.test_capture(stdout)
    hive.test_capture(err)
  end)
end
`, module)

	values := waitForCaptures(t, h.capture, 2)
	require.Len(t, values, 2)
	assert.Equal(t, glua.LString("hello"), values[0])
	assert.Equal(t, glua.LNil, values[1])
}
