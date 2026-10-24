// Package git provides an abstraction for git operations.
package git

import (
	"context"
	"net/url"
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
	// SetRemoteURL points origin in dir at url.
	SetRemoteURL(ctx context.Context, dir, url string) error
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

// RemoteIdentity returns the key under which remotes that name the same
// repository compare equal: host and repository path, without the transport,
// user, default port, or ".git" suffix. The host is lowercased everywhere but
// the path only on GitHub, which ignores its case; other forges may not. A
// local path or a remote that does not parse is returned trimmed.
func RemoteIdentity(remote string) string {
	remote = strings.TrimSpace(remote)
	host, path, ok := remoteHostPath(remote)
	if !ok {
		return remote
	}
	path = repositoryPath(path)
	if path == "" {
		return remote
	}
	host = strings.ToLower(host)
	if host == "github.com" {
		path = strings.ToLower(path)
	}
	return host + "/" + path
}

// EquivalentRemote reports whether two remotes refer to the same repository
// under RemoteIdentity.
func EquivalentRemote(left, right string) bool {
	leftID, rightID := RemoteIdentity(left), RemoteIdentity(right)
	return leftID != "" && leftID == rightID
}

// WebURL returns the browser address of the repository a remote names:
// https://host/path, without ".git". An http(s) remote keeps its scheme and
// port, which are the web server's. Any other transport's port belongs to git,
// so WebURL drops it. A local path or a remote that does not parse returns "".
func WebURL(remote string) string {
	remote = strings.TrimSpace(remote)
	host, path, ok := remoteHostPath(remote)
	if !ok {
		return ""
	}
	path = repositoryPath(path)
	if path == "" {
		return ""
	}
	if u, err := url.Parse(remote); err == nil && (u.Scheme == "http" || u.Scheme == "https") {
		return u.Scheme + "://" + host + "/" + path
	}
	if at := strings.LastIndex(host, "@"); at != -1 {
		host = host[at+1:]
	}
	return "https://" + stripPort(host) + "/" + path
}

func repositoryPath(path string) string {
	path = strings.Trim(path, "/")
	if len(path) >= len(".git") && strings.EqualFold(path[len(path)-len(".git"):], ".git") {
		path = strings.TrimRight(path[:len(path)-len(".git")], "/")
	}
	return path
}

var defaultRemotePorts = map[string]string{"ssh": "22", "git": "9418", "http": "80", "https": "443"}

func remoteHostPath(remote string) (host, path string, ok bool) {
	if strings.Contains(remote, "://") {
		u, err := url.Parse(remote)
		if err != nil || u.Hostname() == "" {
			return "", "", false
		}
		defaultPort, known := defaultRemotePorts[u.Scheme]
		if !known {
			return "", "", false
		}
		host = u.Hostname()
		if port := u.Port(); port != "" && port != defaultPort {
			host = u.Host
		}
		return host, u.Path, true
	}

	// Git reads [user@]host:path as SCP-style SSH only when no slash comes
	// before the first colon; otherwise it is a local path.
	host, path, found := strings.Cut(remote, ":")
	if !found || host == "" || strings.Contains(host, "/") {
		return "", "", false
	}
	user := ""
	if at := strings.LastIndex(host, "@"); at != -1 {
		user, host = host[:at], host[at+1:]
	}
	// A relative path is relative to the user's home, so alice@host:repo and
	// bob@host:repo are different repositories. Forges serve every repository
	// as git@, so that user is dropped to match the HTTPS spelling.
	if user != "" && user != "git" && !strings.HasPrefix(path, "/") {
		host = user + "@" + host
	}
	return host, path, host != ""
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
