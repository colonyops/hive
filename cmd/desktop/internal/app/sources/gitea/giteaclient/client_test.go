package giteaclient

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/desktop/internal/app/sources/sourcehttp"
)

func TestUserAuthenticatesAsBearer(t *testing.T) {
	t.Parallel()

	var gotAuth, gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth, gotPath = r.Header.Get("Authorization"), r.URL.Path
		_, _ = w.Write([]byte(`{"login":"octocat","full_name":"Octo Cat","avatar_url":"https://git.example.com/avatar/1"}`))
	}))
	defer server.Close()

	user, err := NewClient(server.URL, "gta_token").User(t.Context())
	require.NoError(t, err)
	assert.Equal(t, "octocat", user.Login)
	assert.Equal(t, "Octo Cat", user.FullName)
	assert.Equal(t, "/api/v1/user", gotPath)
	assert.Equal(t, "Bearer gta_token", gotAuth, "a Gitea access token is sent as a bearer")
}

func TestUserUnauthorized(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	_, err := NewClient(server.URL, "bad").User(t.Context())
	assert.ErrorIs(t, err, sourcehttp.ErrUnauthorized)
}

// A base URL with a path is a Gitea served under a subpath; the API hangs off
// it rather than off the host root.
func TestClientPreservesABaseSubpath(t *testing.T) {
	t.Parallel()

	var gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_, _ = w.Write([]byte(`{"version":"1.27.1"}`))
	}))
	defer server.Close()

	version, err := NewClient(server.URL+"/git", "t").Version(t.Context())
	require.NoError(t, err)
	assert.Equal(t, "1.27.1", version)
	assert.Equal(t, "/git/api/v1/version", gotPath)
}

func TestSearchIssuesSendsEveryFilter(t *testing.T) {
	t.Parallel()

	var got url.Values
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/repos/issues/search", r.URL.Path)
		got = r.URL.Query()
		_, _ = w.Write([]byte(`[
			{"id":1,"number":42,"title":"Fix the thing","body":"details","state":"open",
			 "html_url":"https://git.example.com/acme/app/pulls/42","updated_at":"2026-08-11T22:10:05Z",
			 "user":{"login":"octocat"},"labels":[{"name":"bug"},{"name":"p1"}],
			 "repository":{"full_name":"acme/app"},"pull_request":{"merged":false,"draft":false}},
			{"id":2,"number":7,"title":"An issue","body":"","state":"closed",
			 "html_url":"https://git.example.com/acme/app/issues/7","updated_at":"2026-08-10T09:00:00Z",
			 "user":{"login":"hubot"},"labels":[],"repository":{"full_name":"acme/app"}}
		]`))
	}))
	defer server.Close()

	issues, err := NewClient(server.URL, "t").SearchIssues(t.Context(), SearchRequest{
		Type:        "pulls",
		State:       "open",
		Owner:       "acme",
		Labels:      []string{"bug", "p1"},
		Text:        "thing",
		Involvement: InvolvementReviewRequested,
		Limit:       25,
	})
	require.NoError(t, err)
	require.Len(t, issues, 2)

	assert.Equal(t, "pulls", got.Get("type"))
	assert.Equal(t, "open", got.Get("state"))
	assert.Equal(t, "acme", got.Get("owner"))
	assert.Equal(t, "bug,p1", got.Get("labels"), "labels go as one comma-joined value")
	assert.Equal(t, "thing", got.Get("q"))
	assert.Equal(t, "true", got.Get("review_requested"))
	assert.Equal(t, "25", got.Get("limit"))

	assert.True(t, issues[0].IsPullRequest())
	assert.Equal(t, "acme/app", issues[0].Repository.FullName)
	assert.Equal(t, []Label{{Name: "bug"}, {Name: "p1"}}, issues[0].Labels)
	assert.False(t, issues[1].IsPullRequest(), "an item with no pull_request block is an issue")
}

// The zero request must not send empty parameters: the request should state
// only what it means, and Gitea would ignore an empty `type=` anyway.
func TestSearchIssuesOmitsUnsetFilters(t *testing.T) {
	t.Parallel()

	var got url.Values
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.URL.Query()
		_, _ = w.Write([]byte(`[]`))
	}))
	defer server.Close()

	_, err := NewClient(server.URL, "t").SearchIssues(t.Context(), SearchRequest{State: "open", Limit: 50})
	require.NoError(t, err)

	assert.Equal(t, []string{"limit", "state"}, sortedKeys(got))
}

func TestLifecycleStateFoldsMergedOntoClosed(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "open", Issue{State: "open"}.LifecycleState())
	assert.Equal(t, "closed", Issue{State: "closed"}.LifecycleState())
	assert.Equal(t, "closed", Issue{State: "closed", PullReq: &PullMeta{Merged: false}}.LifecycleState())
	assert.Equal(t, "merged", Issue{State: "closed", PullReq: &PullMeta{Merged: true}}.LifecycleState(),
		"Gitea reports a merged pull request as closed; the merge flag is the only thing that separates them")
}

func TestNotificationsReadsTheInbox(t *testing.T) {
	t.Parallel()

	var got url.Values
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/notifications", r.URL.Path)
		got = r.URL.Query()
		_, _ = w.Write([]byte(`[
			{"id":1174,"unread":true,"updated_at":"2026-08-12T08:34:18Z",
			 "repository":{"full_name":"acme/app"},
			 "subject":{"title":"Back up listmonk","type":"Pull","state":"merged",
			            "url":"https://git.example.com/api/v1/repos/acme/app/issues/524",
			            "html_url":"https://git.example.com/acme/app/pulls/524"}}
		]`))
	}))
	defer server.Close()

	threads, err := NewClient(server.URL, "t").Notifications(t.Context(), 25)
	require.NoError(t, err)
	require.Len(t, threads, 1)

	assert.Equal(t, "true", got.Get("all"), "read threads are included so the app mirrors the whole inbox")
	assert.Equal(t, "25", got.Get("limit"))
	assert.Equal(t, "merged", threads[0].Subject.State)
	assert.Equal(t, 524, threads[0].Subject.Number())
}

// Gitea sends no item number on a notification, so it is read off the subject's
// API URL. A subject with no number in it must read as 0 rather than as garbage.
func TestSubjectNumber(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		url  string
		want int
	}{
		{"https://git.example.com/api/v1/repos/acme/app/issues/524", 524},
		{"https://git.example.com/api/v1/repos/acme/app/issues/524/", 524},
		{"https://git.example.com/api/v1/repos/acme/app/git/commits/abc123", 0},
		{"", 0},
		{"https://git.example.com/api/v1/repos/acme/app/issues/-3", 0},
	} {
		assert.Equalf(t, tc.want, Subject{URL: tc.url}.Number(), "url %q", tc.url)
	}
}

// A deleted item, or one in a repository the token lost access to, is a verdict
// the caller acts on rather than an error that should abort a batch.
func TestIssueReportsNotFoundWithoutFailing(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/repos/acme/gone/issues/9" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		assert.Equal(t, "/api/v1/repos/acme/app/issues/42", r.URL.Path)
		_, _ = w.Write([]byte(`{"id":1,"number":42,"title":"Fix","state":"closed",
			"updated_at":"2026-08-11T22:10:05Z","repository":{"full_name":"acme/app"},
			"pull_request":{"merged":true}}`))
	}))
	defer server.Close()

	client := NewClient(server.URL, "t")

	issue, found, err := client.Issue(t.Context(), "acme", "app", 42)
	require.NoError(t, err)
	require.True(t, found)
	assert.Equal(t, "merged", issue.LifecycleState())

	_, found, err = client.Issue(t.Context(), "acme", "gone", 9)
	require.NoError(t, err, "a 404 is not a failure")
	assert.False(t, found)
}

func sortedKeys(values url.Values) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	return keys
}
