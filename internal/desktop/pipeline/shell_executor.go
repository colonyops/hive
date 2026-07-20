package pipeline

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/colonyops/hive/internal/desktop/pipeline/actions"
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

// ShellExecutor runs a shell action's command_template via `sh -c`. The
// command is author-trusted config (actions.yml is a local file the
// desktop user authors themselves, not untrusted input), so no sandboxing
// beyond cwd/env/timeout is applied — matching the design's "author-trusted,
// no heavy sandbox" posture for flow function nodes.
type ShellExecutor struct {
	logger zerolog.Logger
}

func NewShellExecutor(logger zerolog.Logger) *ShellExecutor { return &ShellExecutor{logger: logger} }
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
	runCtx := ctx
	if d := cfg.Timeout.Duration(); d > 0 {
		var cancel context.CancelFunc
		runCtx, cancel = context.WithTimeout(ctx, d)
		defer cancel()
	}
	cmd := exec.CommandContext(runCtx, "sh", "-c", command)
	cmd.WaitDelay = shellKillGrace
	if cfg.Cwd != "" {
		cmd.Dir = cfg.Cwd
	}
	if len(cfg.Env) > 0 {
		env := os.Environ()
		for k, v := range cfg.Env {
			env = append(env, k+"="+v)
		}
		cmd.Env = env
	}
	stdout, stderr := &boundedExecutionWriter{}, &boundedExecutionWriter{}
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	if err := cmd.Run(); err != nil {
		result := ExecutionResult{Attempted: true, Log: ExecutionLog{Stdout: stdout.String(), Stderr: stderr.String()}}
		e.logger.Warn().Err(err).Str("action_id", action.ID).Msg("shell action: command failed")
		return result, fmt.Errorf("shell: command failed: %w", err)
	}
	result := ExecutionResult{Attempted: true, Log: ExecutionLog{Stdout: stdout.String(), Stderr: stderr.String()}}
	e.logger.Info().Str("action_id", action.ID).Msg("shell action: command executed")
	return result, nil
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
