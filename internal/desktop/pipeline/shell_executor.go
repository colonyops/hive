package pipeline

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/rs/zerolog"

	"github.com/colonyops/hive/internal/desktop/pipeline/actions"
	"github.com/colonyops/hive/pkg/tmpl"
)

// maxShellOutputLogBytes caps how much of a shell action's combined
// stdout/stderr is attached to a log line or error message, so a runaway
// command's output can't blow up the log.
const maxShellOutputLogBytes = 4096

// ShellExecutor runs a shell action's command_template via `sh -c`. The
// command is author-trusted config (actions.yml is a local file the
// desktop user authors themselves, not untrusted input), so no sandboxing
// beyond cwd/env/timeout is applied — matching the design's "author-trusted,
// no heavy sandbox" posture for flow function nodes.
type ShellExecutor struct {
	logger zerolog.Logger
}

// NewShellExecutor builds a ShellExecutor that logs command output at logger.
func NewShellExecutor(logger zerolog.Logger) *ShellExecutor {
	return &ShellExecutor{logger: logger}
}

func (e *ShellExecutor) Execute(ctx context.Context, action actions.Action, data OutputData) error {
	cfg, ok := action.Config.(*actions.ShellConfig)
	if !ok {
		return fmt.Errorf("shell executor: action %q has config type %T", action.ID, action.Config)
	}

	renderer := tmpl.New(tmpl.Config{})
	command, err := renderer.Render(cfg.CommandTemplate, data)
	if err != nil {
		return fmt.Errorf("shell: command_template: %w", err)
	}
	command = strings.TrimSpace(command)

	runCtx := ctx
	if d := cfg.Timeout.Duration(); d > 0 {
		var cancel context.CancelFunc
		runCtx, cancel = context.WithTimeout(ctx, d)
		defer cancel()
	}

	cmd := exec.CommandContext(runCtx, "sh", "-c", command)
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

	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out

	runErr := cmd.Run()
	output := truncateOutput(out.String())

	if runErr != nil {
		e.logger.Warn().Err(runErr).Str("action_id", action.ID).Str("output", output).
			Msg("shell action: command failed")
		return fmt.Errorf("shell: command failed: %w (output: %s)", runErr, output)
	}

	e.logger.Info().Str("action_id", action.ID).Str("output", output).
		Msg("shell action: command executed")
	return nil
}

func truncateOutput(s string) string {
	s = strings.TrimSpace(s)
	if len(s) <= maxShellOutputLogBytes {
		return s
	}
	return s[:maxShellOutputLogBytes] + "... (truncated)"
}
