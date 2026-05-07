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
// Calls run on the Lua dispatcher and synchronously block it; concurrency
// across plugins is bounded by the shared Pool. Close cancels in-flight work.
type ShModule struct {
	PluginName     string
	Pool           *plugins.WorkerPool
	Executor       ShellExecutor
	DefaultTimeout time.Duration
	Logger         zerolog.Logger

	rootCtx    context.Context
	rootCancel context.CancelFunc
	wg         sync.WaitGroup

	closeOnce sync.Once
}

// Register installs the hive.sh subtable. Initialises rootCtx/rootCancel and
// applies the default executor and timeout if unset.
func (m *ShModule) Register(state *glua.LState, hive *glua.LTable) error {
	if m.Executor == nil {
		m.Executor = osExecutor{}
	}
	if m.DefaultTimeout <= 0 {
		m.DefaultTimeout = shDefaultTimeout
	}
	m.rootCtx, m.rootCancel = context.WithCancel(context.Background())

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
	m.closeOnce.Do(func() {
		if m.rootCancel != nil {
			m.rootCancel()
		}
		m.wg.Wait()
	})
	return nil
}

// runViaPool executes cmd through the shared worker pool. Pool acquisition
// itself respects rootCtx so a Close mid-wait returns without running.
func (m *ShModule) runViaPool(cmd string, opts shOptions) shResult {
	m.wg.Add(1)
	defer m.wg.Done()

	start := time.Now()
	m.Logger.Debug().
		Str("cmd", cmd).
		Str("cwd", opts.Cwd).
		Dur("timeout", opts.Timeout).
		Msg("hive.sh: starting shell command")

	var result shResult
	if err := m.Pool.RunContext(m.rootCtx, func() {
		result = m.Executor.Exec(m.rootCtx, cmd, opts)
	}); err != nil {
		m.Logger.Warn().
			Str("cmd", cmd).
			Err(err).
			Msg("hive.sh: pool acquisition cancelled")
		return shResult{Code: -1, Err: err}
	}

	level := m.Logger.Debug
	if result.Err != nil || result.Code != 0 {
		level = m.Logger.Warn
	}
	level().
		Str("cmd", cmd).
		Int("code", result.Code).
		Dur("duration", time.Since(start)).
		Err(result.Err).
		Msg("hive.sh: shell command finished")

	return result
}

func (m *ShModule) luaRun(state *glua.LState) int {
	cmd := state.CheckString(1)
	result := m.runViaPool(cmd, shOptions{Timeout: m.DefaultTimeout})
	state.Push(glua.LNumber(result.Code))
	return 1
}

func (m *ShModule) luaOutput(state *glua.LState) int {
	cmd := state.CheckString(1)
	result := m.runViaPool(cmd, shOptions{Timeout: m.DefaultTimeout})

	if result.Err != nil {
		state.RaiseError("hive.sh.output: %s", formatExecError(result))
		return 0
	}
	if result.Code != 0 {
		state.RaiseError("hive.sh.output: %s", formatExecError(result))
		return 0
	}

	state.Push(glua.LString(strings.TrimRight(result.Stdout, "\n")))
	return 1
}

func (m *ShModule) luaExec(state *glua.LState) int {
	cmd := state.CheckString(1)
	opts := shOptions{Timeout: m.DefaultTimeout}

	if state.GetTop() >= 2 {
		optsTable := state.CheckTable(2)
		if cwd, ok := optsTable.RawGetString("cwd").(glua.LString); ok {
			opts.Cwd = string(cwd)
		}
		if timeoutSecs, ok := optsTable.RawGetString("timeout").(glua.LNumber); ok {
			opts.Timeout = time.Duration(float64(timeoutSecs) * float64(time.Second))
		}
	}

	result := m.runViaPool(cmd, opts)

	out := state.NewTable()
	state.SetField(out, "stdout", glua.LString(result.Stdout))
	state.SetField(out, "stderr", glua.LString(result.Stderr))
	state.SetField(out, "code", glua.LNumber(result.Code))
	if result.Err != nil {
		state.SetField(out, "err", glua.LString(result.Err.Error()))
	} else {
		state.SetField(out, "err", glua.LNil)
	}
	state.Push(out)
	return 1
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
