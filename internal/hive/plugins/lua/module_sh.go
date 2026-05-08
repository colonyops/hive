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
// Every entry point is async: the call returns a handle immediately, the
// subprocess runs on a goroutine bound to a per-handle context, and the
// caller-supplied callback fires on the dispatcher when the subprocess
// finishes. Concurrency across plugins is bounded by the shared Pool.
// Close cancels every in-flight call.
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

// spawnAsync pins fn, allocates a handle, and starts the executor on a
// goroutine. Runs on the dispatcher; the goroutine hops back via
// Runtime.submitSync to invoke dispatch with the executor result.
//
// We use submitSync (not Submit) so the registry's WaitGroup accurately
// tracks every callback in a chain. If a Lua callback fires another
// hive.sh.* call, that call's wg.Add happens synchronously on the
// dispatcher inside dispatch(). By blocking the parent goroutine until
// dispatch returns, we ensure wg.Wait sees the child goroutine before
// the parent decrements. This lets registry.shutdown drain the entire
// callback chain rather than returning after just the first hop.
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
		err := m.Runtime.submitSync(func(state *glua.LState) error {
			fn := m.registry.loadFunction(state, id)
			if fn == nil {
				// Cancelled between completion and dispatch.
				return nil
			}
			dispatch(state, fn, result)
			m.registry.release(state, id)
			h.poison()
			return nil
		})
		if err != nil {
			m.Logger.Debug().
				Uint64("handle", id).
				Err(err).
				Msg("hive.sh: dispatch dropped (runtime closed)")
		}
	})

	return h
}

// execWithLogging acquires a pool slot, runs cmd through the executor,
// and emits start/finish/cancel logs tagged with the handle id so
// operators can correlate the events for a specific in-flight call.
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
		Uint64("handle", id).
		Str("cmd", cmd).
		Str("cwd", opts.Cwd).
		Dur("timeout", opts.Timeout).
		Msg("hive.sh: starting shell command")
}

func (m *ShModule) logShellPoolCancelled(id uint64, cmd string, err error) {
	m.Logger.Warn().
		Uint64("handle", id).
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
		Uint64("handle", id).
		Str("cmd", cmd).
		Int("code", result.Code).
		Dur("duration", dur).
		Err(result.Err).
		Msg("hive.sh: shell command finished")
}

// dispatchRun invokes the run callback with the exit code.
func (m *ShModule) dispatchRun(state *glua.LState, fn *glua.LFunction, result shResult) {
	if err := state.CallByParam(glua.P{
		Fn:      fn,
		NRet:    0,
		Protect: true,
	}, glua.LNumber(result.Code)); err != nil {
		m.Logger.Warn().
			Err(err).
			Msg("hive.sh.run: callback returned error")
	}
}

// dispatchOutput invokes the output callback with (stdout, err). On
// non-zero exit or executor error, stdout is LNil and err is a string
// describing the failure; otherwise stdout is the trimmed output and
// err is LNil.
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
			Msg("hive.sh.output: callback returned error")
	}
}

// dispatchExec invokes the exec callback with a result table:
// { stdout, stderr, code, err }.
func (m *ShModule) dispatchExec(state *glua.LState, fn *glua.LFunction, result shResult) {
	if err := state.CallByParam(glua.P{
		Fn:      fn,
		NRet:    0,
		Protect: true,
	}, buildExecTable(state, result)); err != nil {
		m.Logger.Warn().
			Err(err).
			Msg("hive.sh.exec: callback returned error")
	}
}

// parseRunArgs parses the args for hive.sh.run(cmd, fn). The callback is
// required; non-function arg 2 raises a Lua error via CheckFunction.
func (m *ShModule) parseRunArgs(state *glua.LState) (cmd string, callback *glua.LFunction) {
	cmd = state.CheckString(1)
	callback = state.CheckFunction(2)
	return cmd, callback
}

// parseOutputArgs parses the args for hive.sh.output(cmd, fn). Same
// shape as run.
func (m *ShModule) parseOutputArgs(state *glua.LState) (cmd string, callback *glua.LFunction) {
	return m.parseRunArgs(state)
}

// parseExecArgs parses the args for hive.sh.exec. Accepts:
//
//	exec(cmd, fn)
//	exec(cmd, opts, fn)
//
// where opts is a Lua table and fn is a Lua function. The callback is
// required; non-function final arg raises a Lua error.
func (m *ShModule) parseExecArgs(state *glua.LState) (cmd string, opts shOptions, callback *glua.LFunction) {
	cmd = state.CheckString(1)
	opts = shOptions{Timeout: m.DefaultTimeout}

	switch state.GetTop() {
	case 2:
		callback = state.CheckFunction(2)
	case 3:
		if state.Get(2) != glua.LNil {
			applyExecOpts(state.CheckTable(2), &opts)
		}
		callback = state.CheckFunction(3)
	default:
		state.RaiseError("hive.sh.exec: expected (cmd, fn) or (cmd, opts, fn)")
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
	handle := m.spawnAsync(state, callback, cmd, shOptions{Timeout: m.DefaultTimeout}, m.dispatchRun)
	state.Push(m.registry.handleUserData(state, handle))
	return 1
}

func (m *ShModule) luaOutput(state *glua.LState) int {
	cmd, callback := m.parseOutputArgs(state)
	handle := m.spawnAsync(state, callback, cmd, shOptions{Timeout: m.DefaultTimeout}, m.dispatchOutput)
	state.Push(m.registry.handleUserData(state, handle))
	return 1
}

func (m *ShModule) luaExec(state *glua.LState) int {
	cmd, opts, callback := m.parseExecArgs(state)
	handle := m.spawnAsync(state, callback, cmd, opts, m.dispatchExec)
	state.Push(m.registry.handleUserData(state, handle))
	return 1
}

// buildExecTable builds the Lua result table {stdout, stderr, code, err}
// passed to hive.sh.exec callbacks.
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

// formatExecError builds the err string passed to hive.sh.output
// callbacks on non-zero exit. Includes stderr (trimmed) and Err when
// present.
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
