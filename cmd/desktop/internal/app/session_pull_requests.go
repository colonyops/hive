package app

import (
	"context"
	"sync"
	"time"
)

// SessionPullRequestKey addresses the pull request a session's branch has.
// Host decides which forge is asked, so a lookup carries it rather than
// inferring one from owner and repo, which every forge spells the same.
type SessionPullRequestKey struct {
	Host   string
	Owner  string
	Repo   string
	Branch string
}

// PullRequestStatus is why a session has no pull request to show, or that it
// does. The four are kept apart deliberately: rendering "no pull request" for
// a failed lookup or a disconnected account states a different, wrong fact.
type PullRequestStatus string

const (
	PullRequestStatusNone         PullRequestStatus = "none"
	PullRequestStatusFound        PullRequestStatus = "found"
	PullRequestStatusDisconnected PullRequestStatus = "disconnected"
	PullRequestStatusUnsupported  PullRequestStatus = "unsupported"
)

// SessionPullRequest is the branch's pull request as the session status bar
// shows it. Everything below Status is meaningful only for
// PullRequestStatusFound.
type SessionPullRequest struct {
	Status  PullRequestStatus
	Number  int
	Title   string
	State   string
	IsDraft bool
	URL     string
	// ReviewDecision is GitHub's own vocabulary (APPROVED, CHANGES_REQUESTED,
	// REVIEW_REQUIRED), or empty when review is not required.
	ReviewDecision string
	// Checks is passing, pending, failing, or empty for a head commit with no
	// checks configured.
	Checks string
	// The pull request's own line counts, deliberately not the git status's:
	// those measure the working tree and drift as the branch moves on.
	Additions int
	Deletions int
	// Cached distinguishes "this just arrived" from "this was already known".
	// The bar animates only the former.
	Cached bool
}

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
	pullRequest(ctx context.Context, key SessionPullRequestKey) (SessionPullRequest, error)
}

// sessionPullRequests answers "what is this branch's pull request" for the
// session status bar, over the app's own API clients rather than a forge CLI —
// which caches an empty result on error and so cannot keep "no pull request"
// apart from "the lookup failed".
type sessionPullRequests struct {
	forges []forge

	mu     sync.Mutex
	cached map[SessionPullRequestKey]cachedPullRequest
	// now is the clock, injected so a test does not sleep out a TTL.
	now func() time.Time
}

type cachedPullRequest struct {
	view   SessionPullRequest
	readAt time.Time
}

func newSessionPullRequests(forges ...forge) *sessionPullRequests {
	return &sessionPullRequests{
		forges: forges,
		cached: map[SessionPullRequestKey]cachedPullRequest{},
		now:    time.Now,
	}
}

// Lookup resolves one branch's pull request, answering from cache while the
// entry is fresh. refresh discards the cached entry first, which is what a
// user clicking the badge asks for.
func (p *sessionPullRequests) Lookup(ctx context.Context, key SessionPullRequestKey, refresh bool) (SessionPullRequest, error) {
	if key.Host == "" || key.Owner == "" || key.Repo == "" || key.Branch == "" {
		// A remote that named no repository, or a branch that did not resolve —
		// neither is a failure the bar should report as one.
		return SessionPullRequest{Status: PullRequestStatusUnsupported}, nil
	}

	if !refresh {
		if view, ok := p.fresh(key); ok {
			return view, nil
		}
	}

	view, err := p.fetch(ctx, key)
	if err != nil {
		return SessionPullRequest{}, err
	}
	p.mu.Lock()
	p.cached[key] = cachedPullRequest{view: view, readAt: p.now()}
	p.mu.Unlock()
	return view, nil
}

func (p *sessionPullRequests) fresh(key SessionPullRequestKey) (SessionPullRequest, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	entry, ok := p.cached[key]
	if !ok || p.now().Sub(entry.readAt) > sessionPRCacheTTL {
		return SessionPullRequest{}, false
	}
	// Stamped on the way out: the same view is fresh the first time it is
	// returned and cached after.
	view := entry.view
	view.Cached = true
	return view, true
}

func (p *sessionPullRequests) fetch(ctx context.Context, key SessionPullRequestKey) (SessionPullRequest, error) {
	for _, f := range p.forges {
		if f.serves(key.Host) {
			return f.pullRequest(ctx, key)
		}
	}
	return SessionPullRequest{Status: PullRequestStatusUnsupported}, nil
}
