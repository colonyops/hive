package main

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testCache(t *testing.T, ttl time.Duration) *Cache {
	t.Helper()
	cache, err := OpenCache(filepath.Join(t.TempDir(), "cache.db"), ttl)
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, cache.Close()) })
	return cache
}

// testProxy wires a proxy in front of upstream with a fresh cache and store.
func testProxy(t *testing.T, upstream string, ttl time.Duration) (*Proxy, *Store, *Cache) {
	t.Helper()
	cache := testCache(t, ttl)
	store := fixedStore(t)
	return NewProxy(upstream, cache, store, zerolog.Nop()), store, cache
}

func do(t *testing.T, proxy *Proxy, method, target string, body string, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	for name, value := range headers {
		req.Header.Set(name, value)
	}
	rec := httptest.NewRecorder()
	proxy.ServeHTTP(rec, req)
	return rec
}

func TestProxyServesSecondRequestFromCache(t *testing.T) {
	var calls atomic.Int64
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"login":"hay-kot"}`)) //nolint:errcheck // test server
	}))
	defer upstream.Close()

	proxy, _, _ := testProxy(t, upstream.URL, time.Minute)
	auth := map[string]string{"Authorization": "Bearer token-a"}

	first := do(t, proxy, http.MethodGet, "/user", "", auth)
	assert.Equal(t, "miss", first.Header().Get("X-Devserver-Outcome"))
	assert.JSONEq(t, `{"login":"hay-kot"}`, first.Body.String())

	second := do(t, proxy, http.MethodGet, "/user", "", auth)
	assert.Equal(t, "hit", second.Header().Get("X-Devserver-Outcome"))
	assert.JSONEq(t, `{"login":"hay-kot"}`, second.Body.String())

	assert.Equal(t, int64(1), calls.Load(),
		"the whole point of the proxy: the second caller costs no upstream request")
}

func TestProxyIsolatesCacheByToken(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// Echo the caller's identity so a leak across tokens is visible.
		w.Write([]byte(`{"seen":"` + r.Header.Get("Authorization") + `"}`)) //nolint:errcheck // test server
	}))
	defer upstream.Close()

	proxy, _, _ := testProxy(t, upstream.URL, time.Minute)

	a := do(t, proxy, http.MethodGet, "/user", "", map[string]string{"Authorization": "Bearer token-a"}).Body.String()
	b := do(t, proxy, http.MethodGet, "/user", "", map[string]string{"Authorization": "Bearer token-b"}).Body.String()

	// Two accounts can point at one devserver, and GitHub responses are
	// account-scoped. Serving A's cached private data to B would be a leak.
	assert.Contains(t, a, "token-a")
	assert.Contains(t, b, "token-b")
}

func TestProxyRevalidatesWithETagAndServesStoredBody(t *testing.T) {
	var (
		calls          atomic.Int64
		sawIfNoneMatch string
	)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if match := r.Header.Get("If-None-Match"); match != "" {
			sawIfNoneMatch = match
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("ETag", `"v1"`)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"login":"hay-kot"}`)) //nolint:errcheck // test server
	}))
	defer upstream.Close()

	// A zero TTL forces revalidation on every request.
	proxy, _, _ := testProxy(t, upstream.URL, 0)
	auth := map[string]string{"Authorization": "Bearer token-a"}

	do(t, proxy, http.MethodGet, "/user", "", auth) // warm the cache
	second := do(t, proxy, http.MethodGet, "/user", "", auth)

	assert.Equal(t, "revalidated", second.Header().Get("X-Devserver-Outcome"))
	assert.Equal(t, `"v1"`, sawIfNoneMatch, "the stored validator must be replayed upstream")
	// A 304 costs no primary rate-limit quota, and the client still gets the
	// full body — this is headroom the desktop client cannot get on its own.
	assert.JSONEq(t, `{"login":"hay-kot"}`, second.Body.String())
	assert.Equal(t, int64(2), calls.Load())
}

func TestProxyCachesGraphQLByRequestBody(t *testing.T) {
	var calls atomic.Int64
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"data":{"s0":{"nodes":[]}}}`)) //nolint:errcheck // test server
	}))
	defer upstream.Close()

	proxy, _, _ := testProxy(t, upstream.URL, time.Minute)
	auth := map[string]string{"Authorization": "Bearer token-a"}

	queryA := `{"query":"query($q0:String!){s0:search(query:$q0){nodes{number}}}","variables":{"q0":"is:open"}}`
	queryB := `{"query":"query($q0:String!){s0:search(query:$q0){nodes{number}}}","variables":{"q0":"is:closed"}}`

	do(t, proxy, http.MethodPost, "/graphql", queryA, auth)
	do(t, proxy, http.MethodPost, "/graphql", queryA, auth)
	assert.Equal(t, int64(1), calls.Load(), "an identical search must hit cache")

	do(t, proxy, http.MethodPost, "/graphql", queryB, auth)
	// The desktop puts the whole search in the POST body; the URL is identical
	// for every distinct search, so a body-blind key would collapse them all.
	assert.Equal(t, int64(2), calls.Load(), "a different search must miss")
}

func TestProxyCoalescesConcurrentIdenticalRequests(t *testing.T) {
	var calls atomic.Int64
	release := make(chan struct{})
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		<-release
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"login":"hay-kot"}`)) //nolint:errcheck // test server
	}))
	defer upstream.Close()

	proxy, _, _ := testProxy(t, upstream.URL, time.Minute)
	auth := map[string]string{"Authorization": "Bearer token-a"}

	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			do(t, proxy, http.MethodGet, "/user", "", auth)
		})
	}
	// Give every goroutine time to reach the flight group before answering.
	time.Sleep(50 * time.Millisecond)
	close(release)
	wg.Wait()

	// Several dev instances tick on the same 60s boundary; without
	// singleflight that is a burst of identical upstream calls.
	assert.Equal(t, int64(1), calls.Load())
}

func TestProxyServesStaleWhenUpstreamFails(t *testing.T) {
	var fail atomic.Bool
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if fail.Load() {
			panic("upstream down") // httptest turns this into a connection error
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"login":"hay-kot"}`)) //nolint:errcheck // test server
	}))
	defer upstream.Close()

	proxy, _, _ := testProxy(t, upstream.URL, 0)
	auth := map[string]string{"Authorization": "Bearer token-a"}

	do(t, proxy, http.MethodGet, "/user", "", auth) // warm the cache
	fail.Store(true)

	resp := do(t, proxy, http.MethodGet, "/user", "", auth)
	assert.Equal(t, "stale", resp.Header().Get("X-Devserver-Outcome"))
	assert.JSONEq(t, `{"login":"hay-kot"}`, resp.Body.String(),
		"a flaky upstream must not break a dev instance that already has data")
}

func TestProxyDoesNotCacheErrorResponses(t *testing.T) {
	var calls atomic.Int64
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		if count == 1 {
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte(`{"message":"Bad credentials"}`)) //nolint:errcheck // test server
			return
		}
		w.Write([]byte(`{"login":"hay-kot"}`)) //nolint:errcheck // test server
	}))
	defer upstream.Close()

	proxy, _, _ := testProxy(t, upstream.URL, time.Minute)
	auth := map[string]string{"Authorization": "Bearer token-a"}

	first := do(t, proxy, http.MethodGet, "/user", "", auth)
	assert.Equal(t, http.StatusUnauthorized, first.Code)

	// Caching a 401 would keep serving it after the token is fixed; caching a
	// rate-limit 403 would extend the outage past its own reset.
	second := do(t, proxy, http.MethodGet, "/user", "", auth)
	assert.Equal(t, http.StatusOK, second.Code)
	assert.Equal(t, int64(2), calls.Load())
}

func TestProxyPassesThroughUnknownRoutes(t *testing.T) {
	var method, path string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method, path = r.Method, r.URL.Path
		w.WriteHeader(http.StatusResetContent)
	}))
	defer upstream.Close()

	proxy, _, _ := testProxy(t, upstream.URL, time.Minute)
	resp := do(t, proxy, http.MethodPatch, "/notifications/threads/1", `{"read":true}`, nil)

	assert.Equal(t, "passthrough", resp.Header().Get("X-Devserver-Outcome"))
	assert.Equal(t, http.StatusResetContent, resp.Code)
	assert.Equal(t, http.MethodPatch, method)
	assert.Equal(t, "/notifications/threads/1", path)
}

func TestProxyForwardsQueryString(t *testing.T) {
	var query string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`[]`)) //nolint:errcheck // test server
	}))
	defer upstream.Close()

	proxy, _, _ := testProxy(t, upstream.URL, time.Minute)
	do(t, proxy, http.MethodGet, "/notifications?all=true&per_page=50", "",
		map[string]string{"Authorization": "Bearer token-a"})

	assert.Equal(t, "all=true&per_page=50", query)
}

func TestClassify(t *testing.T) {
	cases := []struct {
		method, path string
		kind         routeKind
		cacheable    bool
	}{
		{http.MethodPost, "/graphql", routeGraphQL, true},
		{http.MethodGet, "/notifications", routeNotifications, true},
		{http.MethodGet, "/user", routeUser, true},
		// Not item endpoints: must not be mistaken for one and rewritten. The
		// item-shaped GETs (/pulls/58, /issues/7) are no longer classified at
		// all — the app confirms terminal state through the batched GraphQL
		// state lookup, so these also pass through.
		{http.MethodGet, "/repos/o/r/pulls/58", routeOther, false},
		{http.MethodGet, "/repos/o/r/issues/7", routeOther, false},
		{http.MethodGet, "/repos/o/r/pulls/58/reviews", routeOther, false},
		{http.MethodGet, "/repos/o/r/releases/9", routeOther, false},
		{http.MethodGet, "/repos/o/r/issues/abc", routeOther, false},
		{http.MethodPost, "/repos/o/r/issues/7", routeOther, false},
		{http.MethodGet, "/graphql", routeOther, false},
		{http.MethodPatch, "/notifications/threads/1", routeOther, false},
	}
	for _, tc := range cases {
		got := classify(tc.method, tc.path)
		assert.Equal(t, tc.kind, got.kind, "%s %s kind", tc.method, tc.path)
		assert.Equal(t, tc.cacheable, got.cacheable, "%s %s cacheable", tc.method, tc.path)
	}
}

func TestCacheKeyIsOrderInsensitiveForQueryParams(t *testing.T) {
	a := CacheKey(http.MethodGet, "/notifications", "all=true&per_page=50", "t", nil)
	b := CacheKey(http.MethodGet, "/notifications", "per_page=50&all=true", "t", nil)
	assert.Equal(t, a, b)
}

func TestCacheKeyVariesByEveryInput(t *testing.T) {
	base := CacheKey(http.MethodGet, "/user", "a=1", "token", []byte("body"))
	assert.NotEqual(t, base, CacheKey(http.MethodPost, "/user", "a=1", "token", []byte("body")))
	assert.NotEqual(t, base, CacheKey(http.MethodGet, "/other", "a=1", "token", []byte("body")))
	assert.NotEqual(t, base, CacheKey(http.MethodGet, "/user", "a=2", "token", []byte("body")))
	assert.NotEqual(t, base, CacheKey(http.MethodGet, "/user", "a=1", "other", []byte("body")))
	assert.NotEqual(t, base, CacheKey(http.MethodGet, "/user", "a=1", "token", []byte("other")))
}

func TestCacheKeyIgnoresBearerPrefix(t *testing.T) {
	// The client always sends "Bearer x", but a hand-rolled curl call may not.
	assert.Equal(t,
		CacheKey(http.MethodGet, "/user", "", "Bearer x", nil),
		CacheKey(http.MethodGet, "/user", "", "x", nil))
}

func TestCacheRoundTripAndFreshness(t *testing.T) {
	cache := testCache(t, time.Minute)
	now := time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)
	cache.now = func() time.Time { return now }

	entry := Entry{
		Status:    http.StatusOK,
		Header:    http.Header{"Content-Type": []string{"application/json"}},
		Body:      []byte(`{"a":1}`),
		ETag:      `"v1"`,
		FetchedAt: now.Add(-30 * time.Second),
	}
	require.NoError(t, cache.Put("k", entry))

	stored, ok := cache.Get("k")
	require.True(t, ok)
	assert.Equal(t, entry.Status, stored.Status)
	assert.Equal(t, entry.Body, stored.Body)
	assert.Equal(t, entry.ETag, stored.ETag)
	assert.Equal(t, "application/json", stored.Header.Get("Content-Type"))
	assert.True(t, cache.Fresh(stored))

	// Past the TTL the entry is stale but still present, which is what makes
	// revalidation and stale-on-error possible.
	stored.FetchedAt = now.Add(-2 * time.Minute)
	assert.False(t, cache.Fresh(stored))

	require.NoError(t, cache.Touch("k", now))
	refreshed, ok := cache.Get("k")
	require.True(t, ok)
	assert.True(t, cache.Fresh(refreshed))

	assert.Equal(t, 1, cache.Entries())
	require.NoError(t, cache.Purge())
	assert.Equal(t, 0, cache.Entries())
	_, ok = cache.Get("k")
	assert.False(t, ok)
}

func TestCacheSurvivesReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cache.db")
	cache, err := OpenCache(path, time.Minute)
	require.NoError(t, err)
	require.NoError(t, cache.Put("k", Entry{
		Status: http.StatusOK, Header: http.Header{}, Body: []byte(`{}`), FetchedAt: time.Now(),
	}))
	require.NoError(t, cache.Close())

	// The whole reason the cache is on disk: a devserver restart must not
	// re-fetch everything, any more than a desktop restart should.
	reopened, err := OpenCache(path, time.Minute)
	require.NoError(t, err)
	defer reopened.Close() //nolint:errcheck // test cleanup
	_, ok := reopened.Get("k")
	assert.True(t, ok)
}
