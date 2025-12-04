package git

import (
	"context"
	"fmt"
	"strings"

	"github.com/hay-kot/hive/pkg/executil"
)

// Executor implements Git using the git command-line tool.
type Executor struct {
	gitPath string
	exec    executil.Executor
}

// NewExecutor creates a new git executor with the specified git binary path.
func NewExecutor(gitPath string, exec executil.Executor) *Executor {
	return &Executor{gitPath: gitPath, exec: exec}
}

func (e *Executor) Clone(ctx context.Context, url, dest string) error {
	if _, err := e.exec.Run(ctx, e.gitPath, "clone", url, dest); err != nil {
		return fmt.Errorf("clone %s to %s: %w", url, dest, err)
	}
	return nil
}

func (e *Executor) Checkout(ctx context.Context, dir, branch string) error {
	if _, err := e.exec.RunDir(ctx, dir, e.gitPath, "checkout", branch); err != nil {
		return fmt.Errorf("checkout %s: %w", branch, err)
	}
	return nil
}

func (e *Executor) Pull(ctx context.Context, dir string) error {
	if _, err := e.exec.RunDir(ctx, dir, e.gitPath, "pull"); err != nil {
		return fmt.Errorf("pull: %w", err)
	}
	return nil
}

func (e *Executor) ResetHard(ctx context.Context, dir string) error {
	if _, err := e.exec.RunDir(ctx, dir, e.gitPath, "reset", "--hard"); err != nil {
		return fmt.Errorf("reset --hard: %w", err)
	}
	return nil
}

func (e *Executor) RemoteURL(ctx context.Context, dir string) (string, error) {
	out, err := e.exec.RunDir(ctx, dir, e.gitPath, "remote", "get-url", "origin")
	if err != nil {
		return "", fmt.Errorf("get remote url: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

func (e *Executor) IsClean(ctx context.Context, dir string) (bool, error) {
	out, err := e.exec.RunDir(ctx, dir, e.gitPath, "status", "--porcelain")
	if err != nil {
		return false, fmt.Errorf("git status: %w", err)
	}
	return len(strings.TrimSpace(string(out))) == 0, nil
}
