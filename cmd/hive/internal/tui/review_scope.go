package tui

import (
	"strings"

	"github.com/colonyops/hive/cmd/hive/internal/config"
	review "github.com/colonyops/hive/cmd/hive/internal/tui/views/review"
	"github.com/colonyops/hive/internal/domain/drive"
	"github.com/colonyops/hive/internal/platform/git"
)

// DriveSource is the drive as the TUI reads it: the review view's listing
// plus the spelling of a space key as it exists on disk, which a remote URL
// does not settle on a case-insensitive file system.
type DriveSource interface {
	review.Source
	CanonicalSpace(space drive.Space) drive.Space
}

// remoteReviewScope is the review scope of the repository a git remote
// names, with its owner/repo key. ok is false when the remote names no
// repository that can be a space.
func remoteReviewScope(cfg *config.Config, source DriveSource, remote string) (scope review.Scope, repoKey string, ok bool) {
	owner, repo := git.ExtractOwnerRepo(remote)
	if owner == "" || repo == "" {
		return review.Scope{}, "", false
	}
	space, err := drive.RepositorySpace(owner, repo)
	if err != nil {
		return review.Scope{}, "", false
	}
	if source != nil {
		space = source.CanonicalSpace(space)
		owner, repo, _ = strings.Cut(space.Key, "/")
	}
	return review.Scope{Space: space, Dir: cfg.RepoContextDir(owner, repo)}, space.Key, true
}

// sharedReviewScope is what the docs view lists outside any repository.
func sharedReviewScope(cfg *config.Config) review.Scope {
	return review.Scope{Space: drive.Shared, Dir: cfg.SharedContextDir()}
}
