package gitea

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/desktop/internal/app/credentials"
)

// giteaServer answers the two calls Connect makes. handler, when set, runs
// first and may take over the response.
func giteaServer(t *testing.T, login string, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if handler != nil {
			handler(w, r)
			if w.Header().Get("X-Handled") != "" {
				return
			}
		}
		switch r.URL.Path {
		case "/api/v1/version":
			_, _ = w.Write([]byte(`{"version":"1.27.1"}`))
		case "/api/v1/user":
			_, _ = w.Write([]byte(`{"login":"` + login + `","full_name":"Octo Cat"}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)
	return server
}

func newAuthenticator(t *testing.T) (*Authenticator, credentials.Store, *InstanceStore) {
	t.Helper()
	creds := credentials.NewMemoryStore()
	instances := NewInstanceStore(filepath.Join(t.TempDir(), "gitea-instances.json"))
	return NewAuthenticator(creds, instances, zerolog.Nop(), nil), creds, instances
}

func TestConnectStoresTheTokenAndBinding(t *testing.T) {
	t.Parallel()

	server := giteaServer(t, "octocat", nil)
	auth, creds, instances := newAuthenticator(t)

	instance, err := auth.Connect(t.Context(), server.URL, "gta_token")
	require.NoError(t, err)

	// The account binds host and login: host alone collides across two accounts
	// on one server, login alone collides across servers.
	host := server.Listener.Addr().String()
	assert.Equal(t, host+"-octocat", instance.Account)
	assert.Equal(t, "octocat", instance.Login)
	assert.Equal(t, "1.27.1", instance.Version)

	ref := credentials.Ref{Provider: Provider, Account: instance.Account}
	token, err := creds.Get(ref)
	require.NoError(t, err)
	assert.Equal(t, "gta_token", token)

	binding, err := instances.Get(ref)
	require.NoError(t, err)
	assert.Equal(t, server.URL, binding.URL)
}

// The version probe runs first so a wrong URL is diagnosable: /api/v1/user
// alone would answer 401 and read as "your token was rejected".
func TestConnectRejectsAHostThatIsNotGitea(t *testing.T) {
	t.Parallel()

	server := giteaServer(t, "octocat", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/version" {
			w.Header().Set("X-Handled", "1")
			w.WriteHeader(http.StatusNotFound)
		}
	})
	auth, creds, _ := newAuthenticator(t)

	_, err := auth.Connect(t.Context(), server.URL, "gta_token")
	require.ErrorContains(t, err, "does not answer as a Gitea or Forgejo instance")

	refs, err := credentials.ListProvider(creds, Provider)
	require.NoError(t, err)
	assert.Empty(t, refs, "a rejected paste leaves nothing behind")
}

func TestConnectRejectsABadToken(t *testing.T) {
	t.Parallel()

	server := giteaServer(t, "octocat", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/user" {
			w.Header().Set("X-Handled", "1")
			w.WriteHeader(http.StatusUnauthorized)
		}
	})
	auth, creds, _ := newAuthenticator(t)

	_, err := auth.Connect(t.Context(), server.URL, "bad")
	require.ErrorContains(t, err, "rejected the token")

	refs, err := credentials.ListProvider(creds, Provider)
	require.NoError(t, err)
	assert.Empty(t, refs)
}

// Gitea validates any presented credential, so an invalid token 401s even the
// version probe — that must read as a token rejection, not as "this host is
// not a Gitea instance".
func TestConnectReportsATokenRejectionFromTheVersionProbe(t *testing.T) {
	t.Parallel()

	server := giteaServer(t, "octocat", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/version" {
			w.Header().Set("X-Handled", "1")
			w.WriteHeader(http.StatusUnauthorized)
		}
	})
	auth, creds, _ := newAuthenticator(t)

	_, err := auth.Connect(t.Context(), server.URL, "expired")
	require.ErrorContains(t, err, "rejected the token")

	refs, err := credentials.ListProvider(creds, Provider)
	require.NoError(t, err)
	assert.Empty(t, refs)
}

// A validated token is stored before the binding. If the binding write fails,
// the token must be rolled back rather than left orphaned in the keychain with
// no instance to fetch against.
func TestConnectRollsBackTokenWhenBindingWriteFails(t *testing.T) {
	t.Parallel()

	server := giteaServer(t, "octocat", nil)
	creds := credentials.NewMemoryStore()
	auth := NewAuthenticator(creds, unwritableInstanceStore(t), zerolog.Nop(), nil)

	_, err := auth.Connect(t.Context(), server.URL, "gta_token")
	require.Error(t, err, "a failed binding write fails the connect")

	refs, err := credentials.ListProvider(creds, Provider)
	require.NoError(t, err)
	assert.Empty(t, refs, "the token is rolled back when the binding cannot be stored")
}

func TestConnectRestoresPreviousTokenWhenBindingWriteFails(t *testing.T) {
	t.Parallel()

	server := giteaServer(t, "octocat", nil)
	creds := credentials.NewMemoryStore()
	host := server.Listener.Addr().String()
	ref := credentials.Ref{Provider: Provider, Account: accountID(host, "octocat")}
	require.NoError(t, creds.Set(ref, "old"))
	auth := NewAuthenticator(creds, unwritableInstanceStore(t), zerolog.Nop(), nil)

	_, err := auth.Connect(t.Context(), server.URL, "new")
	require.Error(t, err)

	token, err := creds.Get(ref)
	require.NoError(t, err)
	assert.Equal(t, "old", token, "the previous token is restored when a reconnect cannot store the binding")
}

func unwritableInstanceStore(t *testing.T) *InstanceStore {
	t.Helper()
	blocker := filepath.Join(t.TempDir(), "blocker")
	require.NoError(t, os.WriteFile(blocker, []byte("x"), 0o600))
	return NewInstanceStore(filepath.Join(blocker, "gitea-instances.json"))
}

func TestConnectRequiresAToken(t *testing.T) {
	t.Parallel()

	auth, _, _ := newAuthenticator(t)
	_, err := auth.Connect(t.Context(), "https://git.example.com", "   ")
	assert.ErrorContains(t, err, "token is empty")
}

func TestDisconnectRemovesBothHalvesAndIsIdempotent(t *testing.T) {
	t.Parallel()

	server := giteaServer(t, "octocat", nil)
	auth, creds, instances := newAuthenticator(t)

	instance, err := auth.Connect(t.Context(), server.URL, "gta_token")
	require.NoError(t, err)

	require.NoError(t, auth.Disconnect(t.Context(), instance.Account))

	refs, err := credentials.ListProvider(creds, Provider)
	require.NoError(t, err)
	assert.Empty(t, refs)

	binding, err := instances.Get(credentials.Ref{Provider: Provider, Account: instance.Account})
	require.NoError(t, err)
	assert.Empty(t, binding.URL)

	assert.NoError(t, auth.Disconnect(t.Context(), instance.Account), "disconnecting nothing disconnects cleanly")
}

func TestNormalizeInstanceURL(t *testing.T) {
	t.Parallel()

	for name, tc := range map[string]struct {
		raw      string
		wantBase string
		wantHost string
		wantErr  string
	}{
		"https":             {raw: "https://Git.Example.com", wantBase: "https://Git.Example.com", wantHost: "git.example.com"},
		"trailing slash":    {raw: "https://git.example.com/", wantBase: "https://git.example.com", wantHost: "git.example.com"},
		"subpath preserved": {raw: "https://example.com/git/", wantBase: "https://example.com/git", wantHost: "example.com"},
		"loopback http":     {raw: "http://localhost:3000", wantBase: "http://localhost:3000", wantHost: "localhost:3000"},
		"loopback ip":       {raw: "http://127.0.0.1:3000", wantBase: "http://127.0.0.1:3000", wantHost: "127.0.0.1:3000"},
		"remote http":       {raw: "http://git.example.com", wantErr: "must use https"},
		"other scheme":      {raw: "ssh://git.example.com", wantErr: "must use https"},
		"no host":           {raw: "git.example.com", wantErr: "no host"},
		"userinfo rejected": {raw: "https://user:pass@git.example.com", wantErr: "userinfo"},
		"empty":             {raw: "  ", wantErr: "instance URL is required"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			base, host, err := normalizeInstanceURL(tc.raw)
			if tc.wantErr != "" {
				assert.ErrorContains(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.wantBase, base)
			assert.Equal(t, tc.wantHost, host)
		})
	}
}
