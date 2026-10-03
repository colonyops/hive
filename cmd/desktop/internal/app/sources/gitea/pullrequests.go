package gitea

import (
	"context"
	"net/url"
	"strings"

	"github.com/colonyops/hive/cmd/desktop/internal/app/sources/gitea/giteaclient"
	"github.com/colonyops/hive/internal/platform/credentials"
)

// PullRequests resolves a session branch's pull request on whichever connected
// instance serves the remote's host.
//
// Which instance that is only the connected accounts can say: a remote names a
// host, and nothing about a host identifies it as Gitea until an account
// bound to it is present. That is also what keeps a session on some third
// forge from being looked up here.
type PullRequests struct {
	instances *InstanceStore
	creds     credentials.Store
	fetchers  *Fetchers
}

func NewPullRequests(instances *InstanceStore, creds credentials.Store, fetchers *Fetchers) *PullRequests {
	return &PullRequests{instances: instances, creds: creds, fetchers: fetchers}
}

// Serves reports whether any connected account's instance answers for host.
func (p *PullRequests) Serves(host string) bool {
	return len(p.accounts(host)) > 0
}

// ForBranch resolves the branch's pull request on the first connected account
// that can see it. found is false when no account's instance has one, which is
// not the same as the lookup having failed — that arrives as an error.
//
// Several accounts on one instance are asked in turn for the same reason the
// GitHub path tries every token: a repository one account cannot see is not an
// error there, so the only way to know another can see it is to ask.
func (p *PullRequests) ForBranch(ctx context.Context, host, owner, repo, branch string) (giteaclient.PullRequest, bool, error) {
	var lastErr error
	for _, ref := range p.accounts(host) {
		fetcher := p.fetchers.For(ref)
		client, err := fetcher.prepare()
		if err != nil {
			lastErr = err
			continue
		}
		pull, found, err := client.PullRequestForBranch(ctx, owner, repo, branch)
		if err != nil {
			fetcher.noteError(err)
			lastErr = err
			continue
		}
		if found {
			return pull, true, nil
		}
	}
	return giteaclient.PullRequest{}, false, lastErr
}

// accounts lists every connected account whose instance is hosted at host.
// The comparison drops the port: a remote carries the git transport's port,
// which is not the one the API is served on.
func (p *PullRequests) accounts(host string) []credentials.Ref {
	host = strings.ToLower(strings.TrimSpace(host))
	if host == "" {
		return nil
	}
	refs, err := credentials.ListProvider(p.creds, Provider)
	if err != nil {
		return nil
	}

	var matched []credentials.Ref
	for _, ref := range refs {
		binding, err := p.instances.Get(ref)
		if err != nil || binding.URL == "" {
			continue
		}
		parsed, err := url.Parse(binding.URL)
		if err != nil {
			continue
		}
		if strings.EqualFold(parsed.Hostname(), host) {
			matched = append(matched, ref)
		}
	}
	return matched
}
