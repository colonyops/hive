package lua

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog"
	glua "github.com/yuin/gopher-lua"

	"github.com/colonyops/hive/internal/hive/plugins"
)

// shDefaultTimeout is used when ShModule.DefaultTimeout is zero.
const shDefaultTimeout = 30 * time.Second

// shOptions captures per-call execution settings parsed from the Lua side.
type shOptions struct {
	Cwd     string
	Timeout time.Duration
}

// shResult is the executor's complete return value. Exec never returns an
// error separately; non-exit failures go in Err.
type shResult struct {
	Stdout string
	Stderr string
	Code   int
	Err    error
}

// ShellExecutor runs a shell command and reports the outcome. The default
// implementation (osExecutor) shells out via "sh -c"; tests inject fakes.
type ShellExecutor interface {
	Exec(ctx context.Context, cmd string, opts shOptions) shResult
}

// osExecutor runs commands through "sh -c" with separate stdout/stderr buffers
// and applies opts.Cwd / opts.Timeout. Returns -1 on signal/timeout.
type osExecutor struct{}

func (osExecutor) Exec(ctx context.Context, cmd string, opts shOptions) shResult {
	if opts.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, opts.Timeout)
		defer cancel()
	}

	c := exec.CommandContext(ctx, "sh", "-c", cmd)
	if opts.Cwd != "" {
		c.Dir = opts.Cwd
	}

	var stdout, stderr bytes.Buffer
	c.Stdout = &stdout
	c.Stderr = &stderr

	err := c.Run()

	result := shResult{
		Stdout: stdout.String(),
		Stderr: stderr.String(),
		Code:   0,
	}

	if err == nil {
		return result
	}

	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		result.Code = exitErr.ExitCode()
		return result
	}

	result.Code = -1
	result.Err = err
	return result
}

// ShModule exposes hive.sh.{run,output,exec} for shell command execution.
// Synchronous calls block the Lua dispatcher; async calls (callback as the
// last argument) return a handle and run on the registry's WaitGroup.
// Concurrency across plugins is bounded by the shared Pool. Close cancels
// in-flight work.
type ShModule struct {
	Pool           *plugins.WorkerPool
	Executor       ShellExecutor
	DefaultTimeout time.Duration
	Logger         zerolog.Logger

	Runtime *Runtime

	registry asyncRegistry

	closeOnce sync.Once
}

// Register installs the hive.sh subtable and initialises the per-module
// async registry. Defaults the executor and timeout if unset.
func (m *ShModule) Register(state *glua.LState, hive *glua.LTable) error {
	if m.Executor == nil {
		m.Executor = osExecutor{}
	}
	if m.DefaultTimeout <= 0 {
		m.DefaultTimeout = shDefaultTimeout
	}

	if err := m.registry.init(state, m, asyncRegistryConfig{
		KeyPrefix:     "hive.sh.",
		MetatableName: "hive.sh.handle",
	}); err != nil {
		return fmt.Errorf("sh module: %w", err)
	}

	sh := state.NewTable()
	state.SetField(sh, "run", state.NewFunction(m.luaRun))
	state.SetField(sh, "output", state.NewFunction(m.luaOutput))
	state.SetField(sh, "exec", state.NewFunction(m.luaExec))
	state.SetField(hive, "sh", sh)
	return nil
}

// Close cancels rootCtx and waits for outstanding shell calls to drain.
// Idempotent.
func (m *ShModule) Close() error {
	m.closeOnce.Do(func() { m.registry.shutdown() })
	return nil
}

// runViaPool executes cmd through the shared worker pool. Pool acquisition
// itself respects rootCtx so a Close mid-wait returns without running.
func (m *ShModule) runViaPool(cmd string, opts shOptions) shResult {
	defer m.registry.trackGoroutine()()
	return m.execWithLogging(0, m.registry.rootContext(), cmd, opts)
}

// spawnAsync pins fn, allocates a handle, and starts the executor on a
// goroutine. Runs on the dispatcher; the goroutine hops back via
// Runtime.Submit to invoke dispatch with the executor result.
func (m *ShModule) spawnAsync(
	state *glua.LState,
	fn *glua.LFunction,
	cmd string,
	opts shOptions,
	dispatch func(*glua.LState, *glua.LFunction, shResult),
) *asyncHandle {
	id, ctx := m.registry.allocate(state, fn)
	h := m.registry.newHandle(id, m.Runtime)

	m.registry.Go(func() {
		result := m.execWithLogging(id, ctx, cmd, opts)
		m.Runtime.Submit(func(state *glua.LState) {
			fn := m.registry.loadFunction(state, id)
			if fn == nil {
				// Cancelled between completion and dispatch.
				return
			}
			dispatch(state, fn, result)
			m.registry.release(state, id)
			h.poison()
		})
	})

	return h
}

// execWithLogging acquires a pool slot, runs cmd through the executor,
// and emits start/finish/cancel logs. Shared between the sync and async
// paths; id is 0 for sync (no handle field is logged) and non-zero for
// async (handle is included in every log line so operators can correlate
// start/finish/cancel for a specific in-flight call).
func (m *ShModule) execWithLogging(id uint64, ctx context.Context, cmd string, opts shOptions) shResult {
	start := time.Now()
	m.logShellStart(id, cmd, opts)

	var result shResult
	if err := m.Pool.RunContext(ctx, func() {
		result = m.Executor.Exec(ctx, cmd, opts)
	}); err != nil {
		m.logShellPoolCancelled(id, cmd, err)
		return shResult{Code: -1, Err: err}
	}

	m.logShellFinish(id, cmd, time.Since(start), result)
	return result
}

func (m *ShModule) logShellStart(id uint64, cmd string, opts shOptions) {
	m.Logger.Debug().
		Func(addHandle(id)).
		Str("cmd", cmd).
		Str("cwd", opts.Cwd).
		Dur("timeout", opts.Timeout).
		Msg("hive.sh: starting shell command")
}

func (m *ShModule) logShellPoolCancelled(id uint64, cmd string, err error) {
	m.Logger.Warn().
		Func(addHandle(id)).
		Str("cmd", cmd).
		Err(err).
		Msg("hive.sh: pool acquisition cancelled")
}

func (m *ShModule) logShellFinish(id uint64, cmd string, dur time.Duration, result shResult) {
	level := m.Logger.Debug
	if result.Err != nil || result.Code != 0 {
		level = m.Logger.Warn
	}
	level().
		Func(addHandle(id)).
		Str("cmd", cmd).
		Int("code", result.Code).
		Dur("duration", dur).
		Err(result.Err).
		Msg("hive.sh: shell command finished")
}

// addHandle returns a zerolog Event hook that attaches the handle field
// when id is non-zero. Used by sh's start/finish/cancel logs to
// correlate async log lines.
func addHandle(id uint64) func(*zerolog.Event) {
	return func(e *zerolog.Event) {
		if id != 0 {
			e.Uint64("handle", id)
		}
	}
}

// dispatchRun invokes the async run callback with the exit code.
func (m *ShModule) dispatchRun(state *glua.LState, fn *glua.LFunction, result shResult) {
	if err := state.CallByParam(glua.P{
		Fn:      fn,
		NRet:    0,
		Protect: true,
	}, glua.LNumber(result.Code)); err != nil {
		m.Logger.Warn().
			Err(err).
			Msg("hive.sh.run: async callback returned error")
	}
}

// dispatchOutput invokes the async output callback with (stdout, err).
// On non-zero exit or executor error, stdout is LNil and err is the
// same string the sync form would raise; otherwise stdout is the
// trimmed output and err is LNil.
func (m *ShModule) dispatchOutput(state *glua.LState, fn *glua.LFunction, result shResult) {
	var stdoutArg, errArg glua.LValue
	if result.Err != nil || result.Code != 0 {
		stdoutArg = glua.LNil
		errArg = glua.LString(formatExecError(result))
	} else {
		stdoutArg = glua.LString(strings.TrimRight(result.Stdout, "\n"))
		errArg = glua.LNil
	}
	if err := state.CallByParam(glua.P{
		Fn:      fn,
		NRet:    0,
		Protect: true,
	}, stdoutArg, errArg); err != nil {
		m.Logger.Warn().
			Err(err).
			Msg("hive.sh.output: async callback returned error")
	}
}

// dispatchExec invokes the async exec callback with the result table
// matching the sync form's shape: { stdout, stderr, code, err }.
func (m *ShModule) dispatchExec(state *glua.LState, fn *glua.LFunction, result shResult) {
	if err := state.CallByParam(glua.P{
		Fn:      fn,
		NRet:    0,
		Protect: true,
	}, buildExecTable(state, result)); err != nil {
		m.Logger.Warn().
			Err(err).
			Msg("hive.sh.exec: async callback returned error")
	}
}

// parseRunArgs parses the args for hive.sh.run. Returns the trailing
// callback if the second arg is a function; otherwise nil for sync.
func (m *ShModule) parseRunArgs(state *glua.LState) (cmd string, callback *glua.LFunction) {
	cmd = state.CheckString(1)
	if state.GetTop() >= 2 {
		if fn, ok := state.Get(2).(*glua.LFunction); ok {
			callback = fn
		}
	}
	return cmd, callback
}

// parseOutputArgs parses the args for hive.sh.output. Same shape as run.
func (m *ShModule) parseOutputArgs(state *glua.LState) (cmd string, callback *glua.LFunction) {
	return m.parseRunArgs(state)
}

// parseExecArgs parses the args for hive.sh.exec. Accepts:
//
//	exec(cmd)
//	exec(cmd, opts)
//	exec(cmd, fn)
//	exec(cmd, opts, fn)
//
// where opts is a Lua table and fn is a Lua function. Anything else at
// arg 2 raises a Lua error.
func (m *ShModule) parseExecArgs(state *glua.LState) (cmd string, opts shOptions, callback *glua.LFunction) {
	cmd = state.CheckString(1)
	opts = shOptions{Timeout: m.DefaultTimeout}

	if state.GetTop() < 2 {
		return cmd, opts, nil
	}

	switch v := state.Get(2).(type) {
	case *glua.LTable:
		applyExecOpts(v, &opts)
		if state.GetTop() >= 3 {
			if fn, ok := state.Get(3).(*glua.LFunction); ok {
				callback = fn
			} else if state.Get(3) != glua.LNil {
				state.ArgError(3, "expected function or nil")
			}
		}
	case *glua.LFunction:
		callback = v
	case *glua.LNilType:
		// no-op: treat as sync no-opts
	default:
		state.ArgError(2, "expected table, function, or nil")
	}

	return cmd, opts, callback
}

// applyExecOpts copies cwd/timeout from the Lua opts table.
func applyExecOpts(t *glua.LTable, opts *shOptions) {
	if cwd, ok := t.RawGetString("cwd").(glua.LString); ok {
		opts.Cwd = string(cwd)
	}
	if timeoutSecs, ok := t.RawGetString("timeout").(glua.LNumber); ok {
		opts.Timeout = time.Duration(float64(timeoutSecs) * float64(time.Second))
	}
}

func (m *ShModule) luaRun(state *glua.LState) int {
	cmd, callback := m.parseRunArgs(state)
	if callback == nil {
		result := m.runViaPool(cmd, shOptions{Timeout: m.DefaultTimeout})
		state.Push(glua.LNumber(result.Code))
		return 1
	}
	handle := m.spawnAsync(state, callback, cmd, shOptions{Timeout: m.DefaultTimeout}, m.dispatchRun)
	state.Push(m.registry.handleUserData(state, handle))
	return 1
}

func (m *ShModule) luaOutput(state *glua.LState) int {
	cmd, callback := m.parseOutputArgs(state)
	if callback == nil {
		result := m.runViaPool(cmd, shOptions{Timeout: m.DefaultTimeout})

		if result.Err != nil || result.Code != 0 {
			state.RaiseError("hive.sh.output: %s", formatExecError(result))
			return 0
		}

		state.Push(glua.LString(strings.TrimRight(result.Stdout, "\n")))
		return 1
	}
	handle := m.spawnAsync(state, callback, cmd, shOptions{Timeout: m.DefaultTimeout}, m.dispatchOutput)
	state.Push(m.registry.handleUserData(state, handle))
	return 1
}

func (m *ShModule) luaExec(state *glua.LState) int {
	cmd, opts, callback := m.parseExecArgs(state)
	if callback == nil {
		result := m.runViaPool(cmd, opts)
		state.Push(buildExecTable(state, result))
		return 1
	}
	handle := m.spawnAsync(state, callback, cmd, opts, m.dispatchExec)
	state.Push(m.registry.handleUserData(state, handle))
	return 1
}

// buildExecTable builds the Lua result table {stdout, stderr, code, err}.
// Shared between the sync luaExec return and the async dispatchExec path.
func buildExecTable(state *glua.LState, r shResult) *glua.LTable {
	out := state.NewTable()
	state.SetField(out, "stdout", glua.LString(r.Stdout))
	state.SetField(out, "stderr", glua.LString(r.Stderr))
	state.SetField(out, "code", glua.LNumber(r.Code))
	if r.Err != nil {
		state.SetField(out, "err", glua.LString(r.Err.Error()))
	} else {
		state.SetField(out, "err", glua.LNil)
	}
	return out
}

// formatExecError builds the Lua error message used by hive.sh.output.
// Includes stderr (trimmed) and Err when present.
func formatExecError(r shResult) string {
	var parts []string
	parts = append(parts, fmt.Sprintf("exit %d", r.Code))
	if r.Err != nil {
		parts = append(parts, r.Err.Error())
	}
	if msg := strings.TrimRight(r.Stderr, "\n"); msg != "" {
		parts = append(parts, msg)
	}
	return strings.Join(parts, ": ")
}
