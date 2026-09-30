package rss

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sync"

	"github.com/hay-kot/appkit/httpclient"
	"github.com/mmcdole/gofeed"
	"github.com/rs/zerolog"

	"github.com/colonyops/hive/cmd/desktop/internal/app/sources/sourcehttp"
)

// sourceName identifies this connector in sourcehttp's log lines, client spans
// and request metrics.
const sourceName = "rss"

// userAgent identifies the app to feed hosts. Go's default is
// "Go-http-client/1.1", which a fair number of CDNs answer with a 403.
const userAgent = "hive-desktop (+https://hivedesktop.com)"

// accept lists what the parser can read, in the order a server should prefer.
// The trailing catch-all is there because plenty of feeds are served as
// text/plain and the parser decides from the document anyway.
const accept = "application/atom+xml, application/rss+xml, application/feed+json, application/xml;q=0.9, text/xml;q=0.8, */*;q=0.5"

// maxBody caps one feed document. A full-content feed carrying a year of posts
// runs to a few megabytes; past this it is an archive, not a subscription.
//
// Exceeding it fails the fetch rather than truncating: a truncated document
// either fails to parse or parses short, and a short parse is a smaller window
// than the feed actually published.
const maxBody = 8 << 20

// Fetchers is the feed client every rss source node fetches through, plus one
// cache entry per feed URL. Two nodes pointing at the same feed share both.
type Fetchers struct {
	client *httpclient.Client
	logger zerolog.Logger
	errs   sourcehttp.Errors

	mu    sync.Mutex
	cache map[string]cacheEntry
}

// cacheEntry is one feed URL's last successful fetch: the validators that make
// the next request conditional, and the entries a 304 answers with.
//
// The two are written and dropped together, and must never diverge — a 304
// with no entries beside it would ingest an empty window, which every
// downstream node reads as "this feed published nothing".
type cacheEntry struct {
	validators sourcehttp.Validators
	entries    []Entry
}

// NewFetchers builds the shared client. There is no base URL: a node names an
// absolute feed URL, which httpclient passes through untouched.
func NewFetchers(logger zerolog.Logger) *Fetchers {
	return &Fetchers{
		client: sourcehttp.New(sourcehttp.Config{Name: sourceName, Logger: logger}, feedHeaders),
		logger: logger,
		errs:   sourcehttp.Errors{Name: sourceName},
		cache:  map[string]cacheEntry{},
	}
}

// InvalidateAll drops every feed's cached window and validators, so the next
// fetch is unconditional. A manual refresh calls it: the user is asserting
// that something changed, which is exactly the case a stale ETag hides.
func (f *Fetchers) InvalidateAll() {
	f.mu.Lock()
	defer f.mu.Unlock()
	clear(f.cache)
}

// Fetch returns the feed's current entries, newest first. A 304 answers from
// the cached window rather than as an empty one.
func (f *Fetchers) Fetch(ctx context.Context, feedURL string) ([]Entry, error) {
	cached, hasCache := f.cached(feedURL)

	entries, notModified, err := f.load(ctx, feedURL, cached.validators, hasCache)
	if err != nil {
		return nil, err
	}
	if notModified {
		f.logger.Debug().Ctx(ctx).Str("source", sourceName).Str("feed", feedRef(feedURL)).
			Int("entries", len(cached.entries)).Msg("feed unchanged")
		return cached.entries, nil
	}

	f.logger.Debug().Ctx(ctx).Str("source", sourceName).Str("feed", feedRef(feedURL)).
		Int("entries", len(entries)).Msg("feed fetched")
	return entries, nil
}

// load performs one request and parses what comes back. notModified is
// reported only when the caller holds a window to answer with; without one the
// request is retried unconditionally, because a 304 the cache cannot answer is
// the empty-window failure this connector must not produce.
func (f *Fetchers) load(ctx context.Context, feedURL string, prev sourcehttp.Validators, hasCache bool) (entries []Entry, notModified bool, err error) {
	var mws []httpclient.Middleware
	if hasCache && !prev.Empty() {
		mws = append(mws, sourcehttp.Conditional(prev))
	}

	resp, err := f.client.Get(ctx, feedURL, mws...)
	if err != nil {
		record(ctx, resultUnreachable)
		return nil, false, f.errs.Unreachable(err)
	}
	defer resp.Body.Close() //nolint:errcheck // read-only body close

	if sourcehttp.NotModified(resp) {
		record(ctx, resultNotModified)
		return nil, true, nil
	}
	if err := f.errs.Status(resp); err != nil {
		record(ctx, resultHTTPError)
		return nil, false, err
	}

	body, err := readBody(resp)
	switch {
	case errors.Is(err, errTooLarge):
		record(ctx, resultTooLarge)
		return nil, false, f.errs.Errorf("%s: %w", feedRef(feedURL), err)
	case err != nil:
		// The body died partway. That is the connection, not the document.
		record(ctx, resultUnreachable)
		return nil, false, f.errs.Unreachable(err)
	}

	// A parser per call: gofeed's is stateful and this one is cheap, which is
	// a smaller price than reasoning about whether the poll loop stays
	// single-threaded forever.
	feed, err := gofeed.NewParser().Parse(bytes.NewReader(body))
	if err != nil {
		record(ctx, resultParseFailure)
		return nil, false, f.errs.Errorf("%s is not a feed this parser can read: %w", feedRef(feedURL), err)
	}

	record(ctx, resultModified)
	entries = entriesOf(feed)
	f.store(feedURL, cacheEntry{validators: sourcehttp.ReadValidators(resp.Header), entries: entries})
	return entries, false, nil
}

func (f *Fetchers) cached(feedURL string) (cacheEntry, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	entry, ok := f.cache[feedURL]
	return entry, ok
}

func (f *Fetchers) store(feedURL string, entry cacheEntry) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.cache[feedURL] = entry
}

// errTooLarge separates an oversized document from a body that died partway.
// They are the same io.ReadAll return but different failures to report.
var errTooLarge = fmt.Errorf("the feed is larger than %d bytes", maxBody)

// readBody reads the document, failing rather than truncating past maxBody.
func readBody(resp *http.Response) ([]byte, error) {
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBody+1))
	if err != nil {
		return nil, err
	}
	if len(body) > maxBody {
		return nil, errTooLarge
	}
	return body, nil
}

// feedHeaders states what this client is and what it can read. Both are set as
// middleware rather than per call so every request carries them.
func feedHeaders(next httpclient.Doer) httpclient.Doer {
	return httpclient.DoerFunc(func(req *http.Request) (*http.Response, error) {
		req.Header.Set("Accept", accept)
		req.Header.Set("User-Agent", userAgent)
		return next.Do(req)
	})
}

func record(ctx context.Context, result string) {
	fetchCounter.Add(ctx, 1, fetchAttrs[result])
}

// feedRef is the form of a feed URL that is safe to write into a log line, an
// error, or a span: scheme, host and path, with the query dropped whole.
//
// Whole, rather than redacting the keys sourcehttp knows: a private feed's
// token is a query parameter under whatever name its reader chose, and an
// allow-list cannot be complete. Errors reach the Activity log, which is
// exported and read; the path already says which feed this is.
func feedRef(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Host == "" {
		return "the feed URL"
	}
	return parsed.Scheme + "://" + parsed.Host + parsed.Path
}
