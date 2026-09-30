package dispatch

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/colonyops/hive/cmd/desktop/internal/app/actions"
	"github.com/colonyops/hive/cmd/desktop/internal/app/observe"
	"github.com/colonyops/hive/pkg/tmpl"
	"github.com/rs/zerolog"
)

const (
	maxExecutionStreamBytes = 64 * 1024
	truncatedStreamMarker   = "\n... (truncated)"
)

// shellKillGrace bounds how long Run blocks after the command's context is
// cancelled (timeout hit). CommandContext SIGKILLs `sh`, but a descendant it
// spawned (e.g. `sleep`) can inherit the stdout/stderr pipe and keep it open,
// which would otherwise make Wait block on the copy goroutine until that
// grandchild exits on its own. WaitDelay force-closes the pipes shortly after
// the kill so a timed-out command returns promptly instead of running its full
// duration.
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
		Timeout: cfg.Timeout.Duration(),
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
	proc := exec.CommandContext(runCtx, "sh", "-c", cmd.Command)
	proc.WaitDelay = shellKillGrace
	proc.Dir = cmd.Dir
	environ := env.Environ(runCtx)
	for k, v := range cmd.Env {
		environ = append(environ, k+"="+v)
	}
	proc.Env = environ
	stdout, stderr := &boundedExecutionWriter{}, &boundedExecutionWriter{}
	proc.Stdout, proc.Stderr = stdout, stderr
	err = proc.Run()
	return ExecutionLog{Stdout: stdout.String(), Stderr: stderr.String()}, err
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

// boundedExecutionWriter drains every write while retaining only a bounded
// diagnostic prefix. It must return the full input length so a noisy child
// cannot block on a full stdout/stderr pipe.
type boundedExecutionWriter struct {
	buf       bytes.Buffer
	truncated bool
}

func (w *boundedExecutionWriter) Write(p []byte) (int, error) {
	original := len(p)
	if w.buf.Len() < maxExecutionStreamBytes {
		remaining := maxExecutionStreamBytes - w.buf.Len()
		if len(p) > remaining {
			p = p[:remaining]
			w.truncated = true
		}
		_, _ = w.buf.Write(p)
	} else if len(p) > 0 {
		w.truncated = true
	}
	return original, nil
}

func (w *boundedExecutionWriter) String() string {
	stream := w.buf.String()
	if !w.truncated {
		return stream
	}
	return stream[:maxExecutionStreamBytes-len(truncatedStreamMarker)] + truncatedStreamMarker
}
