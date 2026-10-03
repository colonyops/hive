package grafana

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/desktop/internal/app/sources/grafana/client"
	"github.com/colonyops/hive/internal/platform/credentials"
)

func TestNormalizeStackURL(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		in       string
		wantBase string
		wantHost string
		wantErr  bool
	}{
		{name: "https root", in: "https://play.grafana.org", wantBase: "https://play.grafana.org", wantHost: "play.grafana.org"},
		{name: "trailing slash trimmed", in: "https://play.grafana.org/", wantBase: "https://play.grafana.org", wantHost: "play.grafana.org"},
		{name: "subpath kept", in: "https://ops.example.com/grafana/", wantBase: "https://ops.example.com/grafana", wantHost: "ops.example.com"},
		{name: "uppercased host", in: "https://Play.Grafana.ORG", wantBase: "https://Play.Grafana.ORG", wantHost: "play.grafana.org"},
		{name: "loopback http allowed", in: "http://localhost:3000", wantBase: "http://localhost:3000", wantHost: "localhost:3000"},
		{name: "loopback ip http allowed", in: "http://127.0.0.1:3000", wantBase: "http://127.0.0.1:3000", wantHost: "127.0.0.1:3000"},
		{name: "remote http rejected", in: "http://example.com", wantErr: true},
		{name: "userinfo rejected", in: "https://user:pass@example.com", wantErr: true},
		{name: "non-http scheme rejected", in: "ftp://example.com", wantErr: true},
		{name: "no host rejected", in: "https://", wantErr: true},
		{name: "empty rejected", in: "   ", wantErr: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			base, host, err := normalizeStackURL(tc.in)
			if tc.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.wantBase, base)
			assert.Equal(t, tc.wantHost, host)
		})
	}
}

func TestAccountID(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "play.grafana.org-1", accountID("play.grafana.org", 1))
	assert.Equal(t, "localhost:3000-2", accountID("localhost:3000", 2))
}

// orgServer serves GET /api/org/ as one org, returning 401 when the token is
// not "good".
func orgServer(t *testing.T, id int, name string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer good" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_ = json.NewEncoder(w).Encode(client.Org{ID: id, Name: name})
	}))
}

// testAuth bundles an authenticator with the stores behind it and a counter of
// change notifications, so a test can assert what a connect or disconnect
// persisted and announced.
type testAuth struct {
	auth    *Authenticator
	creds   credentials.Store
	stacks  *StackStore
	changes *int
}

func newTestAuth(t *testing.T) testAuth {
	t.Helper()
	creds := credentials.NewMemoryStore()
	stacks := NewStackStore(filepath.Join(t.TempDir(), "grafana-stacks.json"))
	changes := 0
	auth := NewAuthenticator(creds, stacks, zerolog.Nop(), func(credentials.Ref) { changes++ })
	return testAuth{auth: auth, creds: creds, stacks: stacks, changes: &changes}
}

func TestConnectStoresTokenAndURL(t *testing.T) {
	t.Parallel()

	server := orgServer(t, 1, "Main Org.")
	defer server.Close()
	env := newTestAuth(t)

	stack, err := env.auth.Connect(t.Context(), server.URL, "good")
	require.NoError(t, err)
	assert.Equal(t, accountID(hostOf(t, server.URL), 1), stack.Account)
	assert.Equal(t, 1, stack.OrgID)
	assert.Equal(t, "Main Org.", stack.OrgName)
	assert.Equal(t, 1, *env.changes, "a successful connect announces the change once")

	ref := credentials.Ref{Provider: Provider, Account: stack.Account}
	token, err := env.creds.Get(ref)
	require.NoError(t, err)
	assert.Equal(t, "good", token, "the token lands in the credential store")

	entry, err := env.stacks.Get(ref)
	require.NoError(t, err)
	assert.Equal(t, server.URL, entry.URL, "the non-secret stack URL is persisted")
}

func TestConnectRejectsBadTokenWithoutStoring(t *testing.T) {
	t.Parallel()

	server := orgServer(t, 1, "Main Org.")
	defer server.Close()
	env := newTestAuth(t)

	_, err := env.auth.Connect(t.Context(), server.URL, "wrong")
	require.Error(t, err)

	// Nothing is persisted for a rejected paste.
	ref := credentials.Ref{Provider: Provider, Account: accountID(hostOf(t, server.URL), 1)}
	_, err = env.creds.Get(ref)
	require.ErrorIs(t, err, credentials.ErrNotFound)
	entry, _ := env.stacks.Get(ref)
	assert.Empty(t, entry.URL)
}

// A validated token is stored before the stack URL. If that URL write fails,
// the token must be rolled back rather than left orphaned in the keychain with
// no URL to fetch against.
func TestConnectRollsBackTokenWhenURLWriteFails(t *testing.T) {
	t.Parallel()

	server := orgServer(t, 1, "Main Org.")
	defer server.Close()

	creds, stacks := storesWithUnwritableStackPath(t)
	auth := NewAuthenticator(creds, stacks, zerolog.Nop(), func(credentials.Ref) {})

	_, err := auth.Connect(t.Context(), server.URL, "good")
	require.Error(t, err, "a failed URL write fails the connect")

	ref := credentials.Ref{Provider: Provider, Account: accountID(hostOf(t, server.URL), 1)}
	_, err = creds.Get(ref)
	require.ErrorIs(t, err, credentials.ErrNotFound, "the token is rolled back when the URL cannot be stored")
}

func TestConnectRestoresPreviousTokenWhenURLWriteFails(t *testing.T) {
	t.Parallel()

	server := orgServer(t, 1, "Main Org.")
	defer server.Close()

	creds, stacks := storesWithUnwritableStackPath(t)
	ref := credentials.Ref{Provider: Provider, Account: accountID(hostOf(t, server.URL), 1)}
	require.NoError(t, creds.Set(ref, "old"))
	auth := NewAuthenticator(creds, stacks, zerolog.Nop(), func(credentials.Ref) {})

	_, err := auth.Connect(t.Context(), server.URL, "good")
	require.Error(t, err, "a failed URL write fails the connect")

	token, err := creds.Get(ref)
	require.NoError(t, err)
	assert.Equal(t, "old", token, "the previous token is restored when a reconnect cannot store the URL")
}

func TestConnectRejectsRemoteHTTP(t *testing.T) {
	t.Parallel()
	env := newTestAuth(t)
	_, err := env.auth.Connect(t.Context(), "http://grafana.example.com", "good")
	assert.Error(t, err, "a token must never be sent to a non-loopback host over http")
}

func TestDisconnectRemovesTokenAndURL(t *testing.T) {
	t.Parallel()

	server := orgServer(t, 1, "Main Org.")
	defer server.Close()
	env := newTestAuth(t)

	stack, err := env.auth.Connect(t.Context(), server.URL, "good")
	require.NoError(t, err)

	require.NoError(t, env.auth.Disconnect(t.Context(), stack.Account))

	ref := credentials.Ref{Provider: Provider, Account: stack.Account}
	_, err = env.creds.Get(ref)
	require.ErrorIs(t, err, credentials.ErrNotFound)
	entry, _ := env.stacks.Get(ref)
	assert.Empty(t, entry.URL)
}

func TestDisconnectIsIdempotent(t *testing.T) {
	t.Parallel()
	env := newTestAuth(t)
	assert.NoError(t, env.auth.Disconnect(t.Context(), "never.connected-1"))
}

func storesWithUnwritableStackPath(t *testing.T) (credentials.Store, *StackStore) {
	t.Helper()
	blocker := filepath.Join(t.TempDir(), "blocker")
	require.NoError(t, os.WriteFile(blocker, []byte("x"), 0o600))
	return credentials.NewMemoryStore(), NewStackStore(filepath.Join(blocker, "grafana-stacks.json"))
}

// hostOf returns the lowercased host of a URL, matching normalizeStackURL.
func hostOf(t *testing.T, raw string) string {
	t.Helper()
	_, host, err := normalizeStackURL(raw)
	require.NoError(t, err)
	return host
}
