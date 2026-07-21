package feed

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/internal/desktop/activity"
	"github.com/colonyops/hive/internal/github"
)

type searchBatchAPI struct {
	mu       sync.Mutex
	calls    int
	failNext bool
	aliases  int
}

func (a *searchBatchAPI) handler(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.calls++
	if a.failNext {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	var request struct {
		Query string `json:"query"`
	}
	_ = json.NewDecoder(r.Body).Decode(&request)
	a.aliases = strings.Count(request.Query, ": search(")
	data := make(map[string]any, a.aliases)
	for i := range a.aliases {
		data["s"+string(rune('0'+i))] = map[string]any{"nodes": []any{searchNode(i + 1)}}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"data": data})
}

func (a *searchBatchAPI) snapshot() (calls, aliases int) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.calls, a.aliases
}

func (a *searchBatchAPI) setFailNext(fail bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.failNext = fail
}

func searchNode(number int) map[string]any {
	return map[string]any{
		"__typename": "Issue",
		"number":     number,
		"title":      "item",
		"state":      "OPEN",
		"url":        "https://github.com/o/r/issues/1",
		"author":     map[string]any{"login": "octo"},
		"repository": map[string]any{"nameWithOwner": "o/r"},
		"labels":     map[string]any{"nodes": []any{}},
		"createdAt":  "2026-07-20T00:00:00Z",
		"updatedAt":  "2026-07-20T00:00:00Z",
	}
}

func newLiveProviderForTest(t *testing.T, api *searchBatchAPI, token string) (*LiveProvider, *github.MemoryTokenStore) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(api.handler))
	t.Cleanup(server.Close)
	tokens := github.NewMemoryTokenStore(token)
	live := NewLiveProvider(github.NewClient(github.WithAPIBase(server.URL)), tokens, zerolog.Nop())
	return live, tokens
}

func TestPrefetchSearch_OneRequestForManySources(t *testing.T) {
	api := &searchBatchAPI{}
	live, _ := newLiveProviderForTest(t, api, "token")
	defs := []SourceDef{
		{ID: "one", Kind: "search", Query: "is:open"},
		{ID: "two", Kind: "search", Query: "is:open"},
		{ID: "three", Kind: "search", Query: "is:pr"},
	}

	require.NoError(t, live.PrefetchSearch(t.Context(), defs))
	calls, aliases := api.snapshot()
	assert.Equal(t, 1, calls)
	assert.Equal(t, 2, aliases)

	for _, def := range defs {
		items, err := live.SourceItems(t.Context(), def)
		require.NoError(t, err)
		require.Len(t, items, 1)
		assert.Equal(t, time.Date(2026, 7, 20, 0, 0, 0, 0, time.UTC).UnixMilli(), items[0].UpdatedAt)
	}
	calls, _ = api.snapshot()
	assert.Equal(t, 1, calls, "prefetched entries should serve every source from cache")
}

func TestPrefetchSearch_SkipsNotifications(t *testing.T) {
	api := &searchBatchAPI{}
	live, _ := newLiveProviderForTest(t, api, "")

	require.NoError(t, live.PrefetchSearch(t.Context(), []SourceDef{{ID: "inbox", Kind: "notifications"}}))
	calls, _ := api.snapshot()
	assert.Zero(t, calls)
}

func TestPrefetchSearch_FailureServesStaleWithoutRefetch(t *testing.T) {
	api := &searchBatchAPI{}
	live, _ := newLiveProviderForTest(t, api, "token")
	now := time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)
	live.now = func() time.Time { return now }
	def := SourceDef{ID: "open", Kind: "search", Query: "is:open"}

	require.NoError(t, live.PrefetchSearch(t.Context(), []SourceDef{def}))
	api.setFailNext(true)
	now = now.Add(DefaultPollInterval)
	require.Error(t, live.PrefetchSearch(t.Context(), []SourceDef{def}))

	items, err := live.SourceItems(t.Context(), def)
	require.NoError(t, err)
	require.Len(t, items, 1)
	calls, _ := api.snapshot()
	assert.Equal(t, 2, calls, "the failed prefetch should suppress a per-source retry")
}

func TestPrefetchSearch_AuthErrorPassesThrough(t *testing.T) {
	api := &searchBatchAPI{}
	live, tokens := newLiveProviderForTest(t, api, "token")
	now := time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)
	live.now = func() time.Time { return now }
	def := SourceDef{ID: "open", Kind: "search", Query: "is:open"}

	require.NoError(t, live.PrefetchSearch(t.Context(), []SourceDef{def}))
	require.NoError(t, tokens.DeleteToken())
	now = now.Add(DefaultPollInterval)
	err := live.PrefetchSearch(t.Context(), []SourceDef{def})
	require.ErrorIs(t, err, ErrNotAuthenticated)

	items, err := live.SourceItems(t.Context(), def)
	assert.Nil(t, items)
	require.ErrorIs(t, err, ErrNotAuthenticated)
	calls, _ := api.snapshot()
	assert.Equal(t, 1, calls, "auth failure is retained and must not cause a retry")
}

func TestSourceItems_SearchTTLHonorsSetSearchTTL(t *testing.T) {
	api := &searchBatchAPI{}
	live, _ := newLiveProviderForTest(t, api, "token")
	live.SetSearchTTL(2 * time.Minute)
	now := time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)
	live.now = func() time.Time { return now }
	def := SourceDef{ID: "open", Kind: "search", Query: "is:open"}

	_, err := live.SourceItems(t.Context(), def)
	require.NoError(t, err)
	now = now.Add(90 * time.Second)
	_, err = live.SourceItems(t.Context(), def)
	require.NoError(t, err)
	now = now.Add(40 * time.Second)
	_, err = live.SourceItems(t.Context(), def)
	require.NoError(t, err)

	calls, _ := api.snapshot()
	assert.Equal(t, 2, calls, "90s is fresh with a 2m TTL, while 130s is stale")
}

type recordingActivity struct {
	mu     sync.Mutex
	events []activity.Event
}

func (r *recordingActivity) Record(_ context.Context, event activity.Event) {
	r.mu.Lock()
	r.events = append(r.events, event)
	r.mu.Unlock()
}

func (r *recordingActivity) snapshot() []activity.Event {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]activity.Event(nil), r.events...)
}

func newLiveProviderWithHandler(t *testing.T, handler http.HandlerFunc) *LiveProvider {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	return NewLiveProvider(github.NewClient(github.WithAPIBase(server.URL)), github.NewMemoryTokenStore("token"), zerolog.Nop())
}

func writeSearchResponse(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{
		"s0": map[string]any{"nodes": []any{searchNode(1)}},
	}})
}

func TestCooldown_SuppressesAllFetches(t *testing.T) {
	var mu sync.Mutex
	requests := 0
	limited := false
	live := newLiveProviderWithHandler(t, func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		requests++
		if r.URL.Path == "/graphql" && !limited {
			limited = true
			w.Header().Set("Retry-After", "120")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		if r.URL.Path == "/graphql" {
			writeSearchResponse(w)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[]`))
	})
	now := time.Now()
	live.now = func() time.Time { return now }
	search := SourceDef{ID: "search", Kind: "search", Query: "is:open"}
	notifications := SourceDef{ID: "inbox", Kind: "notifications"}

	_, err := live.SourceItems(t.Context(), search)
	require.ErrorIs(t, err, github.ErrRateLimited)
	mu.Lock()
	assert.Equal(t, 1, requests)
	mu.Unlock()

	_, err = live.SourceItems(t.Context(), search)
	require.ErrorIs(t, err, github.ErrRateLimited)
	_, err = live.SourceItems(t.Context(), notifications)
	require.ErrorIs(t, err, github.ErrRateLimited)
	mu.Lock()
	assert.Equal(t, 1, requests, "cooldown suppresses search and notifications")
	mu.Unlock()

	live.mu.Lock()
	reset := live.cooldownUntil
	live.mu.Unlock()
	require.False(t, reset.IsZero())
	now = reset.Add(time.Second)
	_, err = live.SourceItems(t.Context(), search)
	require.NoError(t, err)
	_, err = live.SourceItems(t.Context(), notifications)
	require.NoError(t, err)
	mu.Lock()
	assert.Equal(t, 3, requests, "fetching resumes once the cooldown expires")
	mu.Unlock()
}

func TestCooldown_ServesStale(t *testing.T) {
	var mu sync.Mutex
	requests := 0
	limited := false
	live := newLiveProviderWithHandler(t, func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		requests++
		if limited {
			w.Header().Set("Retry-After", "120")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		writeSearchResponse(w)
	})
	now := time.Now()
	live.now = func() time.Time { return now }
	def := SourceDef{ID: "search", Kind: "search", Query: "is:open"}

	items, err := live.SourceItems(t.Context(), def)
	require.NoError(t, err)
	require.Len(t, items, 1)
	limited = true
	now = now.Add(DefaultPollInterval)
	items, err = live.SourceItems(t.Context(), def)
	require.NoError(t, err, "the rate-limited request serves stale cache")
	require.Len(t, items, 1)
	items, err = live.SourceItems(t.Context(), def)
	require.NoError(t, err, "cooldown continues to serve stale cache")
	require.Len(t, items, 1)
	mu.Lock()
	assert.Equal(t, 2, requests)
	mu.Unlock()
}

func TestCooldown_RecordsActivityOnce(t *testing.T) {
	requests := 0
	live := newLiveProviderWithHandler(t, func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.Header().Set("Retry-After", "120")
		w.WriteHeader(http.StatusTooManyRequests)
	})
	now := time.Now()
	live.now = func() time.Time { return now }
	recorder := &recordingActivity{}
	live.SetRecorder(recorder)

	_, err := live.SourceItems(t.Context(), SourceDef{ID: "search", Kind: "search", Query: "is:open"})
	require.ErrorIs(t, err, github.ErrRateLimited)
	_, err = live.SourceItems(t.Context(), SourceDef{ID: "inbox", Kind: "notifications"})
	require.ErrorIs(t, err, github.ErrRateLimited)
	err = live.PrefetchSearch(t.Context(), []SourceDef{{ID: "other", Kind: "search", Query: "is:pr"}})
	require.ErrorIs(t, err, github.ErrRateLimited)

	events := recorder.snapshot()
	require.Len(t, events, 1)
	assert.Equal(t, activity.RefreshFailed("github", events[0].Body).Title, events[0].Title)
	assert.Contains(t, events[0].Body, "rate limited; fetches paused until")
	assert.Equal(t, 1, requests, "suppressed fetches never reach GitHub")
}

func TestInvalidate_ClearsCooldown(t *testing.T) {
	requests := 0
	limited := true
	live := newLiveProviderWithHandler(t, func(w http.ResponseWriter, r *http.Request) {
		requests++
		if limited {
			w.Header().Set("Retry-After", "120")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		writeSearchResponse(w)
	})
	now := time.Now()
	live.now = func() time.Time { return now }
	def := SourceDef{ID: "search", Kind: "search", Query: "is:open"}

	_, err := live.SourceItems(t.Context(), def)
	require.ErrorIs(t, err, github.ErrRateLimited)
	_, err = live.SourceItems(t.Context(), def)
	require.ErrorIs(t, err, github.ErrRateLimited)
	assert.Equal(t, 1, requests)

	limited = false
	live.Invalidate()
	items, err := live.SourceItems(t.Context(), def)
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, 2, requests)
}

func TestPrefetchSearch_RecordsTokenErrors(t *testing.T) {
	// The empty-token path is intentionally covered separately from stale-data
	// behavior: every due key receives the same authentication failure.
	api := &searchBatchAPI{}
	live, _ := newLiveProviderForTest(t, api, "")
	defs := []SourceDef{{ID: "one", Kind: "search", Query: "is:open"}, {ID: "two", Kind: "search", Query: "is:pr"}}

	err := live.PrefetchSearch(context.Background(), defs)
	require.ErrorIs(t, err, ErrNotAuthenticated)
	for _, def := range defs {
		_, err = live.SourceItems(t.Context(), def)
		assert.ErrorIs(t, err, ErrNotAuthenticated)
	}
}
