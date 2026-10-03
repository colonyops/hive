package app

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/desktop/internal/app/sources/gitea"
	"github.com/colonyops/hive/internal/platform/credentials"
)

// giteaInstance wires a connected Gitea account against server, and returns the
// forge and the remote host that account answers for.
func giteaInstance(t *testing.T, server *httptest.Server) (*giteaForge, string) {
	t.Helper()
	parsed, err := url.Parse(server.URL)
	require.NoError(t, err)

	creds := credentials.NewMemoryStore()
	instances := gitea.NewInstanceStore(filepath.Join(t.TempDir(), "gitea-instances.json"))
	ref := credentials.Ref{Provider: gitea.Provider, Account: parsed.Host + "-octocat"}
	require.NoError(t, creds.Set(ref, "gta_token"))
	require.NoError(t, instances.Set(ref, gitea.Binding{URL: server.URL, Login: "octocat"}))

	fetchers := gitea.NewFetchers(instances, creds, zerolog.Nop())
	return newGiteaForge(gitea.NewPullRequests(instances, creds, fetchers)), parsed.Hostname()
}

// The bar renders the view, not the forge, so a Gitea session's badge has to
// arrive in exactly the shape GitHub's does.
func TestSessionPullRequestsAnswersAGiteaSessionInTheSameView(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.EscapedPath() {
		case "/api/v1/repos/acme/site":
			_, _ = w.Write([]byte(`{"default_branch":"main"}`))
		case "/api/v1/repos/acme/site/pulls/main/feat%2Fbar":
			_, _ = w.Write([]byte(`{"number":12,"title":"Add the thing","state":"open","draft":false,
				"html_url":"https://git.example.com/acme/site/pulls/12","additions":31,"deletions":4,
				"head":{"ref":"feat/bar","sha":"cafe"},"requested_reviewers":[{"login":"hubot"}]}`))
		case "/api/v1/repos/acme/site/commits/cafe/status":
			_, _ = w.Write([]byte(`{"state":"failure","total_count":3}`))
		case "/api/v1/repos/acme/site/pulls/12/reviews":
			_, _ = w.Write([]byte(`[]`))
		default:
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"message":"not found"}`))
		}
	}))
	defer server.Close()

	forge, host := giteaInstance(t, server)
	lookup := newSessionPullRequests(newGitHubForge(nil, nil), forge)

	view, err := lookup.Lookup(t.Context(),
		SessionPullRequestKey{Host: host, Owner: "acme", Repo: "site", Branch: "feat/bar"}, false)
	require.NoError(t, err)
	assert.Equal(t, SessionPullRequest{
		Status: PullRequestStatusFound, Number: 12, Title: "Add the thing", State: "OPEN",
		URL: "https://git.example.com/acme/site/pulls/12", ReviewDecision: "REVIEW_REQUIRED",
		Checks: "failing", Additions: 31, Deletions: 4,
	}, view)
}

// A branch with no pull request on an instance that answered is "none"; the
// same branch on a host nobody has connected is unsupported, since nothing
// identifies that host as a forge the app can ask.
func TestSessionPullRequestsSeparatesAGiteaBranchWithNoneFromAnUnservedHost(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.EscapedPath() == "/api/v1/repos/acme/site" {
			_, _ = w.Write([]byte(`{"default_branch":"main"}`))
			return
		}
		if r.URL.EscapedPath() == "/api/v1/repos/acme/site/pulls" {
			_, _ = w.Write([]byte(`[]`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"not found"}`))
	}))
	defer server.Close()

	forge, host := giteaInstance(t, server)
	lookup := newSessionPullRequests(newGitHubForge(nil, nil), forge)

	none, err := lookup.Lookup(t.Context(),
		SessionPullRequestKey{Host: host, Owner: "acme", Repo: "site", Branch: "feat/bar"}, false)
	require.NoError(t, err)
	assert.Equal(t, PullRequestStatusNone, none.Status)

	unsupported, err := lookup.Lookup(t.Context(),
		SessionPullRequestKey{Host: "git.other.test", Owner: "acme", Repo: "site", Branch: "feat/bar"}, false)
	require.NoError(t, err)
	assert.Equal(t, PullRequestStatusUnsupported, unsupported.Status)
}
