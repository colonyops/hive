// Package executil provides shell execution utilities.
package executil

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
)

const maxStderrLen = 500

// RunSh executes a shell command in the given directory (empty means inherit cwd).
// On failure, stderr is returned as the error message, capped at 500 bytes to
// prevent large or ANSI-polluted output from corrupting logs or TUI display.
func RunSh(ctx context.Context, dir, cmd string) error {
	c := exec.CommandContext(ctx, "sh", "-c", cmd)
	if dir != "" {
		c.Dir = dir
	}
	var stderr bytes.Buffer
	c.Stdout = io.Discard
	c.Stderr = &stderr
	if err := c.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if len(msg) > maxStderrLen {
			msg = msg[:maxStderrLen] + "..."
		}
		if msg != "" {
			return errors.New(msg)
		}
		return err
	}
	return nil
}

// Executor runs shell commands.
type Executor interface {
	// Run executes a command and returns its combined output.
	Run(ctx context.Context, cmd string, args ...string) ([]byte, error)
	// RunDir executes a command in a specific directory.
	RunDir(ctx context.Context, dir, cmd string, args ...string) ([]byte, error)
	// RunStream executes a command and streams stdout/stderr to the provided writers.
	RunStream(ctx context.Context, stdout, stderr io.Writer, cmd string, args ...string) error
	// RunDirStream executes a command in a specific directory and streams output.
	RunDirStream(ctx context.Context, dir string, stdout, stderr io.Writer, cmd string, args ...string) error
}

// RealExecutor calls actual shell commands.
type RealExecutor struct{}

// Run executes a command and returns its combined output.
func (e *RealExecutor) Run(ctx context.Context, cmd string, args ...string) ([]byte, error) {
	out, err := exec.CommandContext(ctx, cmd, args...).CombinedOutput()
	if err != nil {
		return out, fmt.Errorf("exec %s: %w", cmd, err)
	}
	return out, nil
}

// RunDir executes a command in a specific directory.
func (e *RealExecutor) RunDir(ctx context.Context, dir, cmd string, args ...string) ([]byte, error) {
	c := exec.CommandContext(ctx, cmd, args...)
	c.Dir = dir
	out, err := c.CombinedOutput()
	if err != nil {
		return out, fmt.Errorf("exec %s in %s: %w", cmd, dir, err)
	}
	return out, nil
}

// RunStream executes a command and streams stdout/stderr to the provided writers.
func (e *RealExecutor) RunStream(ctx context.Context, stdout, stderr io.Writer, cmd string, args ...string) error {
	c := exec.CommandContext(ctx, cmd, args...)
	c.Stdout = stdout
	c.Stderr = stderr
	if err := c.Run(); err != nil {
		return fmt.Errorf("exec %s: %w", cmd, err)
	}
	return nil
}

// RunDirStream executes a command in a specific directory and streams output.
func (e *RealExecutor) RunDirStream(ctx context.Context, dir string, stdout, stderr io.Writer, cmd string, args ...string) error {
	c := exec.CommandContext(ctx, cmd, args...)
	c.Dir = dir
	c.Stdout = stdout
	c.Stderr = stderr
	if err := c.Run(); err != nil {
		return fmt.Errorf("exec %s in %s: %w", cmd, dir, err)
	}
	return nil
}
