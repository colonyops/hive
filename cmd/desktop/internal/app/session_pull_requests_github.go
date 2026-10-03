package app

import (
	"context"

	ghsource "github.com/colonyops/hive/cmd/desktop/internal/app/sources/github"
	"github.com/colonyops/hive/cmd/desktop/internal/app/sources/github/ghclient"
	"github.com/colonyops/hive/internal/platform/credentials"
)

// gitHubForge looks a branch's pull request up on github.com. GitHub
// Enterprise is not it: an instance is reached at its own host, and the app has
// no configuration naming one.
type gitHubForge struct {
	client *ghclient.Client
	creds  credentials.Store
}

func newGitHubForge(client *ghclient.Client, creds credentials.Store) *gitHubForge {
	return &gitHubForge{client: client, creds: creds}
}

func (g *gitHubForge) serves(host string) bool { return host == "github.com" }

func (g *gitHubForge) pullRequest(ctx context.Context, key SessionPullRequestKey) (SessionPullRequest, error) {
	if g.client == nil || g.creds == nil {
		return SessionPullRequest{Status: PullRequestStatusDisconnected}, nil
	}

	tokens, err := g.tokens()
	if err != nil {
		return SessionPullRequest{}, Wrap(err, KindInternal, "reading GitHub credentials")
	}
	if len(tokens) == 0 {
		return SessionPullRequest{Status: PullRequestStatusDisconnected}, nil
	}

	ref := ghclient.BranchRef{Owner: key.Owner, Repo: key.Repo, Branch: key.Branch}
	// A repository an account cannot see resolves to a null alias, not an
	// error, so the only way to know another account can see it is to ask.
	var lastErr error
	for _, token := range tokens {
		results, err := g.client.WithTokenCopy(token).PullRequestsByBranch(ctx, []ghclient.BranchRef{ref})
		if err != nil {
			lastErr = err
			continue
		}
		if len(results) == 0 || !results[0].Found {
			continue
		}
		pr := results[0]
		return SessionPullRequest{
			Status:         PullRequestStatusFound,
			Number:         pr.Number,
			Title:          pr.Title,
			State:          pr.State,
			IsDraft:        pr.IsDraft,
			URL:            pr.URL,
			ReviewDecision: pr.ReviewDecision,
			Checks:         string(pr.Checks),
			Additions:      pr.Additions,
			Deletions:      pr.Deletions,
		}, nil
	}
	if lastErr != nil {
		return SessionPullRequest{}, Wrap(lastErr, KindInternal, "reading the pull request for %s", key.Branch)
	}
	return SessionPullRequest{Status: PullRequestStatusNone}, nil
}

// tokens lists every token that could see the repository. The env override is
// provider-wide and names no account, so a headless run stores no ref at all;
// the synthetic ref exists only to give credentials.Resolve something
// well-formed to answer it with.
func (g *gitHubForge) tokens() ([]string, error) {
	refs, err := credentials.ListProvider(g.creds, ghsource.Provider)
	if err != nil {
		return nil, err
	}
	if len(refs) == 0 && credentials.HasEnvOverride(ghsource.Provider) {
		refs = []credentials.Ref{{Provider: ghsource.Provider, Account: "env"}}
	}
	tokens := make([]string, 0, len(refs))
	for _, ref := range refs {
		token, err := credentials.Resolve(g.creds, ref)
		if err != nil || token == "" {
			continue
		}
		tokens = append(tokens, token)
	}
	return tokens, nil
}
