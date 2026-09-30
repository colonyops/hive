package grafana

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
	"github.com/colonyops/hive/cmd/desktop/internal/app/sources/sourcehttp"
)

func connectedFetcher(t *testing.T, baseURL string) (*fetcher, credentials.Ref) {
	t.Helper()
	creds := credentials.NewMemoryStore()
	stacks := NewStackStore(filepath.Join(t.TempDir(), "grafana-stacks.json"))
	ref := credentials.Ref{Provider: Provider, Account: "host-1"}
	require.NoError(t, creds.Set(ref, "token"))
	require.NoError(t, stacks.Set(ref, stackEntry{URL: baseURL}))
	return NewFetchers(stacks, creds, zerolog.Nop()).For(ref), ref
}

func TestFetcherRequiresAConnectedStack(t *testing.T) {
	t.Parallel()

	creds := credentials.NewMemoryStore()
	stacks := NewStackStore(filepath.Join(t.TempDir(), "grafana-stacks.json"))
	ref := credentials.Ref{Provider: Provider, Account: "host-1"}
	require.NoError(t, creds.Set(ref, "token")) // token but no stored stack URL

	_, err := NewFetchers(stacks, creds, zerolog.Nop()).For(ref).Query(t.Context(), "ds", "up")
	assert.ErrorContains(t, err, "not connected")
}

func TestFetcherRequiresAToken(t *testing.T) {
	t.Parallel()

	creds := credentials.NewMemoryStore()
	stacks := NewStackStore(filepath.Join(t.TempDir(), "grafana-stacks.json"))
	ref := credentials.Ref{Provider: Provider, Account: "host-1"}
	require.NoError(t, stacks.Set(ref, stackEntry{URL: "https://grafana.example.com"})) // URL but no token

	_, err := NewFetchers(stacks, creds, zerolog.Nop()).For(ref).Query(t.Context(), "ds", "up")
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

	_, err := fx.Query(t.Context(), "ds", "up")
	require.ErrorIs(t, err, sourcehttp.ErrRateLimited)
	require.EqualValues(t, 1, hits.Load())

	// The cooldown is armed, so the next poll must not reach the server.
	_, err = fx.Query(t.Context(), "ds", "up")
	require.ErrorIs(t, err, sourcehttp.ErrRateLimited)
	assert.EqualValues(t, 1, hits.Load(), "a fetcher in cooldown must not hit the stack again")

	// Clearing the cooldown (as connect/disconnect does) lets it poll again.
	fx.invalidate()
	_, _ = fx.Query(t.Context(), "ds", "up")
	assert.EqualValues(t, 2, hits.Load())
}

func TestFetchersReuseOnePerStack(t *testing.T) {
	t.Parallel()

	fetchers := NewFetchers(NewStackStore(filepath.Join(t.TempDir(), "s.json")), credentials.NewMemoryStore(), zerolog.Nop())
	ref := credentials.Ref{Provider: Provider, Account: "host-1"}
	first := fetchers.For(ref)
	second := fetchers.For(ref)
	assert.Same(t, first, second, "the same stack shares one fetcher, so its cooldown persists")
}
