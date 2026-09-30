package ghclient

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/desktop/internal/app/sources/sourcehttp"
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

	client := NewClient(WithAPIBase(server.URL)).WithTokenCopy("tok123")
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

	_, err := NewClient(WithAPIBase(server.URL)).WithTokenCopy("bad").User(t.Context())
	require.ErrorIs(t, err, sourcehttp.ErrUnauthorized)
}

func TestRateLimitedMapsToSentinel(t *testing.T) {
	t.Parallel()

	const resetEpoch = 1_780_000_000
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-RateLimit-Remaining", "0")
		w.Header().Set("X-RateLimit-Reset", strconv.FormatInt(resetEpoch, 10))
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"message":"API rate limit exceeded"}`))
	}))
	defer server.Close()

	_, err := NewClient(WithAPIBase(server.URL)).User(t.Context())
	require.ErrorIs(t, err, sourcehttp.ErrRateLimited)
	var rateErr *sourcehttp.RateLimitError
	require.ErrorAs(t, err, &rateErr)
	assert.Equal(t, time.Unix(resetEpoch, 0), rateErr.ResetAt)
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

	before := time.Now()
	_, err := NewClient(WithAPIBase(server.URL)).User(t.Context())
	after := time.Now()
	require.ErrorIs(t, err, sourcehttp.ErrRateLimited)
	var rateErr *sourcehttp.RateLimitError
	require.ErrorAs(t, err, &rateErr)
	assert.WithinDuration(t, before.Add(time.Minute), rateErr.ResetAt, time.Second)
	assert.WithinDuration(t, after.Add(time.Minute), rateErr.ResetAt, time.Second)
}

func TestRateLimitedWithoutResetHasZeroResetAt(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()

	_, err := NewClient(WithAPIBase(server.URL)).User(t.Context())
	require.ErrorIs(t, err, sourcehttp.ErrRateLimited)
	var rateErr *sourcehttp.RateLimitError
	require.ErrorAs(t, err, &rateErr)
	assert.True(t, rateErr.ResetAt.IsZero())
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
	require.NotErrorIs(t, err, sourcehttp.ErrRateLimited)
	require.ErrorContains(t, err, "HTTP 403")
}

func TestUnreachableMapsToSentinel(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	server.Close() // immediately closed: connection refused

	_, err := NewClient(WithAPIBase(server.URL)).User(t.Context())
	require.ErrorIs(t, err, sourcehttp.ErrUnreachable)
}

func TestBuildSearchQuery(t *testing.T) {
	t.Parallel()

	metacharacters := `is:open "quoted" { injected }
second-line`
	tests := []struct {
		name string
		reqs []SearchRequest
	}{
		{name: "one request", reqs: []SearchRequest{{Query: "is:open", Limit: 25}}},
		{name: "two requests", reqs: []SearchRequest{{Query: "is:open", Limit: 25}, {Query: "is:pr", Limit: 50}}},
		{name: "metacharacters stay in variables", reqs: []SearchRequest{{Query: metacharacters, Limit: 1}, {Query: "repo:o/r", Limit: 100}, {Query: "author:@me", Limit: 10}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc, variables := buildSearchQuery(tt.reqs)

			for i, req := range tt.reqs {
				index := strconv.Itoa(i)
				assert.Contains(t, doc, "$q"+index+": String!")
				assert.Contains(t, doc, "$after"+index+": String")
				assert.Contains(t, doc, "s"+index+": search(query: $q"+index+", type: ISSUE, first: "+strconv.Itoa(min(req.Limit, searchPageSize))+", after: $after"+index+")")
				assert.Equal(t, req.Query+" sort:updated-desc", variables["q"+index])
				assert.Nil(t, variables["after"+index])
			}
			assert.Contains(t, doc, "... on Issue {")
			assert.Contains(t, doc, "... on PullRequest {")
			assert.Contains(t, doc, "labels(first: 20)")
			assert.Contains(t, doc, "reviewDecision additions deletions")
			assert.Contains(t, doc, "statusCheckRollup { state }")
			assert.Contains(t, doc, "pageInfo { endCursor hasNextPage }")
			assert.NotContains(t, doc, metacharacters)
			assert.NotContains(t, doc, "quoted")
			assert.Equal(t, len(tt.reqs), strings.Count(doc, "search(query:"))
		})
	}
}

func TestSearchIssuesBatch(t *testing.T) {
	t.Parallel()

	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/graphql", r.URL.Path)
		assert.Equal(t, "Bearer tok123", r.Header.Get("Authorization"))

		var request struct {
			Query     string         `json:"query"`
			Variables map[string]any `json:"variables"`
		}
		assert.NoError(t, json.NewDecoder(r.Body).Decode(&request))
		assert.Equal(t, "is:open sort:updated-desc", request.Variables["q0"])
		assert.Equal(t, "is:pr sort:updated-desc", request.Variables["q1"])
		assert.Contains(t, request.Query, "s0: search(query: $q0, type: ISSUE, first: 25, after: $after0)")
		assert.Contains(t, request.Query, "s1: search(query: $q1, type: ISSUE, first: 25, after: $after1)")

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"s0":{"nodes":[{"__typename":"Issue","number":7,"title":"Docs pass","body":"Update docs","state":"OPEN","url":"https://github.com/o/docs/issues/7","createdAt":"2026-07-16T09:00:00Z","updatedAt":"2026-07-18T09:00:00Z","author":{"login":"mira"},"repository":{"nameWithOwner":"o/docs"},"labels":{"nodes":[{"name":"docs"}]}}]},"s1":{"nodes":[{"__typename":"PullRequest","number":42,"title":"Fix spawn env","body":"Fix it","state":"OPEN","url":"https://github.com/o/hive/pull/42","isDraft":true,"reviewDecision":"APPROVED","additions":42,"deletions":7,"statusCheckRollup":{"state":"SUCCESS"},"createdAt":"2026-07-17T10:00:00Z","updatedAt":"2026-07-18T10:00:00Z","author":{"login":"lena"},"repository":{"nameWithOwner":"o/hive"},"labels":{"nodes":[{"name":"bug"},{"name":"desktop"}]}}]}}}`))
	}))
	defer server.Close()

	results, err := NewClient(WithAPIBase(server.URL)).WithTokenCopy("tok123").SearchIssuesBatch(t.Context(), []SearchRequest{
		{Query: "is:open", Limit: 25},
		{Query: "is:pr", Limit: 50},
	})
	require.NoError(t, err)
	assert.Equal(t, 1, calls)
	require.Len(t, results, 2)
	require.Len(t, results[0], 1)
	require.Len(t, results[1], 1)

	issue := results[0][0]
	assert.False(t, issue.IsPullRequest)
	assert.False(t, issue.Draft)
	assert.Equal(t, "o/docs", issue.Repo)
	assert.Equal(t, "mira", issue.Author)
	assert.Equal(t, []Label{{Name: "docs"}}, issue.Labels)

	pr := results[1][0]
	assert.True(t, pr.IsPullRequest)
	assert.True(t, pr.Draft)
	assert.Equal(t, ReviewStateDraft, pr.Review)
	assert.Equal(t, CheckStatePassing, pr.Checks)
	assert.Equal(t, 42, pr.Additions)
	assert.Equal(t, 7, pr.Deletions)
	assert.Equal(t, "o/hive", pr.Repo)
	assert.Equal(t, "lena", pr.Author)
	assert.Equal(t, []Label{{Name: "bug"}, {Name: "desktop"}}, pr.Labels)
}

func TestSearchIssuesBatch_SplitsAliasBatches(t *testing.T) {
	t.Parallel()

	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		var request struct {
			Query string `json:"query"`
		}
		if !assert.NoError(t, json.NewDecoder(r.Body).Decode(&request)) {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if calls == 1 {
			assert.Contains(t, request.Query, "s0: search")
			assert.Contains(t, request.Query, "s1: search")
			assert.NotContains(t, request.Query, "s2: search")
			_, _ = w.Write([]byte(`{"data":{"s0":{"nodes":[]},"s1":{"nodes":[]}}}`))
			return
		}
		assert.Equal(t, 2, calls)
		assert.Contains(t, request.Query, "s0: search")
		assert.NotContains(t, request.Query, "s1: search")
		_, _ = w.Write([]byte(`{"data":{"s0":{"nodes":[]}}}`))
	}))
	defer server.Close()

	results, err := NewClient(WithAPIBase(server.URL)).SearchIssuesBatch(t.Context(), []SearchRequest{
		{Query: "repo:o/one", Limit: 1},
		{Query: "repo:o/two", Limit: 1},
		{Query: "repo:o/three", Limit: 1},
	})
	require.NoError(t, err)
	assert.Equal(t, 2, calls)
	assert.Len(t, results, 3)
}

func TestSearchIssuesBatch_PaginatesLargeSearches(t *testing.T) {
	t.Parallel()

	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		var request struct {
			Query     string         `json:"query"`
			Variables map[string]any `json:"variables"`
		}
		if !assert.NoError(t, json.NewDecoder(r.Body).Decode(&request)) {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")

		if calls == 1 {
			assert.Contains(t, request.Query, "s0: search(query: $q0, type: ISSUE, first: 25, after: $after0)")
			assert.Contains(t, request.Query, "s1: search(query: $q1, type: ISSUE, first: 25, after: $after1)")
			assert.Nil(t, request.Variables["after0"])
			assert.Nil(t, request.Variables["after1"])
			_, _ = w.Write([]byte(`{"data":{"s0":{"pageInfo":{"endCursor":"cursor-25","hasNextPage":true},"nodes":[{"__typename":"PullRequest","number":1}]},"s1":{"pageInfo":{"hasNextPage":false},"nodes":[{"__typename":"Issue","number":2}]}}}`))
			return
		}

		assert.Equal(t, 2, calls)
		assert.Contains(t, request.Query, "s0: search(query: $q0, type: ISSUE, first: 25, after: $after0)")
		assert.NotContains(t, request.Query, "s1: search")
		assert.Equal(t, "cursor-25", request.Variables["after0"])
		_, _ = w.Write([]byte(`{"data":{"s0":{"pageInfo":{"hasNextPage":false},"nodes":[{"__typename":"PullRequest","number":51}]}}}`))
	}))
	defer server.Close()

	results, err := NewClient(WithAPIBase(server.URL)).SearchIssuesBatch(t.Context(), []SearchRequest{
		{Query: "is:pr", Limit: 100},
		{Query: "is:issue", Limit: 25},
	})
	require.NoError(t, err)
	assert.Equal(t, 2, calls)
	require.Len(t, results, 2)
	require.Len(t, results[0], 2)
	assert.Equal(t, 1, results[0][0].Number)
	assert.Equal(t, 51, results[0][1].Number)
	require.Len(t, results[1], 1)
	assert.Equal(t, 2, results[1][0].Number)
}

func TestSearchIssuesBatch_Empty(t *testing.T) {
	t.Parallel()

	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		calls++
	}))
	defer server.Close()

	items, err := NewClient(WithAPIBase(server.URL)).SearchIssuesBatch(t.Context(), nil)
	require.NoError(t, err)
	assert.Nil(t, items)
	assert.Zero(t, calls)
}

func TestSearchIssuesBatch_RateLimitedGraphQLError(t *testing.T) {
	t.Parallel()

	const resetEpoch = 1_780_000_000
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-RateLimit-Reset", strconv.FormatInt(resetEpoch, 10))
		_, _ = w.Write([]byte(`{"errors":[{"type":"RATE_LIMITED","message":"rate limit exceeded"}]}`))
	}))
	defer server.Close()

	_, err := NewClient(WithAPIBase(server.URL)).SearchIssuesBatch(t.Context(), []SearchRequest{{Query: "is:open", Limit: 25}})
	require.ErrorIs(t, err, sourcehttp.ErrRateLimited)
	var rateErr *sourcehttp.RateLimitError
	require.ErrorAs(t, err, &rateErr)
	assert.Equal(t, time.Unix(resetEpoch, 0), rateErr.ResetAt)
}

func TestSearchIssuesBatch_HTTPErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		status  int
		headers map[string]string
		want    error
	}{
		{name: "unauthorized", status: http.StatusUnauthorized, want: sourcehttp.ErrUnauthorized},
		{name: "rate limited", status: http.StatusForbidden, headers: map[string]string{"X-RateLimit-Remaining": "0"}, want: sourcehttp.ErrRateLimited},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, http.MethodPost, r.Method)
				for key, value := range tt.headers {
					w.Header().Set(key, value)
				}
				w.WriteHeader(tt.status)
			}))
			defer server.Close()

			_, err := NewClient(WithAPIBase(server.URL)).SearchIssuesBatch(t.Context(), []SearchRequest{{Query: "is:open", Limit: 25}})
			require.ErrorIs(t, err, tt.want)
		})
	}
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

	result, err := NewClient(WithAPIBase(server.URL)).Notifications(t.Context(), 50, sourcehttp.Validators{})
	require.NoError(t, err)
	require.Len(t, result.Items, 1)
	assert.False(t, result.NotModified)
	assert.Equal(t, "Sat, 18 Jul 2026 08:00:00 GMT", result.Validators.LastModified)
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

	result, err := NewClient(WithAPIBase(server.URL)).Notifications(t.Context(), 50, sourcehttp.Validators{LastModified: lastModified})
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
