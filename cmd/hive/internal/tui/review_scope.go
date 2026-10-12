package tui

import (
	"github.com/colonyops/hive/cmd/hive/internal/config"
	review "github.com/colonyops/hive/cmd/hive/internal/tui/views/review"
	"github.com/colonyops/hive/internal/domain/drive"
	"github.com/colonyops/hive/internal/platform/git"
)

// remoteReviewScope is the review scope of the repository a git remote
// names, with its owner/repo key. ok is false when the remote names no
// repository that can be a space.
func remoteReviewScope(cfg *config.Config, remote string) (scope review.Scope, repoKey string, ok bool) {
	owner, repo := git.ExtractOwnerRepo(remote)
	if owner == "" || repo == "" {
		return review.Scope{}, "", false
	}
	space, err := drive.RepositorySpace(owner, repo)
	if err != nil {
		return review.Scope{}, "", false
	}
	return review.Scope{Space: space, Dir: cfg.RepoContextDir(owner, repo)}, space.Key, true
}

// sharedReviewScope is what the docs view lists outside any repository.
func sharedReviewScope(cfg *config.Config) review.Scope {
	return review.Scope{Space: drive.Shared, Dir: cfg.SharedContextDir()}
}
