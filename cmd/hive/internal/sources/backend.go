package sources

import "strings"

// Backend identifies which forge driver services a source for a given repo.
// It is derived from the repo's git remote host (see DetectBackend).
//
// ENUM(github, gitea)
type Backend string

// DetectBackend resolves the forge backend for a git remote host.
//
// Resolution order (first match wins):
//  1. An explicit override in overrides (host -> backend), for ambiguous,
//     mirrored, or self-hosted setups the heuristics cannot classify.
//  2. Well-known public hosts (github.com, codeberg.org).
//  3. A hostname heuristic ("gitea"/"forgejo" -> gitea, "github" -> github).
//  4. Default: github. The gh CLI supports GitHub Enterprise hosts, so an
//     unrecognized host is most safely treated as GitHub Enterprise; users
//     point Gitea hosts at the gitea backend via overrides.
func DetectBackend(host string, overrides map[string]Backend) Backend {
	host = strings.ToLower(strings.TrimSpace(host))
	if host == "" {
		return BackendGithub
	}

	if b, ok := overrides[host]; ok {
		return b
	}

	switch host {
	case "github.com":
		return BackendGithub
	case "codeberg.org":
		return BackendGitea
	}

	switch {
	case strings.Contains(host, "gitea"), strings.Contains(host, "forgejo"):
		return BackendGitea
	case strings.Contains(host, "github"):
		return BackendGithub
	default:
		return BackendGithub
	}
}
