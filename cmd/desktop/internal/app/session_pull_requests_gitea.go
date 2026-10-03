package app

import (
	"context"

	"github.com/colonyops/hive/cmd/desktop/internal/app/sources/gitea"
)

// giteaForge looks a branch's pull request up on a connected Gitea or Forgejo
// instance. Unlike GitHub's there is no fixed host: the forge serves whatever
// hosts the connected accounts are bound to, so a session on an instance
// nobody has connected stays unsupported rather than disconnected — nothing
// identifies that host as Gitea to begin with.
type giteaForge struct {
	pulls *gitea.PullRequests
}

func newGiteaForge(pulls *gitea.PullRequests) *giteaForge {
	return &giteaForge{pulls: pulls}
}

func (g *giteaForge) serves(host string) bool {
	return g.pulls != nil && g.pulls.Serves(host)
}

func (g *giteaForge) pullRequest(ctx context.Context, key SessionPullRequestKey) (SessionPullRequest, error) {
	pull, found, err := g.pulls.ForBranch(ctx, key.Host, key.Owner, key.Repo, key.Branch)
	if err != nil {
		return SessionPullRequest{}, Wrap(err, KindInternal, "reading the pull request for %s", key.Branch)
	}
	if !found {
		return SessionPullRequest{Status: PullRequestStatusNone}, nil
	}
	return SessionPullRequest{
		Status:         PullRequestStatusFound,
		Number:         pull.Number,
		Title:          pull.Title,
		State:          pull.State,
		IsDraft:        pull.IsDraft,
		URL:            pull.URL,
		ReviewDecision: pull.ReviewDecision,
		Checks:         string(pull.Checks),
		Additions:      pull.Additions,
		Deletions:      pull.Deletions,
	}, nil
}
