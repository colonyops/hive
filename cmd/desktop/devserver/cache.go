package main

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// busyTimeoutMS matches the desktop's SQLite convention: several dev instances
// drive the proxy concurrently, so a second writer waits rather than failing.
const busyTimeoutMS = 5000

// Cache is the proxy's on-disk response cache. It is the whole point of the
// proxy: N dev instances and every restart of each of them collapse onto one
// upstream request per unique call per TTL.
//
// It also stores validators, which the desktop client itself does not
// (tracked in #62). Revalidating with If-None-Match costs no primary
// rate-limit quota when upstream answers 304, so an expired entry is usually
// far cheaper than a fresh fetch.
type Cache struct {
	db  *sql.DB
	ttl time.Duration
	now func() time.Time
}

// Entry is one cached upstream response.
type Entry struct {
	Status       int
	Header       http.Header
	Body         []byte
	ETag         string
	LastModified string
	FetchedAt    time.Time
}

const cacheSchema = `
CREATE TABLE IF NOT EXISTS responses (
	key           TEXT PRIMARY KEY,
	status        INTEGER NOT NULL,
	headers       BLOB NOT NULL,
	body          BLOB NOT NULL,
	etag          TEXT NOT NULL DEFAULT '',
	last_modified TEXT NOT NULL DEFAULT '',
	fetched_at    INTEGER NOT NULL
);`

// OpenCache opens (creating if needed) the cache database at path.
func OpenCache(path string, ttl time.Duration) (*Cache, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("create cache dir: %w", err)
	}
	dsn := fmt.Sprintf("file:%s?_txlock=immediate&_pragma=journal_mode(WAL)&_pragma=busy_timeout(%d)", path, busyTimeoutMS)
	conn, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open cache %s: %w", path, err)
	}
	if _, err := conn.Exec(cacheSchema); err != nil {
		conn.Close() //nolint:errcheck,gosec // the open error is what matters
		return nil, fmt.Errorf("create cache schema: %w", err)
	}
	return &Cache{db: conn, ttl: ttl, now: time.Now}, nil
}

func (c *Cache) Close() error { return c.db.Close() }

// Get returns the cached entry for key, if one exists. A row that fails to
// decode is reported as a miss rather than an error: a corrupt cache entry
// should cost one upstream call, not break the proxy.
func (c *Cache) Get(key string) (Entry, bool) {
	var (
		entry     Entry
		headers   []byte
		fetchedAt int64
	)
	row := c.db.QueryRow(
		`SELECT status, headers, body, etag, last_modified, fetched_at FROM responses WHERE key = ?`, key)
	err := row.Scan(&entry.Status, &headers, &entry.Body, &entry.ETag, &entry.LastModified, &fetchedAt)
	if err != nil {
		return Entry{}, false
	}
	if err := json.Unmarshal(headers, &entry.Header); err != nil {
		return Entry{}, false
	}
	entry.FetchedAt = time.UnixMilli(fetchedAt).UTC()
	return entry, true
}

// Put stores an entry, replacing any existing one for the key.
func (c *Cache) Put(key string, entry Entry) error {
	headers, err := json.Marshal(entry.Header)
	if err != nil {
		return fmt.Errorf("encode cached headers: %w", err)
	}
	_, err = c.db.Exec(
		`INSERT INTO responses (key, status, headers, body, etag, last_modified, fetched_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(key) DO UPDATE SET
		   status = excluded.status, headers = excluded.headers, body = excluded.body,
		   etag = excluded.etag, last_modified = excluded.last_modified,
		   fetched_at = excluded.fetched_at`,
		key, entry.Status, headers, entry.Body, entry.ETag, entry.LastModified,
		entry.FetchedAt.UnixMilli())
	if err != nil {
		return fmt.Errorf("write cache entry: %w", err)
	}
	return nil
}

// Touch refreshes an entry's freshness without rewriting its body. It is the
// 304 path: upstream confirmed the stored copy is current.
func (c *Cache) Touch(key string, at time.Time) error {
	_, err := c.db.Exec(`UPDATE responses SET fetched_at = ? WHERE key = ?`, at.UnixMilli(), key)
	if err != nil {
		return fmt.Errorf("touch cache entry: %w", err)
	}
	return nil
}

// Fresh reports whether an entry can be served without revalidating.
func (c *Cache) Fresh(entry Entry) bool {
	return c.now().Sub(entry.FetchedAt) < c.ttl
}

// Purge empties the cache.
func (c *Cache) Purge() error {
	if _, err := c.db.Exec(`DELETE FROM responses`); err != nil {
		return fmt.Errorf("purge cache: %w", err)
	}
	return nil
}

// Entries reports how many responses are cached.
func (c *Cache) Entries() int {
	var count int
	if err := c.db.QueryRow(`SELECT COUNT(*) FROM responses`).Scan(&count); err != nil {
		return 0
	}
	return count
}

// CacheKey is the identity of an upstream request.
//
// The token hash is part of the key on purpose. Two accounts' dev instances
// can point at one devserver, and GitHub's responses are account-scoped —
// keying only on the request would serve one account's private issues to the
// other. The token is hashed, never stored.
//
// The body is part of the key because the desktop batches searches through
// POST /graphql, where the query lives entirely in the body and the URL is
// identical for every distinct search.
func CacheKey(method, path, rawQuery, token string, body []byte) string {
	digest := sha256.New()
	write := func(parts ...string) {
		for _, part := range parts {
			digest.Write([]byte(part)) //nolint:errcheck // hash writes never fail
			digest.Write([]byte{0})    //nolint:errcheck // field separator
		}
	}
	write(method, path, canonicalQuery(rawQuery), hashToken(token))
	digest.Write(body) //nolint:errcheck // hash writes never fail
	return hex.EncodeToString(digest.Sum(nil))
}

// canonicalQuery sorts query parameters so semantically identical requests
// that differ only in parameter order share one cache entry.
func canonicalQuery(rawQuery string) string {
	if rawQuery == "" {
		return ""
	}
	parts := strings.Split(rawQuery, "&")
	sort.Strings(parts)
	return strings.Join(parts, "&")
}

// hashToken reduces a bearer token to a stable, non-reversible identity.
func hashToken(token string) string {
	token = strings.TrimSpace(strings.TrimPrefix(token, "Bearer "))
	if token == "" {
		return "anonymous"
	}
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:8])
}
