package app

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/colonyops/hive/cmd/desktop/internal/app/dispatch"
	"github.com/colonyops/hive/cmd/desktop/internal/app/sources/github/ghclient"
	"github.com/colonyops/hive/internal/platform/credentials"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func connectedStore(t *testing.T) credentials.Store {
	t.Helper()
	store := credentials.NewMemoryStore()
	require.NoError(t, store.Set(credentials.Ref{Provider: "github", Account: "octocat"}, "tok"))
	return store
}

func graphQLServer(t *testing.T, body string) (*httptest.Server, *atomic.Int64) {
	t.Helper()
	var calls atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(server.Close)
	return server, &calls
}

const openPRBody = `{"data":{"r0":{"pullRequests":{"nodes":[{"number":311,"state":"OPEN","isDraft":true,
  "url":"https://github.com/acme/site/pull/311","reviewDecision":"REVIEW_REQUIRED",
  "commits":{"nodes":[{"commit":{"statusCheckRollup":{"state":"PENDING"}}}]}}]}}}}`

func TestSessionPullRequestsAnswersFromCacheUntilRefreshed(t *testing.T) {
	server, calls := graphQLServer(t, openPRBody)
	lookup := newSessionPullRequests(newGitHubForge(ghclient.NewClient(ghclient.WithAPIBase(server.URL)), connectedStore(t)))
	key := dispatch.SessionPullRequestKey{Host: "github.com", Owner: "acme", Repo: "site", Branch: "feat/bar"}

	first, err := lookup.Lookup(t.Context(), key, false)
	require.NoError(t, err)
	assert.Equal(t, dispatch.SessionPullRequest{
		Status: dispatch.PullRequestStatusFound, Number: 311, State: "OPEN", IsDraft: true,
		URL: "https://github.com/acme/site/pull/311", ReviewDecision: "REVIEW_REQUIRED", Checks: "pending",
	}, first)

	assert.False(t, first.Cached, "the read that fetched it is not a cached answer")

	second, err := lookup.Lookup(t.Context(), key, false)
	require.NoError(t, err)
	assert.Equal(t, int64(1), calls.Load(), "a fresh entry must not cost a second round trip")
	// The bar animates the pull request in only when it actually just arrived,
	// so "came from cache" has to be visible to the caller.
	assert.True(t, second.Cached)

	_, err = lookup.Lookup(t.Context(), key, true)
	require.NoError(t, err)
	assert.Equal(t, int64(2), calls.Load(), "refresh is what a user clicking the badge asks for")
}

func TestSessionPullRequestsRereadsOnceTheEntryIsStale(t *testing.T) {
	server, calls := graphQLServer(t, openPRBody)
	lookup := newSessionPullRequests(newGitHubForge(ghclient.NewClient(ghclient.WithAPIBase(server.URL)), connectedStore(t)))
	now := time.Now()
	lookup.now = func() time.Time { return now }
	key := dispatch.SessionPullRequestKey{Host: "github.com", Owner: "acme", Repo: "site", Branch: "feat/bar"}

	_, err := lookup.Lookup(t.Context(), key, false)
	require.NoError(t, err)
	now = now.Add(sessionPRCacheTTL + time.Second)
	_, err = lookup.Lookup(t.Context(), key, false)
	require.NoError(t, err)
	assert.Equal(t, int64(2), calls.Load())
}

// The three reasons there is nothing to show stay apart. Collapsing any of them
// into "no pull request" tells the user a fact about their branch that is not
// true — the failure mode the `gh`-backed path has, since it caches an empty
// result on error for the whole TTL.
func TestSessionPullRequestsKeepsItsEmptyAnswersDistinct(t *testing.T) {
	server, _ := graphQLServer(t, `{"data":{"r0":{"pullRequests":{"nodes":[]}}}}`)
	client := ghclient.NewClient(ghclient.WithAPIBase(server.URL))
	key := dispatch.SessionPullRequestKey{Host: "github.com", Owner: "acme", Repo: "site", Branch: "feat/bar"}

	none, err := newSessionPullRequests(newGitHubForge(client, connectedStore(t))).Lookup(t.Context(), key, false)
	require.NoError(t, err)
	assert.Equal(t, dispatch.PullRequestStatusNone, none.Status)

	disconnected, err := newSessionPullRequests(newGitHubForge(client, credentials.NewMemoryStore())).Lookup(t.Context(), key, false)
	require.NoError(t, err)
	assert.Equal(t, dispatch.PullRequestStatusDisconnected, disconnected.Status)

	unsupported, err := newSessionPullRequests(newGitHubForge(client, connectedStore(t))).Lookup(t.Context(),
		dispatch.SessionPullRequestKey{Branch: "feat/bar"}, false)
	require.NoError(t, err)
	assert.Equal(t, dispatch.PullRequestStatusUnsupported, unsupported.Status)

	// A host no forge serves is unsupported too, and never disconnected: a
	// remote is not evidence that its host is a forge the app can ask.
	unknownHost, err := newSessionPullRequests(newGitHubForge(client, connectedStore(t))).Lookup(t.Context(),
		dispatch.SessionPullRequestKey{Host: "git.example.test", Owner: "acme", Repo: "site", Branch: "feat/bar"}, false)
	require.NoError(t, err)
	assert.Equal(t, dispatch.PullRequestStatusUnsupported, unknownHost.Status)
}

// A failed lookup is an error, never a cached "none" — and nothing is cached,
// so the next poll retries instead of showing a blank badge for the TTL.
func TestSessionPullRequestsReportsAFailedLookupAndCachesNothing(t *testing.T) {
	var calls atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"message":"Bad credentials"}`))
	}))
	defer server.Close()

	lookup := newSessionPullRequests(newGitHubForge(ghclient.NewClient(ghclient.WithAPIBase(server.URL)), connectedStore(t)))
	key := dispatch.SessionPullRequestKey{Host: "github.com", Owner: "acme", Repo: "site", Branch: "feat/bar"}

	_, err := lookup.Lookup(t.Context(), key, false)
	require.Error(t, err)
	_, err = lookup.Lookup(t.Context(), key, false)
	require.Error(t, err)
	assert.Equal(t, int64(2), calls.Load())
}
