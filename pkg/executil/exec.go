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
	stderr := &HeadWriter{Max: maxStderrLen}
	c.Stdout = io.Discard
	c.Stderr = stderr
	if err := c.Run(); err != nil {
		return errorWithOutput(err, stderr.Bytes())
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

// RealExecutor calls actual shell commands. Its zero value runs them in this
// process's environment and resolves them through this process's PATH.
type RealExecutor struct {
	// Env, when set, is the environment each command runs with.
	Env func(ctx context.Context) []string
	// LookPath, when set, resolves the command name before it runs. os/exec
	// searches this process's PATH and ignores Cmd.Env, so a program that only
	// the Env PATH holds needs this to start. A name LookPath cannot resolve
	// runs unchanged, so the caller sees the OS's own error for it.
	LookPath func(ctx context.Context, file string) (string, error)
	// StreamDiagnosticBytes, when positive, appends up to that many leading
	// bytes of a failed streamed command's stderr to its error, for callers
	// that stream stderr somewhere nobody reads.
	StreamDiagnosticBytes int
}

func (e *RealExecutor) command(ctx context.Context, dir, cmd string, args ...string) *exec.Cmd {
	if e.LookPath != nil {
		if path, err := e.LookPath(ctx, cmd); err == nil {
			cmd = path
		}
	}
	c := exec.CommandContext(ctx, cmd, args...)
	c.Dir = dir
	if e.Env != nil {
		c.Env = e.Env(ctx)
	}
	return c
}

// Run executes a command and returns its combined output.
func (e *RealExecutor) Run(ctx context.Context, cmd string, args ...string) ([]byte, error) {
	out, err := e.command(ctx, "", cmd, args...).CombinedOutput()
	if err != nil {
		return out, NewCommandError(cmd, "", out, err)
	}
	return out, nil
}

// RunOutputDir executes a command in dir (empty means inherit cwd) and returns
// stdout and stderr separately. Keeping the streams apart lets callers parse
// stdout as JSON while surfacing stderr as the error message.
func (e *RealExecutor) RunOutputDir(ctx context.Context, dir, cmd string, args ...string) ([]byte, []byte, error) {
	c := e.command(ctx, dir, cmd, args...)
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
	out, err := e.command(ctx, dir, cmd, args...).CombinedOutput()
	if err != nil {
		return out, NewCommandError(cmd, dir, out, err)
	}
	return out, nil
}

// RunStream executes a command and streams stdout/stderr to the provided writers.
func (e *RealExecutor) RunStream(ctx context.Context, stdout, stderr io.Writer, cmd string, args ...string) error {
	if err := e.stream(ctx, "", stdout, stderr, cmd, args...); err != nil {
		return fmt.Errorf("exec %s: %w", cmd, err)
	}
	return nil
}

// RunDirStream executes a command in a specific directory and streams output.
func (e *RealExecutor) RunDirStream(ctx context.Context, dir string, stdout, stderr io.Writer, cmd string, args ...string) error {
	if err := e.stream(ctx, dir, stdout, stderr, cmd, args...); err != nil {
		return fmt.Errorf("exec %s in %s: %w", cmd, dir, err)
	}
	return nil
}

func (e *RealExecutor) stream(ctx context.Context, dir string, stdout, stderr io.Writer, cmd string, args ...string) error {
	c := e.command(ctx, dir, cmd, args...)
	c.Stdout = stdout
	c.Stderr = stderr
	var diagnostic *HeadWriter
	if e.StreamDiagnosticBytes > 0 {
		diagnostic = &HeadWriter{Max: e.StreamDiagnosticBytes}
		c.Stderr = diagnostic
		if stderr != nil {
			c.Stderr = io.MultiWriter(stderr, diagnostic)
		}
	}
	err := c.Run()
	if err == nil {
		return nil
	}
	if diagnostic != nil {
		if opening := diagnostic.String(); opening != "" {
			return fmt.Errorf("%w: %s", err, opening)
		}
	}
	return err
}
