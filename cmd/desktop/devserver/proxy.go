package main

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/rs/zerolog"
	"golang.org/x/sync/singleflight"
)

// maxBodyBytes caps a proxied request body. The desktop's largest request by
// far is a batched GraphQL search document, which is kilobytes.
const maxBodyBytes = 4 << 20

// upstreamTimeout bounds one upstream call. It matches the desktop client's
// own 30s request timeout so the proxy never becomes the shorter fuse.
const upstreamTimeout = 30 * time.Second

// hopByHopHeaders are per-connection headers that must not be forwarded or
// cached (RFC 9110 §7.6.1).
var hopByHopHeaders = []string{
	"Connection", "Keep-Alive", "Proxy-Authenticate", "Proxy-Authorization",
	"Te", "Trailer", "Transfer-Encoding", "Upgrade",
}

// Proxy fronts the GitHub API with a shared cache and the overlay rewriter.
type Proxy struct {
	upstream string
	cache    *Cache
	store    *Store
	client   *http.Client
	logger   zerolog.Logger
	flight   singleflight.Group

	mu     sync.Mutex
	stats  Stats
	recent []LogEntry
}

// Stats are the counters the dashboard renders.
type Stats struct {
	Requests       int64     `json:"requests"`
	CacheHits      int64     `json:"cacheHits"`
	UpstreamCalls  int64     `json:"upstreamCalls"`
	Revalidated    int64     `json:"revalidated"`
	ServedStale    int64     `json:"servedStale"`
	Errors         int64     `json:"errors"`
	LastUpstreamAt time.Time `json:"lastUpstreamAt"`
}

// LogEntry is one proxied request, for the dashboard's recent-activity list.
type LogEntry struct {
	At       time.Time `json:"at"`
	Method   string    `json:"method"`
	Path     string    `json:"path"`
	Label    string    `json:"label"`
	Outcome  string    `json:"outcome"`
	Status   int       `json:"status"`
	Overlaid bool      `json:"overlaid"`
}

// recentLimit bounds the in-memory request log.
const recentLimit = 50

func NewProxy(upstream string, cache *Cache, store *Store, logger zerolog.Logger) *Proxy {
	return &Proxy{
		upstream: upstream,
		cache:    cache,
		store:    store,
		client:   &http.Client{Timeout: upstreamTimeout},
		logger:   logger,
	}
}

// Snapshot returns the current counters and recent requests.
func (p *Proxy) Snapshot() (Stats, []LogEntry) {
	p.mu.Lock()
	defer p.mu.Unlock()
	recent := make([]LogEntry, len(p.recent))
	copy(recent, p.recent)
	return p.stats, recent
}

func (p *Proxy) record(entry LogEntry, mutate func(*Stats)) {
	p.mu.Lock()
	defer p.mu.Unlock()
	mutate(&p.stats)
	p.recent = append([]LogEntry{entry}, p.recent...)
	if len(p.recent) > recentLimit {
		p.recent = p.recent[:recentLimit]
	}
}

// ServeHTTP proxies one request. Cacheable requests are served from the shared
// cache when fresh, revalidated when stale, and coalesced so concurrent
// identical calls from several dev instances become one upstream request.
func (p *Proxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	if err != nil {
		http.Error(w, "reading request body", http.StatusBadRequest)
		return
	}

	route := classify(r.Method, r.URL.Path)
	if !route.cacheable {
		p.passthrough(w, r, body, route)
		return
	}

	key := CacheKey(r.Method, r.URL.Path, r.URL.RawQuery, r.Header.Get("Authorization"), body)
	if entry, ok := p.cache.Get(key); ok && p.cache.Fresh(entry) {
		p.serve(w, entry, route, "hit", func(s *Stats) { s.Requests++; s.CacheHits++ })
		return
	}

	// singleflight collapses the thundering herd that several dev instances
	// ticking on the same 60s boundary would otherwise produce.
	result, err, _ := p.flight.Do(key, func() (any, error) {
		return p.fetch(r, body, key)
	})
	if err != nil {
		if stale, ok := p.cache.Get(key); ok {
			p.logger.Warn().Err(err).Str("path", r.URL.Path).Msg("upstream failed; serving stale cache")
			p.serve(w, stale, route, "stale", func(s *Stats) { s.Requests++; s.ServedStale++ })
			return
		}
		p.record(LogEntry{
			At: time.Now().UTC(), Method: r.Method, Path: r.URL.Path,
			Label: route.label, Outcome: "error",
		}, func(s *Stats) { s.Requests++; s.Errors++ })
		p.logger.Error().Err(err).Str("path", r.URL.Path).Msg("upstream failed with no cached copy")
		http.Error(w, "upstream request failed: "+err.Error(), http.StatusBadGateway)
		return
	}

	// singleflight hands back `any`; the only producer is the closure above,
	// which returns p.fetch's typed result. A failed assertion is therefore a
	// programming error here, not an upstream one — hence 500, not the 502 the
	// upstream failure path returns.
	fetched, ok := result.(fetchResult)
	if !ok {
		p.logger.Error().Str("path", r.URL.Path).Msgf("singleflight returned %T, want fetchResult", result)
		http.Error(w, "internal proxy error", http.StatusInternalServerError)
		return
	}
	p.serve(w, fetched.entry, route, fetched.outcome, func(s *Stats) {
		s.Requests++
		s.UpstreamCalls++
		s.LastUpstreamAt = time.Now().UTC()
		if fetched.outcome == "revalidated" {
			s.Revalidated++
		}
	})
}

type fetchResult struct {
	entry   Entry
	outcome string
}

// fetch calls upstream, sending the stored validators so an unchanged resource
// comes back as a 304 — which does not draw down the primary rate-limit quota.
func (p *Proxy) fetch(r *http.Request, body []byte, key string) (fetchResult, error) {
	stored, hasStored := p.cache.Get(key)

	req, err := http.NewRequestWithContext(r.Context(), r.Method, p.upstream+r.URL.Path, bytes.NewReader(body))
	if err != nil {
		return fetchResult{}, fmt.Errorf("build upstream request: %w", err)
	}
	req.URL.RawQuery = r.URL.RawQuery
	copyHeaders(req.Header, r.Header)
	// Let Go negotiate and transparently decompress the response: the overlay
	// rewriter needs plain JSON, and a cached gzip body could not be rewritten.
	req.Header.Del("Accept-Encoding")
	if hasStored {
		if stored.ETag != "" {
			req.Header.Set("If-None-Match", stored.ETag)
		}
		// Only send If-Modified-Since when the client did not, so the
		// desktop's own conditional notifications polling keeps its semantics.
		if stored.LastModified != "" && r.Header.Get("If-Modified-Since") == "" && stored.ETag == "" {
			req.Header.Set("If-Modified-Since", stored.LastModified)
		}
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return fetchResult{}, err
	}
	defer resp.Body.Close() //nolint:errcheck // read-only body close

	now := time.Now().UTC()
	if resp.StatusCode == http.StatusNotModified && hasStored {
		if err := p.cache.Touch(key, now); err != nil {
			p.logger.Warn().Err(err).Msg("cache touch failed")
		}
		stored.FetchedAt = now
		return fetchResult{entry: stored, outcome: "revalidated"}, nil
	}

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fetchResult{}, fmt.Errorf("read upstream body: %w", err)
	}
	entry := Entry{
		Status:       resp.StatusCode,
		Header:       cacheableHeaders(resp.Header),
		Body:         responseBody,
		ETag:         resp.Header.Get("ETag"),
		LastModified: resp.Header.Get("Last-Modified"),
		FetchedAt:    now,
	}
	// Only success is worth caching. Caching a 401 would keep serving it after
	// the token is fixed; caching a 403 rate-limit reply would extend the
	// outage past its own reset.
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		if err := p.cache.Put(key, entry); err != nil {
			p.logger.Warn().Err(err).Msg("cache write failed")
		}
	}
	return fetchResult{entry: entry, outcome: "miss"}, nil
}

// serve writes a cached or freshly fetched entry to the client, applying the
// overlay for the route.
func (p *Proxy) serve(w http.ResponseWriter, entry Entry, route route, outcome string, mutate func(*Stats)) {
	body := entry.Body
	overlaid := false
	if entry.Status >= 200 && entry.Status < 300 {
		if rewritten := p.rewrite(entry.Body, route); !bytes.Equal(rewritten, entry.Body) {
			body = rewritten
			overlaid = true
		}
	}

	header := w.Header()
	for name, values := range entry.Header {
		for _, value := range values {
			header.Add(name, value)
		}
	}
	// The rewriter changes the body length, and a stored Content-Length would
	// now be a lie.
	header.Del("Content-Length")
	header.Set("X-Devserver-Outcome", outcome)
	if overlaid {
		header.Set("X-Devserver-Overlaid", "true")
	}
	w.WriteHeader(entry.Status)
	if _, err := w.Write(body); err != nil {
		p.logger.Debug().Err(err).Msg("writing proxied response")
	}

	p.record(LogEntry{
		At: time.Now().UTC(), Method: route.method, Path: route.path,
		Label: route.label, Outcome: outcome, Status: entry.Status, Overlaid: overlaid,
	}, mutate)
}

// rewrite applies the overlay store to a response body for its route.
func (p *Proxy) rewrite(body []byte, route route) []byte {
	switch route.kind {
	case routeGraphQL:
		return p.store.RewriteGraphQL(body)
	case routeNotifications:
		return p.store.RewriteNotifications(body)
	default:
		return body
	}
}

// passthrough forwards a request the proxy does not cache, unchanged in both
// directions. Anything the desktop might add later — marking a notification
// read, say — keeps working without devserver needing to know about it.
func (p *Proxy) passthrough(w http.ResponseWriter, r *http.Request, body []byte, route route) {
	req, err := http.NewRequestWithContext(r.Context(), r.Method, p.upstream+r.URL.Path, bytes.NewReader(body))
	if err != nil {
		http.Error(w, "building upstream request", http.StatusInternalServerError)
		return
	}
	req.URL.RawQuery = r.URL.RawQuery
	copyHeaders(req.Header, r.Header)

	resp, err := p.client.Do(req)
	if err != nil {
		p.record(LogEntry{
			At: time.Now().UTC(), Method: r.Method, Path: r.URL.Path,
			Label: route.label, Outcome: "error",
		}, func(s *Stats) { s.Requests++; s.Errors++ })
		http.Error(w, "upstream request failed: "+err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close() //nolint:errcheck // read-only body close

	header := w.Header()
	for name, values := range cacheableHeaders(resp.Header) {
		for _, value := range values {
			header.Add(name, value)
		}
	}
	header.Set("X-Devserver-Outcome", "passthrough")
	w.WriteHeader(resp.StatusCode)
	if _, err := io.Copy(w, resp.Body); err != nil {
		p.logger.Debug().Err(err).Msg("writing passthrough response")
	}

	p.record(LogEntry{
		At: time.Now().UTC(), Method: r.Method, Path: r.URL.Path, Label: route.label,
		Outcome: "passthrough", Status: resp.StatusCode,
	}, func(s *Stats) {
		s.Requests++
		s.UpstreamCalls++
		s.LastUpstreamAt = time.Now().UTC()
	})
}

func copyHeaders(dst, src http.Header) {
	for name, values := range src {
		for _, value := range values {
			dst.Add(name, value)
		}
	}
	for _, name := range hopByHopHeaders {
		dst.Del(name)
	}
	dst.Del("Host")
}

// cacheableHeaders strips hop-by-hop headers and the framing headers that no
// longer describe the body once Go has decompressed it or the overlay has
// rewritten it.
func cacheableHeaders(src http.Header) http.Header {
	out := make(http.Header, len(src))
	for name, values := range src {
		out[name] = append([]string(nil), values...)
	}
	for _, name := range hopByHopHeaders {
		out.Del(name)
	}
	out.Del("Content-Length")
	out.Del("Content-Encoding")
	return out
}

// ── Route classification ─────────────────────────────────────────────────────

type routeKind int

const (
	routeOther routeKind = iota
	routeGraphQL
	routeNotifications
	routeUser
)

type route struct {
	kind      routeKind
	method    string
	path      string
	label     string
	cacheable bool
}

// classify identifies the GitHub endpoints devserver knows how to cache and
// rewrite. Everything else falls through to routeOther and is passed straight
// to GitHub.
//
// POST /graphql is cacheable despite being a POST: the desktop uses it as a
// read, batching every search into one document
// (internal/app/sources/github/ghclient/client.go, SearchIssuesBatch).
func classify(method, path string) route {
	r := route{method: method, path: path, label: path}
	switch {
	case method == http.MethodPost && path == "/graphql":
		r.kind, r.label, r.cacheable = routeGraphQL, "graphql search", true
	case method == http.MethodGet && path == "/notifications":
		r.kind, r.label, r.cacheable = routeNotifications, "notifications", true
	case method == http.MethodGet && path == "/user":
		r.kind, r.label, r.cacheable = routeUser, "user", true
	}
	return r
}
