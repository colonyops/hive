// Package git provides an abstraction for git operations.
package git

import (
	"context"
	"strings"
)

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
	// DefaultBranch returns the default branch name (e.g., "main" or "master") for the repository.
	DefaultBranch(ctx context.Context, dir string) (string, error)
	// DiffStats returns the number of lines added and deleted compared to the default branch.
	DiffStats(ctx context.Context, dir string) (additions, deletions int, err error)
	// IsValidRepo checks if dir contains a valid git repository.
	IsValidRepo(ctx context.Context, dir string) error
	// CloneBare creates a bare clone of url at dest.
	CloneBare(ctx context.Context, url, dest string) error
	// WorktreeAdd creates a new worktree at path on a new branch named branch.
	WorktreeAdd(ctx context.Context, repoDir, path, branch string) error
	// WorktreeRemove removes the worktree at path and deletes branch from repoDir when provided.
	WorktreeRemove(ctx context.Context, repoDir, path, branch string) error
	// Fetch fetches all remotes in dir.
	Fetch(ctx context.Context, dir string) error
	// HasUnpushedCommits returns true if there are local commits not yet pushed to a remote.
	// It first checks the upstream tracking branch; if none is set, it falls back to comparing
	// against origin/<default branch>. Returns false (no risk) on any git error.
	HasUnpushedCommits(ctx context.Context, dir string) (bool, error)
}

// ExtractRepoName extracts the repository name from a git remote URL.
// Handles both SSH (git@github.com:user/repo.git) and HTTPS (https://github.com/user/repo.git) formats.
func ExtractRepoName(remote string) string {
	remote = strings.TrimSuffix(remote, ".git")

	if idx := strings.LastIndex(remote, "/"); idx != -1 {
		return remote[idx+1:]
	}

	if idx := strings.LastIndex(remote, ":"); idx != -1 {
		part := remote[idx+1:]
		if slashIdx := strings.LastIndex(part, "/"); slashIdx != -1 {
			return part[slashIdx+1:]
		}
		return part
	}

	return remote
}

// ExtractHost extracts the host (without port) from a git remote URL.
// Handles SCP-style SSH (git@github.com:owner/repo.git), ssh:// and https://
// URLs, and hosts with non-standard ports (git.example.com:2222). Returns an
// empty string when no host can be determined.
func ExtractHost(remote string) string {
	remote = strings.TrimSpace(remote)
	if remote == "" {
		return ""
	}

	// Scheme-based URLs: ssh://, https://, http://, git://.
	if idx := strings.Index(remote, "://"); idx != -1 {
		rest := remote[idx+3:]
		if at := strings.LastIndex(rest, "@"); at != -1 {
			rest = rest[at+1:]
		}
		host := rest
		if slash := strings.IndexAny(host, "/"); slash != -1 {
			host = host[:slash]
		}
		return stripPort(host)
	}

	// SCP-style SSH: [user@]host:owner/repo.git
	if at := strings.LastIndex(remote, "@"); at != -1 {
		remote = remote[at+1:]
	}
	if colon := strings.Index(remote, ":"); colon != -1 {
		return stripPort(remote[:colon])
	}

	return ""
}

// stripPort removes a trailing ":port" from a host, leaving IPv6 literals and
// bare hosts intact.
func stripPort(host string) string {
	if host == "" {
		return ""
	}
	if strings.HasPrefix(host, "[") {
		return host
	}
	if colon := strings.LastIndex(host, ":"); colon != -1 {
		return host[:colon]
	}
	return host
}

// ExtractOwnerRepo extracts owner and repo from a git remote URL.
// Handles SSH (git@github.com:owner/repo.git) and HTTPS (https://github.com/owner/repo.git).
// Returns empty strings if parsing fails.
func ExtractOwnerRepo(remote string) (owner, repo string) {
	remote = strings.TrimSuffix(remote, ".git")

	// SSH format: git@github.com:owner/repo
	if idx := strings.Index(remote, ":"); idx != -1 && !strings.HasPrefix(remote, "http") {
		parts := strings.Split(remote[idx+1:], "/")
		if len(parts) >= 2 {
			return parts[len(parts)-2], parts[len(parts)-1]
		}
	}

	// HTTPS format: https://github.com/owner/repo
	parts := strings.Split(remote, "/")
	if len(parts) >= 2 {
		return parts[len(parts)-2], parts[len(parts)-1]
	}

	return "", ""
}
