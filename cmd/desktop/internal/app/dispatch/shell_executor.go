package dispatch

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"time"

	"github.com/rs/zerolog"

	"github.com/colonyops/hive/cmd/desktop/internal/app/actions"
	"github.com/colonyops/hive/internal/platform/observe"
	"github.com/colonyops/hive/pkg/executil"
	"github.com/colonyops/hive/pkg/tmpl"
)

const (
	maxExecutionStreamBytes = 64 * 1024
	truncatedStreamMarker   = "\n... (truncated)"
)

// shellKillGrace lets an owned process group exit after cancellation before
// the executor escalates to SIGKILL.
const shellKillGrace = 2 * time.Second

// ExecEnvironment supplies the environment a spawned command runs in. A shell
// action's command is the user's own, so it needs the PATH their terminal has
// rather than the one a desktop launch inherits (ADR subprocess-environment).
type ExecEnvironment interface {
	Environ(ctx context.Context) []string
}

// ShellExecutor runs a shell action's command_template via `sh -c`. The
// command is author-trusted config (actions.yml is a local file the
// desktop user authors themselves, not untrusted input), so no sandboxing
// beyond cwd/env/timeout is applied — matching the design's "author-trusted,
// no heavy sandbox" posture for flow function nodes.
type ShellExecutor struct {
	logger zerolog.Logger
	env    ExecEnvironment
}

func NewShellExecutor(logger zerolog.Logger, env ExecEnvironment) *ShellExecutor {
	return &ShellExecutor{logger: logger, env: env}
}

func (e *ShellExecutor) Execute(ctx context.Context, action actions.Action, data OutputData, _ ActionInvocationInput) (ExecutionResult, error) {
	cfg, ok := action.Config.(*actions.ShellConfig)
	if !ok {
		return ExecutionResult{}, fmt.Errorf("shell executor: action %q has config type %T", action.ID, action.Config)
	}
	command, err := tmpl.New(tmpl.Config{}).Render(cfg.CommandTemplate, data)
	if err != nil {
		return ExecutionResult{}, fmt.Errorf("shell: command_template: %w", err)
	}
	command = strings.TrimSpace(command)
	if command == "" {
		return ExecutionResult{}, fmt.Errorf("shell: command_template rendered blank")
	}
	log, err := runShell(ctx, e.env, "dispatch.shell", shellCommand{
		Command: command,
		Dir:     shellWorkingDir(cfg, data),
		Env:     cfg.Env,
		Timeout: cfg.Timeout,
	})
	if err != nil {
		e.logger.Warn().Ctx(ctx).Err(err).Str("action_id", action.ID).Msg("shell action: command failed")
		return ExecutionResult{Attempted: true, Log: log}, fmt.Errorf("shell: command failed: %w", err)
	}
	e.logger.Info().Ctx(ctx).Str("action_id", action.ID).Msg("shell action: command executed")
	return ExecutionResult{Attempted: true, Log: log}, nil
}

type shellCommand struct {
	Command string
	Dir     string
	Env     map[string]string
	Timeout time.Duration
}

// spanName names the wait, and every call site passes a literal so the name
// stays a bounded search key.
func runShell(ctx context.Context, env ExecEnvironment, spanName string, cmd shellCommand) (log ExecutionLog, err error) {
	ctx, span := observe.StartConditionalSpan(ctx, tracer, spanName)
	defer observe.End(span, &err)

	runCtx := ctx
	if cmd.Timeout > 0 {
		var cancel context.CancelFunc
		runCtx, cancel = context.WithTimeout(ctx, cmd.Timeout)
		defer cancel()
	}
	proc := exec.CommandContext(context.WithoutCancel(runCtx), "sh", "-c", cmd.Command)
	proc.Dir = cmd.Dir
	environ := env.Environ(runCtx)
	for k, v := range cmd.Env {
		environ = append(environ, k+"="+v)
	}
	proc.Env = environ
	// One byte past the bound is what lets boundExecutionStream tell output
	// that fit exactly from output that overflowed.
	stdout := &executil.HeadWriter{Max: maxExecutionStreamBytes + 1}
	stderr := &executil.HeadWriter{Max: maxExecutionStreamBytes + 1}
	runLog := RunLogFrom(ctx)
	proc.Stdout, proc.Stderr = io.MultiWriter(stdout, runLog.Stdout()), io.MultiWriter(stderr, runLog.Stderr())
	runLog.Systemf("%s", shellCommandLine(cmd))
	started := time.Now()
	err = runShellProcess(runCtx, proc)
	runLog.Systemf("%s", shellExitLine(err, time.Since(started), cmd.Timeout))
	return ExecutionLog{Stdout: boundExecutionStream(stdout.String()), Stderr: boundExecutionStream(stderr.String())}, err
}

func shellCommandLine(cmd shellCommand) string {
	lines := strings.Split(cmd.Command, "\n")
	for i := range lines {
		if i == 0 {
			lines[i] = "$ " + lines[i]
		} else {
			lines[i] = "  " + lines[i]
		}
	}
	if cmd.Dir != "" {
		lines = append(lines, "in "+cmd.Dir)
	}
	return strings.Join(lines, "\n")
}

func shellExitLine(err error, elapsed, timeout time.Duration) string {
	took := formatRunDuration(elapsed)
	var exitErr *exec.ExitError
	switch {
	case err == nil:
		return "Process exited with code 0 after " + took
	case errors.Is(err, context.DeadlineExceeded):
		return fmt.Sprintf("Process stopped: timed out after %s", formatRunDuration(timeout))
	case errors.Is(err, context.Canceled):
		return "Process stopped after " + took + ": cancelled"
	case errors.As(err, &exitErr) && exitErr.ExitCode() >= 0:
		return fmt.Sprintf("Process exited with code %d after %s", exitErr.ExitCode(), took)
	default:
		return fmt.Sprintf("Process failed after %s: %v", took, err)
	}
}

// shellWorkingDir resolves the directory the command runs in. A configured
// cwd always wins; a terminal invocation otherwise runs in the session's own
// checkout, which is what makes `mise run test` a complete action rather than
// one that has to restate where the session lives. An empty result leaves
// cmd.Dir unset, which is the desktop process's own cwd.
func shellWorkingDir(cfg *actions.ShellConfig, data OutputData) string {
	if cfg.Cwd != "" {
		return cfg.Cwd
	}
	if data.Session != nil {
		return data.Session.Path
	}
	return ""
}

func boundExecutionStream(stream string) string {
	if len(stream) <= maxExecutionStreamBytes {
		return stream
	}
	return stream[:maxExecutionStreamBytes-len(truncatedStreamMarker)] + truncatedStreamMarker
}
