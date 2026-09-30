package app

import (
	"context"
	"sync"
	"time"

	"github.com/colonyops/hive/cmd/desktop/internal/app/dispatch"
)

// sessionPRCacheTTL bounds how stale a session's pull-request badge may be.
// The bar polls its git half far more often: that is local subprocesses, this
// is a network round trip against a shared rate limit.
const sessionPRCacheTTL = 5 * time.Minute

// forge is one hosting service's answer to "what is this branch's pull
// request". The status bar renders the view, not the forge, so a new one is an
// implementation here and nothing else.
type forge interface {
	// serves reports whether this forge answers for a remote's host. A host no
	// forge serves is what makes a session's lookup unsupported.
	serves(host string) bool
	pullRequest(ctx context.Context, key dispatch.SessionPullRequestKey) (dispatch.SessionPullRequest, error)
}

// sessionPullRequests answers "what is this branch's pull request" for the
// session status bar, over the app's own API clients rather than a forge CLI —
// which caches an empty result on error and so cannot keep "no pull request"
// apart from "the lookup failed".
type sessionPullRequests struct {
	forges []forge

	mu     sync.Mutex
	cached map[dispatch.SessionPullRequestKey]cachedPullRequest
	// now is the clock, injected so a test does not sleep out a TTL.
	now func() time.Time
}

type cachedPullRequest struct {
	view   dispatch.SessionPullRequest
	readAt time.Time
}

func newSessionPullRequests(forges ...forge) *sessionPullRequests {
	return &sessionPullRequests{
		forges: forges,
		cached: map[dispatch.SessionPullRequestKey]cachedPullRequest{},
		now:    time.Now,
	}
}

// Lookup resolves one branch's pull request, answering from cache while the
// entry is fresh. refresh discards the cached entry first, which is what a
// user clicking the badge asks for.
func (p *sessionPullRequests) Lookup(ctx context.Context, key dispatch.SessionPullRequestKey, refresh bool) (dispatch.SessionPullRequest, error) {
	if key.Host == "" || key.Owner == "" || key.Repo == "" || key.Branch == "" {
		// A remote that named no repository, or a branch that did not resolve —
		// neither is a failure the bar should report as one.
		return dispatch.SessionPullRequest{Status: dispatch.PullRequestStatusUnsupported}, nil
	}

	if !refresh {
		if view, ok := p.fresh(key); ok {
			return view, nil
		}
	}

	view, err := p.fetch(ctx, key)
	if err != nil {
		return dispatch.SessionPullRequest{}, err
	}
	p.mu.Lock()
	p.cached[key] = cachedPullRequest{view: view, readAt: p.now()}
	p.mu.Unlock()
	return view, nil
}

func (p *sessionPullRequests) fresh(key dispatch.SessionPullRequestKey) (dispatch.SessionPullRequest, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	entry, ok := p.cached[key]
	if !ok || p.now().Sub(entry.readAt) > sessionPRCacheTTL {
		return dispatch.SessionPullRequest{}, false
	}
	// Stamped on the way out: the same view is fresh the first time it is
	// returned and cached after.
	view := entry.view
	view.Cached = true
	return view, true
}

func (p *sessionPullRequests) fetch(ctx context.Context, key dispatch.SessionPullRequestKey) (dispatch.SessionPullRequest, error) {
	for _, f := range p.forges {
		if f.serves(key.Host) {
			return f.pullRequest(ctx, key)
		}
	}
	return dispatch.SessionPullRequest{Status: dispatch.PullRequestStatusUnsupported}, nil
}
