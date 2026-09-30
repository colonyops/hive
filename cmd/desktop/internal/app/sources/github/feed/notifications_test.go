package feed

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNotificationsHydrateAuthor(t *testing.T) {
	for _, subjectType := range []string{"PullRequest", "Issue"} {
		t.Run(subjectType, func(t *testing.T) {
			notifications, details := 0, 0
			live := newLiveProviderWithHandler(t, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if r.URL.Path == "/notifications" {
					notifications++
					if r.Header.Get("If-None-Match") == `"v1"` {
						w.WriteHeader(http.StatusNotModified)
						return
					}
					w.Header().Set("ETag", `"v1"`)
					_ = json.NewEncoder(w).Encode([]any{map[string]any{
						"id": "123", "reason": "review_requested", "unread": true,
						"updated_at": "2026-07-22T12:00:00Z",
						"repository": map[string]any{"full_name": "acme/repo"},
						"subject":    map[string]any{"type": subjectType, "title": "Review me", "url": "https://api.github.com/repos/acme/repo/pulls/42"},
					}})
					return
				}
				assert.Equal(t, "/graphql", r.URL.Path)
				details++
				var request struct{ Query string }
				assert.NoError(t, json.NewDecoder(r.Body).Decode(&request))
				assert.Contains(t, request.Query, "author { login }")
				assert.NotContains(t, request.Query, "body")
				_, _ = w.Write([]byte(`{"data":{"r0":{"issueOrPullRequest":{"author":{"login":"octocat"}}}}}`))
			})
			now := time.Date(2026, 7, 22, 12, 0, 0, 0, time.UTC)
			live.now = func() time.Time { return now }
			src := SourceDef{ID: "reviews", Kind: "notifications"}
			items, err := live.SourceItems(t.Context(), src)
			require.NoError(t, err)
			require.Len(t, items, 1)
			assert.Equal(t, "octocat", items[0].Author)
			assert.Equal(t, "review_requested", items[0].Reason)
			assert.True(t, items[0].Unread)
			assert.Equal(t, now.UnixMilli(), items[0].UpdatedAt)

			cached, err := live.SourceItems(t.Context(), src)
			require.NoError(t, err)
			assert.Equal(t, items, cached)
			assert.Equal(t, 1, notifications)
			now = now.Add(time.Minute)
			cached, err = live.SourceItems(t.Context(), src)
			require.NoError(t, err)
			assert.Equal(t, items, cached)
			assert.Equal(t, 2, notifications)
			assert.Equal(t, 1, details, "304 responses retain the hydrated details")
		})
	}
}

func TestNotificationsRetryAuthorAfterFailure(t *testing.T) {
	details := 0
	live := newLiveProviderWithHandler(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/notifications" {
			if details < 2 {
				assert.Empty(t, r.Header.Get("If-None-Match"), "failed hydration must not be frozen by a 304")
			}
			w.Header().Set("ETag", `"v1"`)
			_, _ = w.Write([]byte(`[{"id":"1","reason":"review_requested","subject":{"type":"PullRequest","title":"Review me","url":"https://api.github.com/repos/acme/repo/pulls/42"},"repository":{"full_name":"acme/repo"}}]`))
			return
		}
		details++
		if details == 1 || details == 3 {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		_, _ = w.Write([]byte(`{"data":{"r0":{"issueOrPullRequest":{"author":{"login":"octocat"}}}}}`))
	})
	now := time.Now()
	live.now = func() time.Time { return now }
	src := SourceDef{ID: "reviews", Kind: "notifications"}
	items, err := live.SourceItems(t.Context(), src)
	require.NoError(t, err, "a detail failure must not hide a new notification")
	require.Len(t, items, 1)
	assert.Empty(t, items[0].Author)
	now = now.Add(time.Minute)
	items, err = live.SourceItems(t.Context(), src)
	require.NoError(t, err)
	assert.Equal(t, "octocat", items[0].Author)
	now = now.Add(time.Minute)
	items, err = live.SourceItems(t.Context(), src)
	require.NoError(t, err)
	assert.Equal(t, "octocat", items[0].Author)
}
