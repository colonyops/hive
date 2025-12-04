// Package git provides an abstraction for git operations.
package git

import "context"

// Git defines git operations needed by hive.
type Git interface {
	// Clone clones a repository from url to dest.
	Clone(ctx context.Context, url, dest string) error
	// Checkout switches to the specified branch in dir.
	Checkout(ctx context.Context, dir, branch string) error
	// Pull fetches and merges changes in dir.
	Pull(ctx context.Context, dir string) error
	// ResetHard discards all local changes in dir.
	ResetHard(ctx context.Context, dir string) error
	// RemoteURL returns the origin remote URL for dir.
	RemoteURL(ctx context.Context, dir string) (string, error)
	// IsClean returns true if there are no uncommitted changes in dir.
	IsClean(ctx context.Context, dir string) (bool, error)
	// Branch returns the current branch name, or short commit SHA if in detached HEAD state.
	Branch(ctx context.Context, dir string) (string, error)
	// DiffStats returns the number of lines added and deleted compared to HEAD.
	DiffStats(ctx context.Context, dir string) (additions, deletions int, err error)
}
