package app

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/desktop/internal/app/sources/relay"
	"github.com/colonyops/hive/internal/platform/credentials"
)

func newTestRelayMCPResolver(t *testing.T) (*relayMCPResolver, credentials.Store, *relay.InstanceStore) {
	t.Helper()
	creds := credentials.NewMemoryStore()
	instances := relay.NewInstanceStore(filepath.Join(t.TempDir(), "relay-instances.json"))
	auth := relay.NewAuthenticator(creds, instances, "test", zerolog.Nop(), nil)
	fetchers := relay.NewFetchers(instances, creds, "test", zerolog.Nop())
	return &relayMCPResolver{accounts: auth.Accounts, clientFor: fetchers.For, creds: creds}, creds, instances
}

func TestRelayMCPResolverMintsOnceAndReuses(t *testing.T) {
	var minted atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/tokens" || r.Header.Get("Authorization") != "Bearer account-key" {
			http.Error(w, `{"kind":"not_found"}`, http.StatusNotFound)
			return
		}
		var body struct{ Name string }
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Name != "workspace:demo" {
			http.Error(w, `{"kind":"invalid"}`, http.StatusUnprocessableEntity)
			return
		}
		minted.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"token":"ws-key","id":"tok-1","name":"workspace:demo","kind":"api"}`))
	}))
	t.Cleanup(srv.Close)

	resolver, creds, instances := newTestRelayMCPResolver(t)
	account := credentials.Ref{Provider: relay.Provider, Account: "relay.test-me@example.com"}
	require.NoError(t, creds.Set(account, "account-key"))
	require.NoError(t, instances.Set(account, relay.Binding{URL: srv.URL, Email: "me@example.com"}))

	base, headers, err := resolver.Resolve(t.Context(), "demo")
	require.NoError(t, err)
	assert.Equal(t, srv.URL, base)
	assert.Equal(t, map[string]string{"Authorization": "Bearer ws-key"}, headers)
	assert.EqualValues(t, 1, minted.Load())

	stored, err := creds.Get(credentials.Ref{Provider: relayMCPProvider, Account: account.Account + "-demo"})
	require.NoError(t, err)
	assert.Equal(t, "ws-key", stored)

	_, headers, err = resolver.Resolve(t.Context(), "demo")
	require.NoError(t, err)
	assert.Equal(t, "Bearer ws-key", headers["Authorization"])
	assert.EqualValues(t, 1, minted.Load(), "a second resolve reuses the stored key")
}

func TestRelayMCPResolverNeedsAConnectedAccount(t *testing.T) {
	resolver, _, _ := newTestRelayMCPResolver(t)
	_, _, err := resolver.Resolve(t.Context(), "demo")
	require.ErrorIs(t, err, errNoRelayAccount)
}
