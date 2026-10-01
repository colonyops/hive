package app

import (
	"context"
	"io"

	"github.com/colonyops/hive/pkg/executil"

	"github.com/colonyops/hive/cmd/desktop/internal/app/tmuxbin"
)

// tmuxExecutor is Hive's shell executor with one substitution: a `tmux` command
// runs the binary discovery found (ADR tmux-discovery). Session spawn, recycle and kill
// all exec tmux by bare name from the shared session code, so a desktop launch
// — whose PATH holds no Homebrew or Nix prefix — cannot create a session at all
// without this. Decorating the interface keeps the discovery out of the shared
// packages, which the CLI also runs.
type tmuxExecutor struct {
	executil.Executor
	tmux *tmuxbin.Resolver
}

func newTmuxExecutor(inner executil.Executor, tmux *tmuxbin.Resolver) executil.Executor {
	return tmuxExecutor{Executor: inner, tmux: tmux}
}

func (e tmuxExecutor) Run(ctx context.Context, cmd string, args ...string) ([]byte, error) {
	return e.Executor.Run(ctx, e.resolve(cmd), args...)
}

func (e tmuxExecutor) RunDir(ctx context.Context, dir, cmd string, args ...string) ([]byte, error) {
	return e.Executor.RunDir(ctx, dir, e.resolve(cmd), args...)
}

func (e tmuxExecutor) RunStream(ctx context.Context, stdout, stderr io.Writer, cmd string, args ...string) error {
	return e.Executor.RunStream(ctx, stdout, stderr, e.resolve(cmd), args...)
}

func (e tmuxExecutor) RunDirStream(ctx context.Context, dir string, stdout, stderr io.Writer, cmd string, args ...string) error {
	return e.Executor.RunDirStream(ctx, dir, stdout, stderr, e.resolve(cmd), args...)
}

// resolve rewrites only the bare name, and only when discovery succeeds. A
// failed lookup passes `tmux` through to the inner executor, which searches
// the probe-derived PATH (ADR subprocess-environment) — a last-resort rescue for a tmux only
// the login shell knows about — before the user sees the OS's own "not found".
func (e tmuxExecutor) resolve(cmd string) string {
	if cmd != "tmux" {
		return cmd
	}
	path, err := e.tmux.Path()
	if err != nil {
		return cmd
	}
	return path
}
