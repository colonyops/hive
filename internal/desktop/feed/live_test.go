package feed

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/internal/github"
)

const testNow = "2026-07-18T12:00:00Z"

// liveAPIServer fakes the search and notifications endpoints. The search
// response includes one PR shared with the notifications inbox so dedupe is
// observable.
func liveAPIServer(t *testing.T) (*httptest.Server, *atomic.Int32) {
	t.Helper()

	var searchCalls atomic.Int32
	mux := http.NewServeMux()
	mux.HandleFunc("/search/issues", func(w http.ResponseWriter, r *http.Request) {
		searchCalls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Query().Get("q") {
		case "is:open is:pr author:@me archived:false":
			_, _ = w.Write([]byte(`{"total_count":2,"items":[
				{"number":42,"title":"Fix spawn env","state":"open",
				 "html_url":"https://github.com/colonyops/hive/pull/42",
				 "repository_url":"https://api.github.com/repos/colonyops/hive",
				 "user":{"login":"hayden"},"labels":[{"name":"bug"}],
				 "pull_request":{"html_url":"https://github.com/colonyops/hive/pull/42"},
				 "updated_at":"2026-07-18T10:00:00Z","created_at":"2026-07-17T00:00:00Z","body":"PR body"},
				{"number":7,"title":"Docs pass","state":"open",
				 "html_url":"https://github.com/colonyops/docs/pull/7",
				 "repository_url":"https://api.github.com/repos/colonyops/docs",
				 "user":{"login":"hayden"},"labels":[],
				 "pull_request":{"html_url":"https://github.com/colonyops/docs/pull/7"},
				 "updated_at":"2026-07-16T10:00:00Z","created_at":"2026-07-15T00:00:00Z","body":""}
			]}`))
		default:
			_, _ = w.Write([]byte(`{"total_count":1,"items":[
				{"number":9,"title":"Assigned issue","state":"open",
				 "html_url":"https://github.com/colonyops/hive/issues/9",
				 "repository_url":"https://api.github.com/repos/colonyops/hive",
				 "user":{"login":"mira"},"labels":[],
				 "updated_at":"2026-07-17T10:00:00Z","created_at":"2026-07-14T00:00:00Z","body":"Issue body"}
			]}`))
		}
	})
	mux.HandleFunc("/notifications", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[
			{"id":"n1","unread":true,"reason":"review_requested","updated_at":"2026-07-18T11:00:00Z",
			 "subject":{"title":"Fix spawn env","url":"https://api.github.com/repos/colonyops/hive/pulls/42","type":"PullRequest"},
			 "repository":{"full_name":"colonyops/hive"}},
			{"id":"n2","unread":false,"reason":"mention","updated_at":"2026-07-18T09:00:00Z",
			 "subject":{"title":"Old issue","url":"https://api.github.com/repos/colonyops/hive/issues/3","type":"Issue"},
			 "repository":{"full_name":"colonyops/hive"}},
			{"id":"n3","unread":true,"reason":"subscribed","updated_at":"2026-07-18T08:00:00Z",
			 "subject":{"title":"v2.0.0","url":"https://api.github.com/repos/colonyops/hive/releases/12","type":"Release"},
			 "repository":{"full_name":"colonyops/hive"}}
		]`))
	})

	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	return server, &searchCalls
}

func newLiveProviderForTest(t *testing.T) (*LiveProvider, *Store, *atomic.Int32) {
	t.Helper()
	server, searchCalls := liveAPIServer(t)
	client := github.NewClient(github.WithAPIBase(server.URL))
	store := NewStore(t.TempDir())
	provider := NewLiveProvider(client, github.NewMemoryTokenStore("tok"), store, zerolog.Nop())
	now, err := time.Parse(time.RFC3339, testNow)
	require.NoError(t, err)
	provider.now = func() time.Time { return now }
	return provider, store, searchCalls
}

func createTestProfile(t *testing.T, store *Store) ProfileDef {
	t.Helper()
	def, err := store.CreateProfile("Frontend Triage")
	require.NoError(t, err)
	return def
}

func TestLiveProviderItemsMergesAndDedupes(t *testing.T) {
	t.Parallel()

	provider, store, _ := newLiveProviderForTest(t)
	def := createTestProfile(t, store)

	items, err := provider.Items(t.Context(), def.ID, "")
	require.NoError(t, err)

	// 2 authored PRs + 1 assigned issue + 2 usable notifications (release
	// skipped), minus the PR#42 dedupe = 4.
	ids := make([]string, 0, len(items))
	for _, item := range items {
		ids = append(ids, item.ID)
	}
	assert.Equal(t, []string{"colonyops/hive#42", "colonyops/hive#3", "colonyops/hive#9", "colonyops/docs#7"}, ids)

	pr := items[0]
	assert.Equal(t, "PR", pr.Kind)
	assert.Equal(t, "colonyops/hive", pr.Repo)
	assert.Equal(t, 42, pr.Num)
	assert.Equal(t, "hayden", pr.Author)
	assert.Equal(t, "2h", pr.Age)
	assert.True(t, pr.Unread)
	assert.Equal(t, "review/42-fix-spawn-env", pr.Branch)
	assert.Equal(t, "https://github.com/colonyops/hive/pull/42", pr.URL)
	assert.Contains(t, pr.Prompt, "Review PR #42 in colonyops/hive")

	// Read notification arrives read; search items start unread.
	assert.False(t, items[1].Unread, "read notification stays read")
	assert.True(t, items[2].Unread)
}

func TestLiveProviderSingleFeedAndUnknownFeed(t *testing.T) {
	t.Parallel()

	provider, store, _ := newLiveProviderForTest(t)
	def := createTestProfile(t, store)

	items, err := provider.Items(t.Context(), def.ID, "my-open-prs")
	require.NoError(t, err)
	require.Len(t, items, 2)
	assert.Equal(t, "colonyops/hive#42", items[0].ID)

	_, err = provider.Items(t.Context(), def.ID, "nope")
	require.ErrorContains(t, err, "unknown feed")

	_, err = provider.Items(t.Context(), "ghost", "")
	require.ErrorContains(t, err, "unknown profile")
}

func TestLiveProviderMarkReadClearsUnread(t *testing.T) {
	t.Parallel()

	provider, store, _ := newLiveProviderForTest(t)
	def := createTestProfile(t, store)

	items, err := provider.Items(t.Context(), def.ID, "")
	require.NoError(t, err)
	require.True(t, items[0].Unread)

	require.NoError(t, provider.MarkRead(t.Context(), def.ID, items[0].ID))

	items, err = provider.Items(t.Context(), def.ID, "")
	require.NoError(t, err)
	assert.False(t, items[0].Unread, "marked item reads as read from cache")
}

func TestLiveProviderProfilesCounts(t *testing.T) {
	t.Parallel()

	provider, store, _ := newLiveProviderForTest(t)
	def := createTestProfile(t, store)

	profiles, err := provider.Profiles(t.Context())
	require.NoError(t, err)
	require.Len(t, profiles, 1)

	profile := profiles[0]
	assert.Equal(t, def.ID, profile.ID)
	assert.Equal(t, "F", profile.Letter)
	assert.Equal(t, "GitHub · 3 sources", profile.SourceSummary)
	require.Len(t, profile.Feeds, 3)

	assert.Equal(t, 2, profile.Feeds[0].Count, "my-open-prs")
	assert.Equal(t, 2, profile.Feeds[1].Count, "notifications inbox skips the release")
	assert.Equal(t, 1, profile.Feeds[1].NewCount, "only the unread notification is new")
	assert.Equal(t, 1, profile.Feeds[2].Count, "assigned")

	// Unique items: hive#42, docs#7, hive#9, hive#3. Unread: all but the
	// read notification hive#3.
	assert.Equal(t, 4, profile.TotalCount)
	assert.Equal(t, 3, profile.UnreadCount)
}

func TestLiveProviderCachesWithinTTL(t *testing.T) {
	t.Parallel()

	provider, store, searchCalls := newLiveProviderForTest(t)
	def := createTestProfile(t, store)

	_, err := provider.Items(t.Context(), def.ID, "my-open-prs")
	require.NoError(t, err)
	_, err = provider.Items(t.Context(), def.ID, "my-open-prs")
	require.NoError(t, err)
	assert.Equal(t, int32(1), searchCalls.Load(), "second read within TTL hits the cache")

	provider.Invalidate()
	_, err = provider.Items(t.Context(), def.ID, "my-open-prs")
	require.NoError(t, err)
	assert.Equal(t, int32(2), searchCalls.Load(), "invalidate forces a refetch")
}

func TestLiveProviderRequiresToken(t *testing.T) {
	t.Parallel()

	server, _ := liveAPIServer(t)
	client := github.NewClient(github.WithAPIBase(server.URL))
	store := NewStore(t.TempDir())
	provider := NewLiveProvider(client, github.NewMemoryTokenStore(""), store, zerolog.Nop())
	def := createTestProfile(t, store)

	_, err := provider.Items(t.Context(), def.ID, "")
	require.ErrorIs(t, err, ErrNotAuthenticated)
}

func TestSlugifyAndShortAge(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "fix-spawn-env", slugify("Fix spawn env"))
	assert.Equal(t, "batch-spawn-fix-detached-tmux-env-path-p", slugify("batch_spawn: fix detached tmux env & PATH propagation"))
	assert.Empty(t, slugify("!!!"))

	assert.Equal(t, "now", shortAge(30*time.Second))
	assert.Equal(t, "5m", shortAge(5*time.Minute))
	assert.Equal(t, "2h", shortAge(2*time.Hour))
	assert.Equal(t, "3d", shortAge(3*24*time.Hour))
	assert.Equal(t, "2w", shortAge(15*24*time.Hour))
}
