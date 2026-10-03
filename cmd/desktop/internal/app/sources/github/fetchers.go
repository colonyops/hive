package github

import (
	"sync"
	"time"

	"github.com/rs/zerolog"

	"github.com/colonyops/hive/cmd/desktop/internal/app/activity"
	"github.com/colonyops/hive/cmd/desktop/internal/app/sources/github/feed"
	"github.com/colonyops/hive/cmd/desktop/internal/app/sources/github/ghclient"
	"github.com/colonyops/hive/internal/platform/credentials"
)

// Fetchers hands out one feed.LiveProvider per credential, constructing them
// on first use.
//
// A LiveProvider is already a per-account object even though it was written
// as a singleton: its response cache and conditional-request state hold one
// account's items, and its rate-limit cooldown "pauses all fetches for the
// current token". Sharing one across accounts would serve one account's items
// to another and let one account's rate limit stall every other. So accounts
// get one each, and the shared thing is this registry.
//
// Providers are never evicted. The set is bounded by the number of connected
// accounts, and a disconnected account's provider holds only a response cache
// that its next resolve would miss anyway.
type Fetchers struct {
	// client is a template, not a connection: every request goes through
	// WithTokenCopy, so one client backs every account's provider.
	client *ghclient.Client
	creds  credentials.Store
	logger zerolog.Logger

	mu        sync.Mutex
	byRef     map[credentials.Ref]*feed.LiveProvider
	searchTTL time.Duration
	recorder  activity.Recorder
}

// NewFetchers builds the per-account provider registry over the owned
// GitHub client.
func NewFetchers(client *ghclient.Client, creds credentials.Store, logger zerolog.Logger) *Fetchers {
	return &Fetchers{
		client:    client,
		creds:     creds,
		logger:    logger,
		byRef:     map[credentials.Ref]*feed.LiveProvider{},
		searchTTL: feed.DefaultPollInterval,
	}
}

// For returns the fetcher for one credential. The provider resolves its token
// through the credential store on every request, so connecting, rotating, or
// disconnecting the account takes effect on the next fetch.
func (f *Fetchers) For(ref credentials.Ref) *feed.LiveProvider {
	f.mu.Lock()
	defer f.mu.Unlock()

	if live, ok := f.byRef[ref]; ok {
		return live
	}

	live := feed.NewLiveProvider(f.client, credentials.Bind(f.creds, ref), f.logger)
	live.SetSearchTTL(f.searchTTL)
	if f.recorder != nil {
		live.SetRecorder(f.recorder)
	}
	f.byRef[ref] = live
	return live
}

// SetSearchTTL applies the search cache TTL to every fetcher, now and on
// every one built later. The poll interval is a global setting, so a fetcher
// created after the user changed it must not silently keep the default.
func (f *Fetchers) SetSearchTTL(ttl time.Duration) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.searchTTL = ttl
	for _, live := range f.byRef {
		live.SetSearchTTL(ttl)
	}
}

// SetRecorder attaches an activity recorder to every fetcher, now and later.
func (f *Fetchers) SetRecorder(r activity.Recorder) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.recorder = r
	for _, live := range f.byRef {
		live.SetRecorder(r)
	}
}

// InvalidateAll drops every account's fetch cache.
func (f *Fetchers) InvalidateAll() {
	f.mu.Lock()
	all := make([]*feed.LiveProvider, 0, len(f.byRef))
	for _, live := range f.byRef {
		all = append(all, live)
	}
	f.mu.Unlock()
	for _, live := range all {
		live.Invalidate()
	}
}
