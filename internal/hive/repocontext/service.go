// Package repocontext manages per-repository context directories.
package repocontext

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/colonyops/hive/pkg/logutils"

	"github.com/colonyops/hive/internal/config"
	"github.com/colonyops/hive/internal/platform/git"
	"github.com/rs/zerolog"
)

// Service manages per-repository context directories.
type Service struct {
	config *config.Config
	git    git.Git
	logger zerolog.Logger
}

// NewService creates a new Service.
func NewService(logger zerolog.Logger, cfg *config.Config, gitClient git.Git) *Service {
	return &Service{
		config: cfg,
		git:    gitClient,
		logger: logutils.Component(logger, "repocontext"),
	}
}

// ResolveDir determines the context directory for the given repo spec.
// If repo is "owner/repo", it resolves directly. If shared is true, returns the shared dir.
// Otherwise detects from the current directory's git remote.
func (c *Service) ResolveDir(ctx context.Context, repo string, shared bool) (string, error) {
	if shared {
		return c.config.SharedContextDir(), nil
	}

	if repo != "" {
		parts := strings.SplitN(repo, "/", 2)
		if len(parts) != 2 {
			return "", fmt.Errorf("invalid repo format, expected owner/repo: %s", repo)
		}
		return c.config.RepoContextDir(parts[0], parts[1]), nil
	}

	remote, err := c.git.RemoteURL(ctx, ".")
	if err != nil {
		return "", fmt.Errorf("detect remote (are you in a git repository?): %w", err)
	}

	owner, repoName := git.ExtractOwnerRepo(remote)
	if owner == "" || repoName == "" {
		return "", fmt.Errorf("could not extract owner/repo from remote: %s", remote)
	}

	return c.config.RepoContextDir(owner, repoName), nil
}

// Init creates the context directory and standard subdirectories.
// Returns the list of subdirectories that were newly created.
func (c *Service) Init(ctxDir string) ([]string, error) {
	if err := os.MkdirAll(ctxDir, 0o755); err != nil {
		return nil, fmt.Errorf("create context directory: %w", err)
	}

	subdirs := []string{"research", "plans", "references"}
	var created []string
	for _, subdir := range subdirs {
		subdirPath := filepath.Join(ctxDir, subdir)
		if _, err := os.Stat(subdirPath); os.IsNotExist(err) {
			if err := os.MkdirAll(subdirPath, 0o755); err != nil {
				return nil, fmt.Errorf("create subdirectory %s: %w", subdir, err)
			}
			created = append(created, subdir)
		}
	}

	return created, nil
}

// CreateSymlink creates a symlink from the current directory to the context directory.
// Returns true if the symlink already existed and pointed to the correct target.
func (c *Service) CreateSymlink(ctxDir string) (bool, error) {
	symlinkName := c.config.Context.SymlinkName
	symlinkPath := filepath.Join(".", symlinkName)

	if info, err := os.Lstat(symlinkPath); err == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			target, _ := os.Readlink(symlinkPath)
			if target == ctxDir {
				return true, nil
			}
			return false, fmt.Errorf("symlink %s exists but points to %s, not %s", symlinkName, target, ctxDir)
		}
		return false, fmt.Errorf("%s already exists and is not a symlink", symlinkName)
	}

	if err := os.Symlink(ctxDir, symlinkPath); err != nil {
		return false, fmt.Errorf("create symlink: %w", err)
	}

	return false, nil
}

// CanvasesDirName is the entry Hive Desktop keeps a repository's canvases in.
// Prune leaves it alone: a canvas is durable output, and the age of its
// directory says nothing about whether it is still wanted.
const CanvasesDirName = "canvases"

// Prune deletes files in the context directory older than the given duration.
// Returns the number of files removed.
func (c *Service) Prune(ctxDir string, olderThan time.Duration) (int, error) {
	cutoff := time.Now().Add(-olderThan)

	entries, err := os.ReadDir(ctxDir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, fmt.Errorf("read directory: %w", err)
	}

	count := 0
	for _, entry := range entries {
		if entry.Name() == CanvasesDirName {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}

		if info.ModTime().Before(cutoff) {
			path := filepath.Join(ctxDir, entry.Name())
			if err := os.RemoveAll(path); err != nil {
				c.logger.Warn().Err(err).Str("path", path).Msg("failed to remove context entry during prune")
				continue
			}
			count++
		}
	}

	return count, nil
}
