package tmuxexec

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
	"time"

	"github.com/colonyops/hive/internal/domain/multiplexer"
	"github.com/colonyops/hive/pkg/executil"
)

// Runner executes tmux operations in captured, input-bearing, or interactive mode.
type Runner interface {
	Available() bool
	Capture(ctx context.Context, args ...string) (stdout, stderr []byte, err error)
	Input(ctx context.Context, input io.Reader, args ...string) (stdout, stderr []byte, err error)
	Interactive(ctx context.Context, streams multiplexer.AttachStreams, args ...string) error
}

// ExecRunnerOptions configures an executable-backed Runner.
type ExecRunnerOptions struct {
	// Binary resolves the tmux executable for each command. Nil uses PATH.
	Binary func(context.Context) (string, error)
	// Environ returns the child environment. Nil inherits the process environment.
	Environ func(context.Context) []string
	// PrepareArgs can add server-selection arguments before execution.
	PrepareArgs func([]string) []string
}

// NewExecRunner creates a Runner that invokes a configurable tmux executable.
func NewExecRunner(options ExecRunnerOptions) Runner {
	return execRunner{
		binary:      options.Binary,
		environ:     options.Environ,
		prepareArgs: options.PrepareArgs,
	}
}

const maxDiagnosticBytes = 500

// pipeWaitDelay bounds how long Wait blocks on stdio pipes after the context
// kills tmux. A child that tmux forked can keep the pipes open after tmux
// itself is gone; without a delay, Wait would wait for that child instead of
// honouring the cancellation.
const pipeWaitDelay = 500 * time.Millisecond

type execRunner struct {
	binary      func(context.Context) (string, error)
	environ     func(context.Context) []string
	prepareArgs func([]string) []string
}

func (r execRunner) Available() bool {
	if r.binary != nil {
		_, err := r.binary(context.Background())
		return err == nil
	}
	_, err := exec.LookPath("tmux")
	return err == nil
}

func (r execRunner) Capture(ctx context.Context, args ...string) ([]byte, []byte, error) {
	return r.runCaptured(ctx, nil, args...)
}

func (r execRunner) Input(ctx context.Context, input io.Reader, args ...string) ([]byte, []byte, error) {
	return r.runCaptured(ctx, input, args...)
}

func (r execRunner) runCaptured(ctx context.Context, input io.Reader, args ...string) ([]byte, []byte, error) {
	cmd, binary, err := r.command(ctx, args...)
	if err != nil {
		return nil, nil, err
	}
	cmd.Stdin = input
	var stdout bytes.Buffer
	stderr := &executil.HeadWriter{Max: maxDiagnosticBytes}
	cmd.Stdout = &stdout
	cmd.Stderr = stderr
	if err := cmd.Run(); err != nil {
		diagnostics := stderr.Bytes()
		if len(diagnostics) == 0 {
			diagnostics = stdout.Bytes()
		}
		if ctx.Err() != nil {
			err = ctx.Err()
		}
		return stdout.Bytes(), stderr.Bytes(), executil.NewCommandError(binary, "", diagnostics, err)
	}
	return stdout.Bytes(), stderr.Bytes(), nil
}

func (r execRunner) Interactive(ctx context.Context, streams multiplexer.AttachStreams, args ...string) error {
	cmd, binary, err := r.command(ctx, args...)
	if err != nil {
		return err
	}
	cmd.Stdin = streams.Stdin
	cmd.Stdout = streams.Stdout
	diagnostics := &executil.HeadWriter{Max: maxDiagnosticBytes}
	if streams.Stderr == nil {
		cmd.Stderr = diagnostics
	} else {
		cmd.Stderr = io.MultiWriter(streams.Stderr, diagnostics)
	}
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			err = ctx.Err()
		}
		return executil.NewCommandError(binary, "", diagnostics.Bytes(), err)
	}
	return nil
}

func (r execRunner) command(ctx context.Context, args ...string) (*exec.Cmd, string, error) {
	binary := "tmux"
	if r.binary != nil {
		resolved, err := r.binary(ctx)
		if err != nil {
			return nil, "", fmt.Errorf("resolve tmux executable: %w", err)
		}
		binary = resolved
	}
	prepared := append([]string(nil), args...)
	if r.prepareArgs != nil {
		prepared = r.prepareArgs(prepared)
	}
	cmd := exec.CommandContext(ctx, binary, prepared...)
	cmd.WaitDelay = pipeWaitDelay
	if r.environ != nil {
		cmd.Env = r.environ(ctx)
	}
	return cmd, binary, nil
}
