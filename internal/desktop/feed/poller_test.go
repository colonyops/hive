package feed

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/internal/github"
)

// mutableAPIServer serves a single search item whose title can be swapped,
// and an empty notifications inbox. It counts search requests so source
// deduplication across profiles is observable.
type mutableAPIServer struct {
	mu          sync.Mutex
	title       string
	searchCalls atomic.Int32
}

func (m *mutableAPIServer) setTitle(title string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.title = title
}

func (m *mutableAPIServer) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/search/issues", func(w http.ResponseWriter, _ *http.Request) {
		m.searchCalls.Add(1)
		m.mu.Lock()
		title := m.title
		m.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		item := map[string]any{
			"number":         1,
			"title":          title,
			"state":          "open",
			"html_url":       "https://github.com/o/r/pull/1",
			"repository_url": "https://api.github.com/repos/o/r",
			"user":           map[string]any{"login": "hayden"},
			"labels":         []any{},
			"pull_request":   map[string]any{"html_url": "https://github.com/o/r/pull/1"},
			// updated_at moves with the title so the change is observable.
			"updated_at": fmt.Sprintf("2026-07-18T%02d:00:00Z", 10+len(title)%10),
			"created_at": "2026-07-17T00:00:00Z",
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"total_count": 1, "items": []any{item}})
	})
	mux.HandleFunc("/notifications", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[]`))
	})
	return mux
}

func newPollerFixture(t *testing.T) (*LiveProvider, *mutableAPIServer, ProfileDef) {
	t.Helper()
	api := &mutableAPIServer{title: "v1"}
	server := httptest.NewServer(api.handler())
	t.Cleanup(server.Close)

	client := github.NewClient(github.WithAPIBase(server.URL))
	store := newStoreAt(t.TempDir())
	provider := NewLiveProvider(client, github.NewMemoryTokenStore("tok"), store, zerolog.Nop())
	def, err := store.CreateProfile("Triage")
	require.NoError(t, err)
	return provider, api, def
}

func TestRefreshDetectsChanges(t *testing.T) {
	t.Parallel()

	provider, api, def := newPollerFixture(t)

	// First refresh populates an empty cache: that is a change.
	changed, err := provider.Refresh(t.Context(), def.ID)
	require.NoError(t, err)
	assert.True(t, changed, "first refresh populates the cache")

	changed, err = provider.Refresh(t.Context(), def.ID)
	require.NoError(t, err)
	assert.False(t, changed, "same upstream data is not a change")

	api.setTitle("v2-renamed")
	changed, err = provider.Refresh(t.Context(), def.ID)
	require.NoError(t, err)
	assert.True(t, changed, "upstream update is a change")
}

func TestPollOnceNotifiesChangedProfiles(t *testing.T) {
	t.Parallel()

	provider, api, def := newPollerFixture(t)

	var notified []string
	poller := NewPoller(provider, time.Hour, func(profileID string) {
		notified = append(notified, profileID)
	}, zerolog.Nop())

	poller.PollOnce(t.Context())
	assert.Equal(t, []string{def.ID}, notified, "initial poll fills the cache and notifies")

	poller.PollOnce(t.Context())
	assert.Len(t, notified, 1, "no change, no notification")

	api.setTitle("v2-renamed")
	poller.PollOnce(t.Context())
	assert.Equal(t, []string{def.ID, def.ID}, notified)
}

// TestPollOnceDedupesSourcesAcrossProfiles is the point of the source split:
// two profiles reading the same source cost one request per poll, and both
// wake when it changes.
func TestPollOnceDedupesSourcesAcrossProfiles(t *testing.T) {
	t.Parallel()

	api := &mutableAPIServer{title: "v1"}
	server := httptest.NewServer(api.handler())
	t.Cleanup(server.Close)

	client := github.NewClient(github.WithAPIBase(server.URL))
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "profiles.yaml"), []byte(`sources:
  - id: shared
    kind: search
    query: "is:open involves:@me"
profiles:
  - id: one
    name: One
    feeds:
      - {id: f1, name: F1, sources: [shared]}
  - id: two
    name: Two
    feeds:
      - {id: f2, name: F2, sources: [shared]}
      - id: f3
        name: F3
        sources: [shared]
        filters:
          types: [issue]
`), 0o600))
	store := newStoreAt(dir)
	provider := NewLiveProvider(client, github.NewMemoryTokenStore("tok"), store, zerolog.Nop())

	var notified []string
	poller := NewPoller(provider, time.Hour, func(profileID string) {
		notified = append(notified, profileID)
	}, zerolog.Nop())

	poller.PollOnce(t.Context())
	assert.Equal(t, int32(1), api.searchCalls.Load(), "three feeds in two profiles, one source, one request")
	assert.Equal(t, []string{"one", "two"}, notified, "both profiles reference the changed source")

	api.setTitle("v2-renamed")
	poller.PollOnce(t.Context())
	assert.Equal(t, int32(2), api.searchCalls.Load())
	assert.Equal(t, []string{"one", "two", "one", "two"}, notified)
}

func TestPollerStartStop(t *testing.T) {
	t.Parallel()

	provider, _, _ := newPollerFixture(t)
	poller := NewPoller(provider, 10*time.Millisecond, nil, zerolog.Nop())
	poller.Start()
	time.Sleep(30 * time.Millisecond)
	poller.Stop()
	poller.Stop() // idempotent
}
