package posthog

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/desktop/internal/app/sources/posthog/client"
	"github.com/colonyops/hive/internal/platform/credentials"
)

func newAuthenticator(t *testing.T) (*Authenticator, credentials.Store, *ProjectStore) {
	t.Helper()
	creds := credentials.NewMemoryStore()
	projects := NewProjectStore(filepath.Join(t.TempDir(), "posthog-projects.json"))
	return NewAuthenticator(creds, projects, zerolog.Nop(), nil), creds, projects
}

// pointAt makes the authenticator reach the test server regardless of the URL
// the caller passed, so host normalization can be asserted independently of
// where the request actually goes.
func pointAt(a *Authenticator, serverURL string) {
	a.newClient = func(_, token string) *client.Client { return client.NewClient(serverURL, token) }
}

func projectsServer(t *testing.T, body string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/projects/", r.URL.Path)
		_, _ = w.Write([]byte(body))
	}))
}

func TestNormalizeHostURL(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		raw      string
		wantBase string
		wantHost string
		wantErr  bool
	}{
		{name: "eu cloud", raw: "https://eu.posthog.com", wantBase: "https://eu.posthog.com", wantHost: "eu.posthog.com"},
		{name: "trailing slash", raw: "https://us.posthog.com/", wantBase: "https://us.posthog.com", wantHost: "us.posthog.com"},
		{name: "self hosted with a path", raw: "https://ph.example.com/posthog", wantBase: "https://ph.example.com/posthog", wantHost: "ph.example.com"},
		{name: "loopback over http", raw: "http://localhost:8000", wantBase: "http://localhost:8000", wantHost: "localhost:8000"},

		// The ingestion hosts are what every install snippet prints, so they
		// are what a user pastes. They answer the API with 401, which reads as
		// "bad key" rather than "wrong host".
		{name: "us ingestion host", raw: "https://us.i.posthog.com", wantBase: "https://us.posthog.com", wantHost: "us.posthog.com"},
		{name: "eu ingestion host", raw: "https://eu.i.posthog.com", wantBase: "https://eu.posthog.com", wantHost: "eu.posthog.com"},
		{name: "legacy app host", raw: "https://app.posthog.com", wantBase: "https://us.posthog.com", wantHost: "us.posthog.com"},
		{name: "ingestion host with a path", raw: "https://us.i.posthog.com/ingest", wantBase: "https://us.posthog.com", wantHost: "us.posthog.com"},

		{name: "empty", raw: "", wantErr: true},
		{name: "no scheme", raw: "us.posthog.com", wantErr: true},
		{name: "plain http off loopback", raw: "http://ph.example.com", wantErr: true},
		{name: "userinfo", raw: "https://user:pass@ph.example.com", wantErr: true},
		{name: "unsupported scheme", raw: "ftp://ph.example.com", wantErr: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			base, host, err := normalizeHostURL(tc.raw)
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

// The account is host-and-project so two projects on one instance stay
// isolated — that is what lets dev and prod feed different flows.
func TestAccountIDIsPerProject(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "us.posthog.com-1", accountID("us.posthog.com", 1))
	assert.NotEqual(t, accountID("us.posthog.com", 1), accountID("us.posthog.com", 2))
}

func TestProjectsListsWithoutPersisting(t *testing.T) {
	t.Parallel()

	server := projectsServer(t, `{"results":[{"id":1,"name":"Dev"},{"id":2,"name":"Prod"}]}`)
	defer server.Close()

	auth, creds, _ := newAuthenticator(t)
	pointAt(auth, server.URL)

	projects, err := auth.Projects(t.Context(), "https://us.posthog.com", "phx-key")
	require.NoError(t, err)
	require.Len(t, projects, 2)
	assert.Equal(t, "Dev", projects[0].Name)

	refs, err := creds.List()
	require.NoError(t, err)
	assert.Empty(t, refs, "listing must not store a credential — the user has not picked a project yet")
}

func TestProjectsRejectsABadKey(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	auth, creds, _ := newAuthenticator(t)
	pointAt(auth, server.URL)

	_, err := auth.Projects(t.Context(), "https://us.posthog.com", "bad")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "project:read", "the message names the scope this call actually needs")

	refs, _ := creds.List()
	assert.Empty(t, refs, "a rejected key leaves nothing behind")
}

// A key that authenticates but sees nothing is a scope problem, and saying so
// beats presenting an empty picker.
func TestProjectsRejectsAKeyThatSeesNothing(t *testing.T) {
	t.Parallel()

	server := projectsServer(t, `{"results":[]}`)
	defer server.Close()

	auth, _, _ := newAuthenticator(t)
	pointAt(auth, server.URL)

	_, err := auth.Projects(t.Context(), "https://us.posthog.com", "phx-key")
	assert.ErrorContains(t, err, "can see no projects")
}

func TestConnectStoresKeyAndBinding(t *testing.T) {
	t.Parallel()

	server := projectsServer(t, `{"results":[{"id":1,"name":"Dev"},{"id":2,"name":"Prod"}]}`)
	defer server.Close()

	auth, creds, projects := newAuthenticator(t)
	pointAt(auth, server.URL)

	project, err := auth.Connect(t.Context(), "https://us.posthog.com", "phx-key", 2)
	require.NoError(t, err)
	assert.Equal(t, "us.posthog.com-2", project.Account)
	assert.Equal(t, "Prod", project.Name)

	ref := credentials.Ref{Provider: Provider, Account: "us.posthog.com-2"}
	token, err := creds.Get(ref)
	require.NoError(t, err)
	assert.Equal(t, "phx-key", token)

	binding, err := projects.Get(ref)
	require.NoError(t, err)
	assert.Equal(t, "https://us.posthog.com", binding.URL, "the normalized base is bound to the account")
	assert.Equal(t, 2, binding.ProjectID)
}

// A hand-supplied project id the key cannot reach must fail at connect, not at
// the first poll — the latter surfaces as an empty feed.
func TestConnectRejectsAnUnreachableProject(t *testing.T) {
	t.Parallel()

	server := projectsServer(t, `{"results":[{"id":1,"name":"Dev"}]}`)
	defer server.Close()

	auth, creds, _ := newAuthenticator(t)
	pointAt(auth, server.URL)

	_, err := auth.Connect(t.Context(), "https://us.posthog.com", "phx-key", 99)
	require.ErrorContains(t, err, "cannot see project 99")

	refs, _ := creds.List()
	assert.Empty(t, refs)
}

func TestConnectRequiresAProject(t *testing.T) {
	t.Parallel()

	auth, _, _ := newAuthenticator(t)
	_, err := auth.Connect(t.Context(), "https://us.posthog.com", "phx-key", 0)
	assert.ErrorContains(t, err, "project must be selected")
}

// Two projects on one instance connect side by side under distinct refs, which
// is the acceptance criterion for routing them to different flows.
func TestConnectSupportsSeveralProjectsOnOneHost(t *testing.T) {
	t.Parallel()

	server := projectsServer(t, `{"results":[{"id":1,"name":"Dev"},{"id":2,"name":"Prod"}]}`)
	defer server.Close()

	auth, creds, projects := newAuthenticator(t)
	pointAt(auth, server.URL)

	_, err := auth.Connect(t.Context(), "https://us.posthog.com", "phx-key", 1)
	require.NoError(t, err)
	_, err = auth.Connect(t.Context(), "https://us.posthog.com", "phx-key", 2)
	require.NoError(t, err)

	refs, err := creds.List()
	require.NoError(t, err)
	assert.Len(t, refs, 2, "each project is its own account")

	dev, err := projects.Get(credentials.Ref{Provider: Provider, Account: "us.posthog.com-1"})
	require.NoError(t, err)
	prod, err := projects.Get(credentials.Ref{Provider: Provider, Account: "us.posthog.com-2"})
	require.NoError(t, err)
	assert.Equal(t, 1, dev.ProjectID)
	assert.Equal(t, 2, prod.ProjectID)
}

func TestDisconnectRemovesBothHalves(t *testing.T) {
	t.Parallel()

	server := projectsServer(t, `{"results":[{"id":1,"name":"Dev"}]}`)
	defer server.Close()

	auth, creds, projects := newAuthenticator(t)
	pointAt(auth, server.URL)

	_, err := auth.Connect(t.Context(), "https://us.posthog.com", "phx-key", 1)
	require.NoError(t, err)

	require.NoError(t, auth.Disconnect(t.Context(), "us.posthog.com-1"))

	ref := credentials.Ref{Provider: Provider, Account: "us.posthog.com-1"}
	_, err = creds.Get(ref)
	require.ErrorIs(t, err, credentials.ErrNotFound)

	binding, err := projects.Get(ref)
	require.NoError(t, err)
	assert.Zero(t, binding.ProjectID, "the binding goes with the credential")
}

func TestDisconnectIsIdempotent(t *testing.T) {
	t.Parallel()

	auth, _, _ := newAuthenticator(t)
	assert.NoError(t, auth.Disconnect(t.Context(), "us.posthog.com-1"))
}

func TestConnectNotifiesOnChange(t *testing.T) {
	t.Parallel()

	server := projectsServer(t, `{"results":[{"id":1,"name":"Dev"}]}`)
	defer server.Close()

	var notified []credentials.Ref
	creds := credentials.NewMemoryStore()
	projects := NewProjectStore(filepath.Join(t.TempDir(), "p.json"))
	auth := NewAuthenticator(creds, projects, zerolog.Nop(), func(ref credentials.Ref) {
		notified = append(notified, ref)
	})
	pointAt(auth, server.URL)

	_, err := auth.Connect(t.Context(), "https://us.posthog.com", "phx-key", 1)
	require.NoError(t, err)
	require.NoError(t, auth.Disconnect(t.Context(), "us.posthog.com-1"))

	assert.Len(t, notified, 2, "connect and disconnect both invalidate cooldowns and refresh Integrations")
}
