package app

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
	"sync"

	"github.com/colonyops/hive/pkg/executil"

	"github.com/colonyops/hive/cmd/desktop/internal/app/execenv"
)

// envExecutor is Hive's shell executor running its children in the environment
// execenv resolves (ADR subprocess-environment). Session hooks are the user's own commands and
// reach it as `sh -c`, so without this they run with the PATH a desktop launch
// inherits and a session cannot be created at all on a machine whose tools came
// from a package manager. It replaces executil.RealExecutor rather than
// decorating it: the shared implementation exposes no seam for the child's
// environment, and the CLI that also runs it has no need for one.
type envExecutor struct {
	env *execenv.Resolver
}

func newEnvExecutor(env *execenv.Resolver) executil.Executor { return envExecutor{env: env} }

func (e envExecutor) Run(ctx context.Context, cmd string, args ...string) ([]byte, error) {
	out, err := e.command(ctx, "", cmd, args...).CombinedOutput()
	if err != nil {
		return out, executil.NewCommandError(cmd, "", out, err)
	}
	return out, nil
}

func (e envExecutor) RunDir(ctx context.Context, dir, cmd string, args ...string) ([]byte, error) {
	out, err := e.command(ctx, dir, cmd, args...).CombinedOutput()
	if err != nil {
		return out, executil.NewCommandError(cmd, dir, out, err)
	}
	return out, nil
}

func (e envExecutor) RunStream(ctx context.Context, stdout, stderr io.Writer, cmd string, args ...string) error {
	if err := e.stream(ctx, "", stdout, stderr, cmd, args...); err != nil {
		return fmt.Errorf("exec %s: %w", cmd, err)
	}
	return nil
}

func (e envExecutor) RunDirStream(ctx context.Context, dir string, stdout, stderr io.Writer, cmd string, args ...string) error {
	if err := e.stream(ctx, dir, stdout, stderr, cmd, args...); err != nil {
		return fmt.Errorf("exec %s in %s: %w", cmd, dir, err)
	}
	return nil
}

// stream copies stderr as it goes and repeats its opening as part of the error,
// because the caller that streams hook output discards it: a failed hook would
// otherwise surface in the jobs list as an exit status with nothing naming the
// command the shell could not find.
func (e envExecutor) stream(ctx context.Context, dir string, stdout, stderr io.Writer, cmd string, args ...string) error {
	command := e.command(ctx, dir, cmd, args...)
	diagnostic := &headBuffer{max: maxDiagnosticBytes}
	command.Stdout = stdout
	command.Stderr = io.MultiWriter(stderr, diagnostic)
	err := command.Run()
	if err == nil {
		return nil
	}
	if opening := diagnostic.String(); opening != "" {
		return fmt.Errorf("%w: %s", err, opening)
	}
	return err
}

// command resolves the binary itself rather than letting os/exec do it:
// exec.Command searches the calling process's PATH and ignores Cmd.Env, so a
// command found in the child's environment but not in ours would never start.
// A name that cannot be resolved is passed through unchanged, so the caller
// sees the OS's own error for the command it asked for.
func (e envExecutor) command(ctx context.Context, dir, cmd string, args ...string) *exec.Cmd {
	env := e.env.Environ(ctx)
	if path, err := e.env.LookPath(ctx, cmd); err == nil {
		cmd = path
	}
	command := exec.CommandContext(ctx, cmd, args...)
	command.Dir = dir
	command.Env = env
	return command
}

// maxDiagnosticBytes is what of a failing command's stderr travels in the
// error. A shell's "command not found" is the first line; a compiler's wall of
// output is not worth carrying into a job record.
const maxDiagnosticBytes = 2 << 10

// headBuffer keeps the first max bytes written to it and drains the rest, so a
// noisy child never blocks on a full pipe.
type headBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
	max int
}

func (b *headBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if room := b.max - b.buf.Len(); room > 0 {
		if len(p) < room {
			room = len(p)
		}
		_, _ = b.buf.Write(p[:room])
	}
	return len(p), nil
}

func (b *headBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}
