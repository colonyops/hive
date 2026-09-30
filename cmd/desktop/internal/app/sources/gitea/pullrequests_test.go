package gitea

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/desktop/internal/app/credentials"
)

// connectedPullRequests wires a lookup whose accounts are bound to baseURL, one
// per login, and returns the remote host that instance answers for.
func connectedPullRequests(t *testing.T, baseURL string, logins ...string) (*PullRequests, string) {
	t.Helper()
	parsed, err := url.Parse(baseURL)
	require.NoError(t, err)

	creds := credentials.NewMemoryStore()
	instances := NewInstanceStore(filepath.Join(t.TempDir(), "gitea-instances.json"))
	for _, login := range logins {
		ref := credentials.Ref{Provider: Provider, Account: accountID(parsed.Host, login)}
		require.NoError(t, creds.Set(ref, "gta_"+login))
		require.NoError(t, instances.Set(ref, Binding{URL: baseURL, Login: login}))
	}
	return NewPullRequests(instances, creds, NewFetchers(instances, creds, zerolog.Nop())), parsed.Hostname()
}

func TestPullRequestsServesOnlyTheConnectedInstancesHost(t *testing.T) {
	t.Parallel()

	// The remote's host carries the git transport's port, never the API's, so
	// the match drops it.
	lookup, _ := connectedPullRequests(t, "https://git.example.com:8443", "octocat")
	assert.True(t, lookup.Serves("git.example.com"))
	assert.True(t, lookup.Serves("GIT.EXAMPLE.COM"))
	assert.False(t, lookup.Serves("github.com"))
	assert.False(t, lookup.Serves(""))

	// Nothing connected is what leaves a Gitea session unsupported rather than
	// disconnected: no host is known to be Gitea until an account says so.
	empty, _ := connectedPullRequests(t, "https://git.example.com")
	assert.False(t, empty.Serves("git.example.com"))
}

func TestPullRequestsForBranchAsksTheAccountBoundToTheHost(t *testing.T) {
	t.Parallel()

	var gotAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		switch r.URL.EscapedPath() {
		case "/api/v1/repos/acme/site":
			_, _ = w.Write([]byte(`{"default_branch":"main"}`))
		case "/api/v1/repos/acme/site/pulls/main/feat%2Fbar":
			_, _ = w.Write([]byte(`{"number":12,"state":"open","html_url":"https://git.example.com/acme/site/pulls/12",
				"head":{"ref":"feat/bar","sha":"cafe"}}`))
		default:
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"message":"not found"}`))
		}
	}))
	defer server.Close()

	lookup, host := connectedPullRequests(t, server.URL, "octocat")
	pull, found, err := lookup.ForBranch(t.Context(), host, "acme", "site", "feat/bar")
	require.NoError(t, err)
	require.True(t, found)
	assert.Equal(t, 12, pull.Number)
	assert.Equal(t, "Bearer gta_octocat", gotAuth)
}

// A repository one account cannot see is a 404, not a failure, so every account
// on the instance is asked before the branch is reported to have none.
func TestPullRequestsForBranchTriesEveryAccountOnTheInstance(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer gta_hubot" {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"message":"not found"}`))
			return
		}
		switch r.URL.EscapedPath() {
		case "/api/v1/repos/acme/site":
			_, _ = w.Write([]byte(`{"default_branch":"main"}`))
		case "/api/v1/repos/acme/site/pulls/main/feat%2Fbar":
			_, _ = w.Write([]byte(`{"number":12,"state":"open","head":{"ref":"feat/bar"}}`))
		default:
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"message":"not found"}`))
		}
	}))
	defer server.Close()

	lookup, host := connectedPullRequests(t, server.URL, "octocat", "hubot")
	pull, found, err := lookup.ForBranch(t.Context(), host, "acme", "site", "feat/bar")
	require.NoError(t, err)
	require.True(t, found)
	assert.Equal(t, 12, pull.Number)
}
