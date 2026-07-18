package feed

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/internal/github"
)

const (
	testNow          = "2026-07-18T12:00:00Z"
	testLastModified = "Sat, 18 Jul 2026 11:00:00 GMT"
)

// testClock is a settable clock for provider TTL tests.
type testClock struct {
	mu  sync.Mutex
	now time.Time
}

func newTestClock(t *testing.T) *testClock {
	t.Helper()
	now, err := time.Parse(time.RFC3339, testNow)
	require.NoError(t, err)
	return &testClock{now: now}
}

func (c *testClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *testClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

// fakeAPI fakes the search and notifications endpoints. The search response
// includes one PR shared with the notifications inbox so dedupe is
// observable. Notifications support the conditional-GET loop: a request
// carrying the current Last-Modified answers 304.
type fakeAPI struct {
	searchCalls  atomic.Int32
	notifCalls   atomic.Int32
	notifFull    atomic.Int32 // non-304 notification responses
	pollInterval atomic.Int32 // X-Poll-Interval value; 0 omits the header
	// searchStarted (when non-nil) receives one signal per search request;
	// searchRelease (when non-nil) blocks search responses until closed.
	// Together they let concurrency tests hold a fetch in flight.
	searchStarted chan struct{}
	searchRelease chan struct{}
}

func (f *fakeAPI) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/search/issues", func(w http.ResponseWriter, r *http.Request) {
		f.searchCalls.Add(1)
		if f.searchStarted != nil {
			f.searchStarted <- struct{}{}
		}
		if f.searchRelease != nil {
			<-f.searchRelease
		}
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
	mux.HandleFunc("/notifications", func(w http.ResponseWriter, r *http.Request) {
		f.notifCalls.Add(1)
		if interval := f.pollInterval.Load(); interval > 0 {
			w.Header().Set("X-Poll-Interval", strconv.Itoa(int(interval)))
		}
		if r.Header.Get("If-Modified-Since") == testLastModified {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		f.notifFull.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Last-Modified", testLastModified)
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
	return mux
}

func newLiveProviderForTest(t *testing.T) (*LiveProvider, *Store, *fakeAPI, *testClock) {
	t.Helper()
	api := &fakeAPI{}
	provider, store := newLiveProviderWithAPI(t, api)
	clock := newTestClock(t)
	provider.now = clock.Now
	return provider, store, api, clock
}

func newLiveProviderWithAPI(t *testing.T, api *fakeAPI) (*LiveProvider, *Store) {
	t.Helper()
	server := httptest.NewServer(api.handler())
	t.Cleanup(server.Close)

	client := github.NewClient(github.WithAPIBase(server.URL))
	store := newStoreAt(t.TempDir())
	provider := NewLiveProvider(client, github.NewMemoryTokenStore("tok"), store, zerolog.Nop())
	return provider, store
}

func createTestProfile(t *testing.T, store *Store) ProfileDef {
	t.Helper()
	def, err := store.CreateProfile("Frontend Triage")
	require.NoError(t, err)
	return def
}

func TestLiveProviderItemsMergesAndDedupes(t *testing.T) {
	t.Parallel()

	provider, store, _, _ := newLiveProviderForTest(t)
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

	// PR#42 exists in both the my-prs source (10:00) and the notifications
	// inbox (11:00): the newer notification copy wins, with the author and
	// labels backfilled from the search copy and the reason carried on the
	// item.
	pr := items[0]
	assert.Equal(t, "PR", pr.Kind)
	assert.Equal(t, "colonyops/hive", pr.Repo)
	assert.Equal(t, 42, pr.Num)
	assert.Equal(t, "hayden", pr.Author)
	assert.Equal(t, "review_requested", pr.Reason)
	assert.Equal(t, "1h", pr.Age)
	assert.Equal(t, []string{"bug"}, pr.Labels, "real labels survive the newer notification copy")
	assert.True(t, pr.Unread)
	assert.Equal(t, "review/42-fix-spawn-env", pr.Branch)
	assert.Equal(t, "https://github.com/colonyops/hive/pull/42", pr.URL)
	assert.Contains(t, pr.Prompt, "Review PR #42 in colonyops/hive")

	// Read notification arrives read; search items start unread.
	assert.False(t, items[1].Unread, "read notification stays read")
	assert.Equal(t, "mention", items[1].Reason)
	assert.Empty(t, items[1].Labels, "notification-only item carries no labels")
	assert.True(t, items[2].Unread)
	assert.Empty(t, items[2].Reason, "search-only item has no reason")
}

func TestLiveProviderSingleFeedAndUnknownFeed(t *testing.T) {
	t.Parallel()

	provider, store, _, _ := newLiveProviderForTest(t)
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

	provider, store, _, _ := newLiveProviderForTest(t)
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

	provider, store, _, _ := newLiveProviderForTest(t)
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

	provider, store, api, _ := newLiveProviderForTest(t)
	def := createTestProfile(t, store)

	_, err := provider.Items(t.Context(), def.ID, "my-open-prs")
	require.NoError(t, err)
	_, err = provider.Items(t.Context(), def.ID, "my-open-prs")
	require.NoError(t, err)
	assert.Equal(t, int32(1), api.searchCalls.Load(), "second read within TTL hits the cache")

	provider.Invalidate()
	_, err = provider.Items(t.Context(), def.ID, "my-open-prs")
	require.NoError(t, err)
	assert.Equal(t, int32(2), api.searchCalls.Load(), "invalidate forces a refetch")
}

func TestLiveProviderSharedSourceSingleFetch(t *testing.T) {
	t.Parallel()

	api := &fakeAPI{}
	server := httptest.NewServer(api.handler())
	t.Cleanup(server.Close)
	client := github.NewClient(github.WithAPIBase(server.URL))
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "profiles.yaml"), []byte(`sources:
  - id: my-work
    kind: search
    query: "is:open is:pr author:@me archived:false"
  - id: my-work-twin # identical acquisition: same canonical key, one request
    kind: search
    query: "is:open is:pr author:@me archived:false"
profiles:
  - id: work
    name: Work
    feeds:
      - id: all
        name: All
        sources: [my-work]
      - id: hive-only
        name: Hive only
        sources: [my-work-twin]
        filters:
          repos: ["colonyops/hive"]
`), 0o600))
	store := newStoreAt(dir)
	provider := NewLiveProvider(client, github.NewMemoryTokenStore("tok"), store, zerolog.Nop())
	provider.now = newTestClock(t).Now

	// Materializing the whole profile touches both feeds — and both source
	// defs — but they share one canonical source key: one request.
	items, err := provider.Items(t.Context(), "work", "")
	require.NoError(t, err)
	require.Len(t, items, 2)
	assert.Equal(t, int32(1), api.searchCalls.Load(), "two feeds over one source cost one request")

	filtered, err := provider.Items(t.Context(), "work", "hive-only")
	require.NoError(t, err)
	require.Len(t, filtered, 1)
	assert.Equal(t, "colonyops/hive#42", filtered[0].ID)
	assert.Equal(t, int32(1), api.searchCalls.Load(), "filtered view reads the same cache")
}

func TestLiveProviderNotificationsConditionalFlow(t *testing.T) {
	t.Parallel()

	provider, store, api, clock := newLiveProviderForTest(t)
	def := createTestProfile(t, store)

	items, err := provider.Items(t.Context(), def.ID, "notifications-inbox")
	require.NoError(t, err)
	require.Len(t, items, 2)
	assert.Equal(t, int32(1), api.notifCalls.Load())
	assert.Equal(t, int32(1), api.notifFull.Load())

	// Within the 60s poll floor: served from cache, no request at all.
	clock.Advance(30 * time.Second)
	_, err = provider.Items(t.Context(), def.ID, "notifications-inbox")
	require.NoError(t, err)
	assert.Equal(t, int32(1), api.notifCalls.Load(), "poll floor gates the fetch")

	// Past the floor: a conditional request goes out and answers 304; the
	// cached items survive.
	clock.Advance(31 * time.Second)
	items, err = provider.Items(t.Context(), def.ID, "notifications-inbox")
	require.NoError(t, err)
	require.Len(t, items, 2)
	assert.Equal(t, int32(2), api.notifCalls.Load(), "conditional refetch past the floor")
	assert.Equal(t, int32(1), api.notifFull.Load(), "304 keeps the cached items")
}

func TestRefreshHonorsNotificationsPollInterval(t *testing.T) {
	t.Parallel()

	provider, store, api, clock := newLiveProviderForTest(t)
	api.pollInterval.Store(300)
	def := createTestProfile(t, store)

	changed, err := provider.Refresh(t.Context(), def.ID)
	require.NoError(t, err)
	assert.True(t, changed, "first refresh populates the cache")
	require.Equal(t, int32(1), api.notifCalls.Load())
	searchCalls := api.searchCalls.Load()

	// A manual refresh bypasses the search TTL but not X-Poll-Interval.
	clock.Advance(2 * time.Minute)
	_, err = provider.Refresh(t.Context(), def.ID)
	require.NoError(t, err)
	assert.Equal(t, int32(1), api.notifCalls.Load(), "notifications wait out X-Poll-Interval")
	assert.Equal(t, searchCalls+2, api.searchCalls.Load(), "search sources refetch on manual refresh")

	// Past the server-mandated interval the conditional request goes out.
	clock.Advance(4 * time.Minute)
	_, err = provider.Refresh(t.Context(), def.ID)
	require.NoError(t, err)
	assert.Equal(t, int32(2), api.notifCalls.Load())
	assert.Equal(t, int32(1), api.notifFull.Load(), "unchanged inbox answers 304")
}

// flakyAPIServer serves one search item and an empty inbox, and fails every
// request with the configured status code once set.
func flakyAPIServer(t *testing.T) (*httptest.Server, *atomic.Int32) {
	t.Helper()

	var failStatus atomic.Int32
	mux := http.NewServeMux()
	mux.HandleFunc("/search/issues", func(w http.ResponseWriter, _ *http.Request) {
		if status := failStatus.Load(); status != 0 {
			w.WriteHeader(int(status))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"total_count":1,"items":[
			{"number":42,"title":"Fix spawn env","state":"open",
			 "html_url":"https://github.com/colonyops/hive/pull/42",
			 "repository_url":"https://api.github.com/repos/colonyops/hive",
			 "user":{"login":"hayden"},"labels":[],
			 "pull_request":{"html_url":"https://github.com/colonyops/hive/pull/42"},
			 "updated_at":"2026-07-18T10:00:00Z","created_at":"2026-07-17T00:00:00Z","body":""}
		]}`))
	})
	mux.HandleFunc("/notifications", func(w http.ResponseWriter, _ *http.Request) {
		if status := failStatus.Load(); status != 0 {
			w.WriteHeader(int(status))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[]`))
	})

	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	return server, &failStatus
}

func TestLiveProviderStaleCacheOnTransientError(t *testing.T) {
	t.Parallel()

	server, failStatus := flakyAPIServer(t)
	client := github.NewClient(github.WithAPIBase(server.URL))
	store := newStoreAt(t.TempDir())
	provider := NewLiveProvider(client, github.NewMemoryTokenStore("tok"), store, zerolog.Nop())
	clock := newTestClock(t)
	provider.now = clock.Now
	def := createTestProfile(t, store)

	items, err := provider.Items(t.Context(), def.ID, "")
	require.NoError(t, err)
	require.Len(t, items, 1)

	// Transient server failure past every TTL: stale cache keeps the feed up.
	failStatus.Store(http.StatusInternalServerError)
	clock.Advance(notificationsMinPoll + time.Second)
	items, err = provider.Items(t.Context(), def.ID, "")
	require.NoError(t, err)
	assert.Len(t, items, 1, "stale cache serves through a transient error")

	// Auth failure past the TTL: pass through so the reconnect state shows
	// instead of an indefinitely stale feed.
	failStatus.Store(http.StatusUnauthorized)
	clock.Advance(notificationsMinPoll + time.Second)
	_, err = provider.Items(t.Context(), def.ID, "")
	require.ErrorIs(t, err, github.ErrUnauthorized)
}

func TestLiveProviderRequiresToken(t *testing.T) {
	t.Parallel()

	api := &fakeAPI{}
	server := httptest.NewServer(api.handler())
	t.Cleanup(server.Close)
	client := github.NewClient(github.WithAPIBase(server.URL))
	store := newStoreAt(t.TempDir())
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

func TestLiveProviderCreateFeedMaterializes(t *testing.T) {
	t.Parallel()

	provider, store, _, _ := newLiveProviderForTest(t)
	def := createTestProfile(t, store)

	summary, err := provider.CreateFeed(t.Context(), def.ID, FeedDef{
		Name:    "Hive PRs",
		Sources: []string{"my-prs"},
		Filters: FilterDef{Repos: []string{"colonyops/hive"}},
	})
	require.NoError(t, err)
	assert.Equal(t, "hive-prs", summary.ID)
	assert.Equal(t, "Hive PRs", summary.Name)
	assert.Equal(t, 1, summary.Count, "filters apply to the materialized summary")
	assert.Equal(t, 1, summary.NewCount)

	items, err := provider.Items(t.Context(), def.ID, "hive-prs")
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, "colonyops/hive#42", items[0].ID)
}

// TestLiveProviderLabelsFilterSurvivesNotificationMerge guards the labels
// merge contract: PR#42 is labeled "bug" in the search source and unlabeled
// in its (newer) notification copy, so a labels filter must still match the
// merged item.
func TestLiveProviderLabelsFilterSurvivesNotificationMerge(t *testing.T) {
	t.Parallel()

	api := &fakeAPI{}
	server := httptest.NewServer(api.handler())
	t.Cleanup(server.Close)
	client := github.NewClient(github.WithAPIBase(server.URL))
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "profiles.yaml"), []byte(`sources:
  - id: my-prs
    kind: search
    query: "is:open is:pr author:@me archived:false"
  - id: inbox
    kind: notifications
profiles:
  - id: work
    name: Work
    feeds:
      - id: bugs
        name: Bugs
        sources: [my-prs, inbox]
        filters:
          labels: ["bug"]
`), 0o600))
	store := newStoreAt(dir)
	provider := NewLiveProvider(client, github.NewMemoryTokenStore("tok"), store, zerolog.Nop())
	provider.now = newTestClock(t).Now

	items, err := provider.Items(t.Context(), "work", "bugs")
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, "colonyops/hive#42", items[0].ID)
	assert.Equal(t, []string{"bug"}, items[0].Labels)
	assert.Equal(t, "review_requested", items[0].Reason)
}

// TestLiveProviderConcurrentReadsAndRefreshRaceFree hammers reads
// (sourceItems via Items) against poller-style refreshes (refreshSource)
// while time marches past the TTLs, so conditional 304 responses republish
// cache entries while readers use previously published ones. Run with -race:
// it fails if cache entries are ever mutated after publication.
func TestLiveProviderConcurrentReadsAndRefreshRaceFree(t *testing.T) {
	t.Parallel()

	provider, store, _, clock := newLiveProviderForTest(t)
	def := createTestProfile(t, store)

	// Populate the cache so refreshes take the conditional-GET path.
	_, err := provider.Items(t.Context(), def.ID, "")
	require.NoError(t, err)

	profileDef, err := provider.profileDef(def.ID)
	require.NoError(t, err)
	srcByID, err := provider.sourcesByID()
	require.NoError(t, err)
	sources := distinctSources(profileDef.Feeds, srcByID)
	require.NotEmpty(t, sources)

	stop := make(chan struct{})
	var wg sync.WaitGroup
	for range 3 {
		wg.Add(2)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				_, err := provider.Items(t.Context(), def.ID, "")
				assert.NoError(t, err)
			}
		}()
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				for _, src := range sources {
					_, err := provider.refreshSource(t.Context(), src)
					assert.NoError(t, err)
				}
			}
		}()
	}

	// March past the search TTL (30s) and the notifications poll floor (60s)
	// repeatedly so refreshes keep fetching — the notifications fetch answers
	// 304 and republishes its entry each time.
	for range 20 {
		clock.Advance(31 * time.Second)
		time.Sleep(2 * time.Millisecond)
	}
	close(stop)
	wg.Wait()
}

// TestLiveProviderConcurrentFetchesCoalesce holds a fetch in flight and lets
// a TTL-bypassing refresh race it: singleflight must collapse both callers
// onto one API request.
func TestLiveProviderConcurrentFetchesCoalesce(t *testing.T) {
	t.Parallel()

	api := &fakeAPI{searchStarted: make(chan struct{}, 8), searchRelease: make(chan struct{})}
	provider, store := newLiveProviderWithAPI(t, api)
	provider.now = newTestClock(t).Now
	def := createTestProfile(t, store)

	srcByID, err := provider.sourcesByID()
	require.NoError(t, err)
	src, ok := srcByID["my-prs"]
	require.True(t, ok)

	// A user navigation (empty cache) and a poll tick (TTL bypass) race for
	// the same source.
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		items, err := provider.Items(t.Context(), def.ID, "my-open-prs")
		assert.NoError(t, err)
		assert.Len(t, items, 2)
	}()
	<-api.searchStarted // the first request is in flight and blocked
	go func() {
		defer wg.Done()
		_, err := provider.refreshSource(t.Context(), src)
		assert.NoError(t, err)
	}()
	// Give the second caller time to reach the fetch and join the flight;
	// without coalescing it would open a second request.
	time.Sleep(100 * time.Millisecond)
	close(api.searchRelease)
	wg.Wait()

	assert.Equal(t, int32(1), api.searchCalls.Load(), "concurrent fetches of one source coalesce into one request")
}

func TestLiveProviderRepoFilters(t *testing.T) {
	t.Parallel()

	api := &fakeAPI{}
	server := httptest.NewServer(api.handler())
	t.Cleanup(server.Close)
	client := github.NewClient(github.WithAPIBase(server.URL))
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "profiles.yaml"), []byte(`sources:
  - id: my-prs
    kind: search
    query: "is:open is:pr author:@me archived:false"
profiles:
  - id: work
    name: Work
    feeds:
      - id: prs
        name: My PRs
        sources: [my-prs]
        filters:
          exclude_repos: ["colonyops/docs"]
`), 0o600))
	store := newStoreAt(dir)
	provider := NewLiveProvider(client, github.NewMemoryTokenStore("tok"), store, zerolog.Nop())

	items, err := provider.Items(t.Context(), "work", "prs")
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, "colonyops/hive#42", items[0].ID)
}
