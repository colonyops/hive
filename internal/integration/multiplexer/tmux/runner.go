package tmux

import (
	"bytes"
	"context"
	"io"
	"os/exec"

	"github.com/colonyops/hive/internal/core/multiplexer"
	"github.com/colonyops/hive/pkg/executil"
)

// Runner executes tmux operations in captured, input-bearing, or interactive mode.
type Runner interface {
	Available() bool
	Capture(ctx context.Context, args ...string) (stdout, stderr []byte, err error)
	Input(ctx context.Context, input io.Reader, args ...string) (stdout, stderr []byte, err error)
	Interactive(ctx context.Context, streams multiplexer.AttachStreams, args ...string) error
}

const maxDiagnosticBytes = 500

type diagnosticBuffer struct {
	bytes.Buffer
}

func (b *diagnosticBuffer) Write(p []byte) (int, error) {
	originalLen := len(p)
	remaining := maxDiagnosticBytes - b.Len()
	if remaining <= 0 {
		return originalLen, nil
	}
	if len(p) > remaining {
		p = p[:remaining]
	}
	if _, err := b.Buffer.Write(p); err != nil {
		return 0, err
	}
	return originalLen, nil
}

type execRunner struct{}

func (execRunner) Available() bool {
	_, err := exec.LookPath("tmux")
	return err == nil
}

func (execRunner) Capture(ctx context.Context, args ...string) ([]byte, []byte, error) {
	return runCaptured(ctx, nil, args...)
}

func (execRunner) Input(ctx context.Context, input io.Reader, args ...string) ([]byte, []byte, error) {
	return runCaptured(ctx, input, args...)
}

func runCaptured(ctx context.Context, input io.Reader, args ...string) ([]byte, []byte, error) {
	cmd := exec.CommandContext(ctx, "tmux", args...)
	cmd.Stdin = input
	var stdout bytes.Buffer
	var stderr diagnosticBuffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		diagnostics := stderr.Bytes()
		if len(diagnostics) == 0 {
			diagnostics = stdout.Bytes()
		}
		if ctx.Err() != nil {
			err = ctx.Err()
		}
		return stdout.Bytes(), stderr.Bytes(), executil.NewCommandError("tmux", "", diagnostics, err)
	}
	return stdout.Bytes(), stderr.Bytes(), nil
}

func (execRunner) Interactive(ctx context.Context, streams multiplexer.AttachStreams, args ...string) error {
	cmd := exec.CommandContext(ctx, "tmux", args...)
	cmd.Stdin = streams.Stdin
	cmd.Stdout = streams.Stdout
	var diagnostics diagnosticBuffer
	if streams.Stderr == nil {
		cmd.Stderr = &diagnostics
	} else {
		cmd.Stderr = io.MultiWriter(streams.Stderr, &diagnostics)
	}
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			err = ctx.Err()
		}
		return executil.NewCommandError("tmux", "", diagnostics.Bytes(), err)
	}
	return nil
}
