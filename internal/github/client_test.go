package github

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUserValidatesToken(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/user", r.URL.Path)
		assert.Equal(t, "Bearer tok123", r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"login":"hayden","name":"Hayden","avatar_url":"https://example.test/a.png"}`))
	}))
	defer server.Close()

	client := NewClient(WithAPIBase(server.URL), WithToken("tok123"))
	user, err := client.User(t.Context())
	require.NoError(t, err)
	assert.Equal(t, "hayden", user.Login)
	assert.Equal(t, "Hayden", user.Name)
}

func TestUserUnauthorized(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"message":"Bad credentials"}`))
	}))
	defer server.Close()

	_, err := NewClient(WithAPIBase(server.URL), WithToken("bad")).User(t.Context())
	require.ErrorIs(t, err, ErrUnauthorized)
}

func TestRateLimitedMapsToSentinel(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-RateLimit-Remaining", "0")
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"message":"API rate limit exceeded"}`))
	}))
	defer server.Close()

	_, err := NewClient(WithAPIBase(server.URL)).User(t.Context())
	require.ErrorIs(t, err, ErrRateLimited)
}

func TestSecondaryRateLimitMapsToSentinel(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Retry-After", "60")
		w.Header().Set("X-RateLimit-Remaining", "1")
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"message":"You have exceeded a secondary rate limit. Please wait a few minutes before you try again."}`))
	}))
	defer server.Close()

	_, err := NewClient(WithAPIBase(server.URL)).User(t.Context())
	require.ErrorIs(t, err, ErrRateLimited)
}

func TestForbiddenWithoutRateLimitIsGeneric(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-RateLimit-Remaining", "42")
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"message":"Resource protected by organization SAML enforcement"}`))
	}))
	defer server.Close()

	_, err := NewClient(WithAPIBase(server.URL)).User(t.Context())
	require.NotErrorIs(t, err, ErrRateLimited)
	require.ErrorContains(t, err, "HTTP 403")
}

func TestUnreachableMapsToSentinel(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	server.Close() // immediately closed: connection refused

	_, err := NewClient(WithAPIBase(server.URL)).User(t.Context())
	require.ErrorIs(t, err, ErrUnreachable)
}

func TestSearchIssues(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/search/issues", r.URL.Path)
		assert.Equal(t, "is:open is:pr author:@me", r.URL.Query().Get("q"))
		assert.Equal(t, "updated", r.URL.Query().Get("sort"))
		assert.Equal(t, "25", r.URL.Query().Get("per_page"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"total_count": 2,
			"items": [
				{
					"number": 42,
					"title": "Fix spawn env",
					"state": "open",
					"html_url": "https://github.com/colonyops/hive/pull/42",
					"repository_url": "https://api.github.com/repos/colonyops/hive",
					"user": {"login": "lena"},
					"labels": [{"name": "bug"}],
					"pull_request": {"html_url": "https://github.com/colonyops/hive/pull/42"},
					"updated_at": "2026-07-18T10:00:00Z",
					"created_at": "2026-07-17T10:00:00Z"
				},
				{
					"number": 7,
					"title": "Docs pass",
					"state": "open",
					"html_url": "https://github.com/colonyops/docs/issues/7",
					"repository_url": "https://api.github.com/repos/colonyops/docs",
					"user": {"login": "mira"},
					"labels": [],
					"updated_at": "2026-07-18T09:00:00Z",
					"created_at": "2026-07-16T09:00:00Z"
				}
			]
		}`))
	}))
	defer server.Close()

	result, err := NewClient(WithAPIBase(server.URL)).SearchIssues(t.Context(), "is:open is:pr author:@me", 25)
	require.NoError(t, err)
	require.Len(t, result.Items, 2)

	pr := result.Items[0]
	assert.True(t, pr.IsPullRequest())
	assert.Equal(t, "colonyops/hive", pr.Repo())
	assert.Equal(t, []Label{{Name: "bug"}}, pr.Labels)

	issue := result.Items[1]
	assert.False(t, issue.IsPullRequest())
	assert.Equal(t, "colonyops/docs", issue.Repo())
}

func TestNotifications(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/notifications", r.URL.Path)
		assert.Equal(t, "true", r.URL.Query().Get("all"))
		assert.Equal(t, "50", r.URL.Query().Get("per_page"))
		assert.Empty(t, r.Header.Get("If-Modified-Since"), "first poll is unconditional")
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Last-Modified", "Sat, 18 Jul 2026 08:00:00 GMT")
		w.Header().Set("X-Poll-Interval", "60")
		_, _ = w.Write([]byte(`[
			{
				"id": "3141",
				"unread": true,
				"reason": "review_requested",
				"updated_at": "2026-07-18T08:00:00Z",
				"subject": {"title": "Fix spawn env", "url": "https://api.github.com/repos/colonyops/hive/pulls/42", "type": "PullRequest"},
				"repository": {"full_name": "colonyops/hive"}
			}
		]`))
	}))
	defer server.Close()

	result, err := NewClient(WithAPIBase(server.URL)).Notifications(t.Context(), 50, "")
	require.NoError(t, err)
	require.Len(t, result.Items, 1)
	assert.False(t, result.NotModified)
	assert.Equal(t, "Sat, 18 Jul 2026 08:00:00 GMT", result.LastModified)
	assert.Equal(t, 60, result.PollInterval)

	n := result.Items[0]
	assert.True(t, n.Unread)
	assert.Equal(t, "PullRequest", n.Subject.Type)
	assert.Equal(t, 42, n.Subject.Number())
	assert.Equal(t, "colonyops/hive", n.Repository.FullName)
}

func TestNotificationsConditionalNotModified(t *testing.T) {
	t.Parallel()

	const lastModified = "Sat, 18 Jul 2026 08:00:00 GMT"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, lastModified, r.Header.Get("If-Modified-Since"))
		w.Header().Set("X-Poll-Interval", "120")
		w.WriteHeader(http.StatusNotModified)
	}))
	defer server.Close()

	result, err := NewClient(WithAPIBase(server.URL)).Notifications(t.Context(), 50, lastModified)
	require.NoError(t, err)
	assert.True(t, result.NotModified)
	assert.Empty(t, result.Items)
	assert.Equal(t, 120, result.PollInterval)
}

func TestParsePollInterval(t *testing.T) {
	t.Parallel()

	assert.Equal(t, 60, parsePollInterval("60"))
	assert.Equal(t, 0, parsePollInterval(""))
	assert.Equal(t, 0, parsePollInterval("soon"))
	assert.Equal(t, 0, parsePollInterval("-5"))
}

func TestRepoFromAPIURL(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "o/r", repoFromAPIURL("https://api.github.com/repos/o/r"))
	assert.Equal(t, "o/r", repoFromAPIURL("https://api.github.com/repos/o/r/issues/5"))
	assert.Empty(t, repoFromAPIURL("https://api.github.com/user"))
	assert.Empty(t, repoFromAPIURL(""))
}
