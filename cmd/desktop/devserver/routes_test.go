package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testHandler wires the full route set over an upstream that counts calls, so
// a test can assert that a request never reached GitHub at all.
func testHandler(t *testing.T, upstream string) (http.Handler, *Proxy) {
	t.Helper()
	cfg := Config{Upstream: upstream}
	require.NoError(t, cfg.normalize())
	cache := testCache(t, cfg.Cache.TTL)
	store := fixedStore(t)
	proxy := NewProxy(cfg.Upstream, cache, store, zerolog.Nop())
	pusher := NewPusher(cfg.Webhooks, zerolog.Nop())
	control := NewControl(cfg, store, cache, proxy, pusher, zerolog.Nop())
	return newHandler(control, proxy, zerolog.Nop()), proxy
}

func get(t *testing.T, handler http.Handler, path string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	return rec
}

// TestBrowserChromePathsNeverReachUpstream is the regression test for the
// dashboard inflating its own numbers: /favicon.ico shares an origin with the
// GitHub passthrough, so before this it was forwarded to api.github.com and
// counted as an upstream call on every page refresh.
func TestBrowserChromePathsNeverReachUpstream(t *testing.T) {
	var calls atomic.Int64
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Write([]byte(`{}`)) //nolint:errcheck // test server
	}))
	defer upstream.Close()

	handler, proxy := testHandler(t, upstream.URL)

	for _, path := range browserChromePaths {
		assert.Equal(t, http.StatusNoContent, get(t, handler, path).Code, path)
	}

	assert.Equal(t, int64(0), calls.Load(), "browser chrome must not generate GitHub traffic")
	stats, recent := proxy.Snapshot()
	assert.Zero(t, stats.Requests, "and must not be counted as proxied requests")
	assert.Zero(t, stats.UpstreamCalls)
	assert.Empty(t, recent, "nor pollute the activity list")
}

// TestDashboardLoadCostsNothingUpstream covers the whole page load the way a
// browser performs it: the document, its favicon, and the state poll.
func TestDashboardLoadCostsNothingUpstream(t *testing.T) {
	var calls atomic.Int64
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Write([]byte(`{}`)) //nolint:errcheck // test server
	}))
	defer upstream.Close()

	handler, proxy := testHandler(t, upstream.URL)

	for range 3 { // three refreshes
		page := get(t, handler, "/")
		require.Equal(t, http.StatusOK, page.Code)
		assert.Contains(t, page.Header().Get("Content-Type"), "text/html")
		get(t, handler, "/favicon.ico")
		require.Equal(t, http.StatusOK, get(t, handler, "/_ctl/state").Code)
	}

	assert.Equal(t, int64(0), calls.Load())
	stats, _ := proxy.Snapshot()
	assert.Zero(t, stats.UpstreamCalls, "refreshing the dashboard must not change the numbers it reports")
}

// TestDashboardDeclaresInlineFavicon keeps the browser from asking for
// /favicon.ico in the first place; the route above is the backstop.
func TestDashboardDeclaresInlineFavicon(t *testing.T) {
	html := string(dashboardHTML)
	require.Contains(t, html, `rel="icon"`)
	assert.Contains(t, html, "data:image/svg+xml", "the icon must be inline, not a fetched URL")
	// A strict CSP is not in play here, but an external icon would reintroduce
	// exactly the request this fixes.
	assert.NotContains(t, html, `rel="icon" href="/`)
}

// TestGitHubPathsStillProxy guards the fix's blast radius: only browser chrome
// is intercepted, and real API paths — including ones devserver does not model
// — still reach GitHub.
func TestGitHubPathsStillProxy(t *testing.T) {
	var seen []string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{}`)) //nolint:errcheck // test server
	}))
	defer upstream.Close()

	handler, _ := testHandler(t, upstream.URL)
	for _, path := range []string{"/user", "/notifications", "/repos/o/r/pulls/1", "/rate_limit"} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.Header.Set("Authorization", "Bearer tok")
		handler.ServeHTTP(rec, req)
		assert.NotEqual(t, http.StatusNoContent, rec.Code, path)
	}
	assert.Equal(t, []string{"/user", "/notifications", "/repos/o/r/pulls/1", "/rate_limit"}, seen)
}

func TestControlRoutesAreNotProxied(t *testing.T) {
	var calls atomic.Int64
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
	}))
	defer upstream.Close()

	handler, _ := testHandler(t, upstream.URL)
	require.Equal(t, http.StatusOK, get(t, handler, "/_ctl/state").Code)

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/_ctl/overlays/clear",
		strings.NewReader(`{}`)))
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, int64(0), calls.Load())
}

func TestRootServesDashboardAndOnlyExactly(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"proxied":true}`)) //nolint:errcheck // test server
	}))
	defer upstream.Close()

	handler, _ := testHandler(t, upstream.URL)
	assert.Contains(t, get(t, handler, "/").Body.String(), "hive devserver")
	// A non-root path must proxy, not serve HTML — {$} exists for this.
	assert.Contains(t, get(t, handler, "/anything").Body.String(), "proxied")
}

// TestDashboardItemsPanelScales pins the structure that keeps the items panel
// usable against a real feed. A feed observing hundreds of items previously
// rendered every one as a card with the full eleven-button action set, which
// is thousands of buttons tall.
//
// The JS behaviour itself has no automated coverage — this asserts the hooks
// it depends on exist, so a refactor cannot quietly drop them.
func TestDashboardItemsPanelScales(t *testing.T) {
	html := string(dashboardHTML)

	// Filter and toggle live outside the polled region; re-creating them every
	// 2s would steal focus and wipe what is being typed.
	require.Contains(t, html, `id="items-filter"`)
	require.Contains(t, html, `id="items-overlaid"`)
	assert.Contains(t, html, `id="items-count"`)
	assert.Contains(t, html, `id="items-more"`)

	// Paged rather than rendering everything.
	assert.Contains(t, html, "ITEMS_PAGE")
	assert.Contains(t, html, "itemsView.limit")

	// Actions render only for the open row.
	assert.Contains(t, html, "itemsView.open")

	// View state must not be reset by the poll.
	assert.Contains(t, html, "const itemsView")
	filterIdx := strings.Index(html, "const itemsView")
	refreshIdx := strings.Index(html, "async function refresh")
	require.Positive(t, filterIdx)
	require.Positive(t, refreshIdx)
	assert.Less(t, filterIdx, refreshIdx, "view state must be declared outside render/refresh")
}
