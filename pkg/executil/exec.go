// Package executil provides shell execution utilities.
package executil

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
	"strings"
)

const maxStderrLen = 500

// limitedWriter caps writes to a bytes.Buffer at a maximum byte count.
// Bytes beyond the limit are silently discarded.
type limitedWriter struct {
	buf *bytes.Buffer
	n   int64
	max int64
}

func (w *limitedWriter) Write(p []byte) (int, error) {
	if w.n >= w.max {
		return len(p), nil
	}
	remaining := w.max - w.n
	origLen := len(p)
	if int64(origLen) > remaining {
		p = p[:remaining]
	}
	n, err := w.buf.Write(p)
	w.n += int64(n)
	if err != nil {
		return n, err
	}
	return origLen, nil
}

// CommandError describes a failed child process and its capped output.
type CommandError struct {
	Command string
	Dir     string
	Output  []byte
	Err     error
}

func cappedErrorOutput(output []byte) []byte {
	if len(output) > maxStderrLen {
		return output[:maxStderrLen]
	}
	return output
}

// NewCommandError creates a command error with at most 500 bytes of child output.
func NewCommandError(command, dir string, output []byte, err error) *CommandError {
	return &CommandError{
		Command: command,
		Dir:     dir,
		Output:  bytes.Clone(cappedErrorOutput(output)),
		Err:     err,
	}
}

func (e *CommandError) Error() string {
	prefix := fmt.Sprintf("exec %s", e.Command)
	if e.Dir != "" {
		prefix = fmt.Sprintf("%s in %s", prefix, e.Dir)
	}
	msg := strings.TrimSpace(string(cappedErrorOutput(e.Output)))
	if msg != "" && !strings.Contains(e.Err.Error(), msg) {
		return fmt.Sprintf("%s: %s: %v", prefix, msg, e.Err)
	}
	return fmt.Sprintf("%s: %v", prefix, e.Err)
}

func (e *CommandError) Unwrap() error {
	return e.Err
}

func errorWithOutput(err error, output []byte) error {
	msg := strings.TrimSpace(string(cappedErrorOutput(output)))
	if msg == "" {
		return err
	}
	return fmt.Errorf("%s: %w", msg, err)
}

// RunSh executes a shell command in the given directory (empty means inherit cwd).
// On failure, stderr is returned as the error message, capped at 500 bytes to
// prevent large or ANSI-polluted output from corrupting logs or TUI display.
// The original *exec.ExitError is preserved via wrapping so callers can inspect
// exit codes with errors.As.
func RunSh(ctx context.Context, dir, cmd string) error {
	c := exec.CommandContext(ctx, "sh", "-c", cmd)
	if dir != "" {
		c.Dir = dir
	}
	var buf bytes.Buffer
	c.Stdout = io.Discard
	c.Stderr = &limitedWriter{buf: &buf, max: maxStderrLen}
	if err := c.Run(); err != nil {
		return errorWithOutput(err, buf.Bytes())
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
		return out, NewCommandError(cmd, "", out, err)
	}
	return out, nil
}

// RunOutputDir executes a command in dir (empty means inherit cwd) and returns
// stdout and stderr separately. Keeping the streams apart lets callers parse
// stdout as JSON while surfacing stderr as the error message.
func (e *RealExecutor) RunOutputDir(ctx context.Context, dir, cmd string, args ...string) ([]byte, []byte, error) {
	c := exec.CommandContext(ctx, cmd, args...)
	if dir != "" {
		c.Dir = dir
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	c.Stdout = &stdout
	c.Stderr = &stderr
	if err := c.Run(); err != nil {
		return stdout.Bytes(), stderr.Bytes(), fmt.Errorf("exec %s: %w", cmd, err)
	}
	return stdout.Bytes(), stderr.Bytes(), nil
}

// RunDir executes a command in a specific directory.
func (e *RealExecutor) RunDir(ctx context.Context, dir, cmd string, args ...string) ([]byte, error) {
	c := exec.CommandContext(ctx, cmd, args...)
	c.Dir = dir
	out, err := c.CombinedOutput()
	if err != nil {
		return out, NewCommandError(cmd, dir, out, err)
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
