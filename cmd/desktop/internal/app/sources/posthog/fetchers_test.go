package posthog

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/desktop/internal/app/credentials"
	"github.com/colonyops/hive/cmd/desktop/internal/app/sources/posthog/client"
	"github.com/colonyops/hive/cmd/desktop/internal/app/sources/sourcehttp"
)

const testProjectID = 42

func connectedFetcher(t *testing.T, baseURL string) (*fetcher, credentials.Ref) {
	t.Helper()
	creds := credentials.NewMemoryStore()
	projects := NewProjectStore(filepath.Join(t.TempDir(), "posthog-projects.json"))
	ref := credentials.Ref{Provider: Provider, Account: "us.posthog.com-42"}
	require.NoError(t, creds.Set(ref, "phx-token"))
	require.NoError(t, projects.Set(ref, Binding{URL: baseURL, ProjectID: testProjectID, Name: "Acme"}))
	return NewFetchers(projects, creds, zerolog.Nop()).For(ref), ref
}

func TestFetcherRequiresAConnectedProject(t *testing.T) {
	t.Parallel()

	creds := credentials.NewMemoryStore()
	projects := NewProjectStore(filepath.Join(t.TempDir(), "posthog-projects.json"))
	ref := credentials.Ref{Provider: Provider, Account: "us.posthog.com-42"}
	require.NoError(t, creds.Set(ref, "phx-token")) // key but no stored binding

	_, _, err := NewFetchers(projects, creds, zerolog.Nop()).For(ref).Alerts(t.Context())
	assert.ErrorContains(t, err, "not connected")
}

// A binding with a host but no project id is as unusable as no binding at all:
// every API path is project-scoped, so /api/projects/0/... would 404 rather
// than report the real problem.
func TestFetcherRequiresAProjectID(t *testing.T) {
	t.Parallel()

	creds := credentials.NewMemoryStore()
	projects := NewProjectStore(filepath.Join(t.TempDir(), "posthog-projects.json"))
	ref := credentials.Ref{Provider: Provider, Account: "us.posthog.com-42"}
	require.NoError(t, creds.Set(ref, "phx-token"))
	require.NoError(t, projects.Set(ref, Binding{URL: "https://us.posthog.com"}))

	_, _, err := NewFetchers(projects, creds, zerolog.Nop()).For(ref).Alerts(t.Context())
	assert.ErrorContains(t, err, "not connected")
}

func TestFetcherRequiresAKey(t *testing.T) {
	t.Parallel()

	creds := credentials.NewMemoryStore()
	projects := NewProjectStore(filepath.Join(t.TempDir(), "posthog-projects.json"))
	ref := credentials.Ref{Provider: Provider, Account: "us.posthog.com-42"}
	require.NoError(t, projects.Set(ref, Binding{URL: "https://us.posthog.com", ProjectID: testProjectID}))

	_, _, err := NewFetchers(projects, creds, zerolog.Nop()).For(ref).Alerts(t.Context())
	assert.ErrorIs(t, err, sourcehttp.ErrUnauthorized)
}

func TestFetcherArmsCooldownAndSkips(t *testing.T) {
	t.Parallel()

	var hits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.Header().Set("Retry-After", "60")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()

	fx, _ := connectedFetcher(t, server.URL)

	_, _, err := fx.Alerts(t.Context())
	require.ErrorIs(t, err, sourcehttp.ErrRateLimited)
	require.EqualValues(t, 1, hits.Load())

	// The cooldown is armed, so the next poll must not reach the server.
	_, _, err = fx.Alerts(t.Context())
	require.ErrorIs(t, err, sourcehttp.ErrRateLimited)
	assert.EqualValues(t, 1, hits.Load(), "a fetcher in cooldown must not hit the project again")

	// Clearing the cooldown (as connect/disconnect does) lets it poll again.
	fx.clearCooldown()
	_, _, err = fx.Alerts(t.Context())
	require.Error(t, err)
	assert.EqualValues(t, 2, hits.Load())
}

// PostHog's per-minute limits are shared across every endpoint for one key, so
// a 429 on the issue query must also hold the alert poll back.
func TestCooldownIsSharedAcrossEndpoints(t *testing.T) {
	t.Parallel()

	var hits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()

	fx, _ := connectedFetcher(t, server.URL)

	_, _, err := fx.Issues(t.Context(), client.IssuesRequest{})
	require.ErrorIs(t, err, sourcehttp.ErrRateLimited)
	require.EqualValues(t, 1, hits.Load())

	_, _, err = fx.Alerts(t.Context())
	require.ErrorIs(t, err, sourcehttp.ErrRateLimited)
	assert.EqualValues(t, 1, hits.Load(), "one key's cooldown covers every endpoint it fetches through")
}

func TestFetchersReuseOnePerProject(t *testing.T) {
	t.Parallel()

	fetchers := NewFetchers(NewProjectStore(filepath.Join(t.TempDir(), "p.json")), credentials.NewMemoryStore(), zerolog.Nop())
	ref := credentials.Ref{Provider: Provider, Account: "us.posthog.com-42"}
	first, second := fetchers.For(ref), fetchers.For(ref)
	assert.Same(t, first, second, "the same project shares one fetcher, so its cooldown persists")
}

// Two projects on one host are separate accounts, so a rate limit on one must
// not stall the other — that is the multi-project routing case.
func TestFetchersAreDistinctPerProject(t *testing.T) {
	t.Parallel()

	fetchers := NewFetchers(NewProjectStore(filepath.Join(t.TempDir(), "p.json")), credentials.NewMemoryStore(), zerolog.Nop())
	dev := fetchers.For(credentials.Ref{Provider: Provider, Account: "us.posthog.com-1"})
	prod := fetchers.For(credentials.Ref{Provider: Provider, Account: "us.posthog.com-2"})
	assert.NotSame(t, dev, prod)
}
