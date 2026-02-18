package sessions

import (
	"context"
	"os"
	"path/filepath"
	"sort"

	"github.com/rs/zerolog/log"

	"github.com/colonyops/hive/internal/core/git"
)

// DiscoveredRepo represents a git repository found during scanning.
type DiscoveredRepo struct {
	Path   string // absolute path to the repository
	Name   string // directory name
	Remote string // origin remote URL
}

// ScanRepoDirs scans the given directories for git repositories.
// Each directory in dirs is expected to contain subdirectories that are git repos.
// Repositories that fail to scan are silently skipped.
func ScanRepoDirs(ctx context.Context, dirs []string, gitExec git.Git) ([]DiscoveredRepo, error) {
	var repos []DiscoveredRepo

	for _, dir := range dirs {
		if len(dir) > 0 && dir[0] == '~' {
			home, err := os.UserHomeDir()
			if err != nil {
				log.Debug().Err(err).Str("dir", dir).Msg("failed to resolve home directory, skipping")
				continue
			}
			dir = filepath.Join(home, dir[1:])
		}

		entries, err := os.ReadDir(dir)
		if err != nil {
			log.Debug().Err(err).Str("dir", dir).Msg("failed to read repo directory, skipping")
			continue
		}

		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}

			repoPath := filepath.Join(dir, entry.Name())
			gitDir := filepath.Join(repoPath, ".git")

			// Check if .git exists (file or directory for worktrees)
			if _, err := os.Stat(gitDir); err != nil {
				continue
			}

			remote, err := gitExec.RemoteURL(ctx, repoPath)
			if err != nil {
				continue
			}

			repos = append(repos, DiscoveredRepo{
				Path:   repoPath,
				Name:   entry.Name(),
				Remote: remote,
			})
		}
	}

	sort.Slice(repos, func(i, j int) bool {
		return repos[i].Name < repos[j].Name
	})

	return repos, nil
}
