package relay

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
	"time"

	"github.com/rs/zerolog"

	"github.com/colonyops/hive/internal/platform/credentials"
)

// The relay's own defaults, used until /v1/me answers with the real limits.
const (
	defaultRetention = 14 * 24 * time.Hour
	defaultMaxKeys   = 1000
)

var ErrNotConnected = errors.New("relay account is not connected")

// Fetchers builds a client per account from its binding and holds each node's
// cursor and parsed item set. The set lives only in memory: a restart refetches
// from the start of the relay's retention, which its caps bound.
type Fetchers struct {
	instances *InstanceStore
	creds     credentials.Store
	version   string
	log       zerolog.Logger

	mu     sync.Mutex
	nodes  map[string]*nodeState
	limits map[credentials.Ref]Limits
}

func NewFetchers(instances *InstanceStore, creds credentials.Store, version string, log zerolog.Logger) *Fetchers {
	return &Fetchers{
		instances: instances,
		creds:     creds,
		version:   version,
		log:       log,
		nodes:     map[string]*nodeState{},
		limits:    map[credentials.Ref]Limits{},
	}
}

// For returns a client for a connected account. The binding is read on every
// call and the key on every request, so connect, rotate and disconnect take
// effect on the next tick.
func (f *Fetchers) For(ref credentials.Ref) (*Client, error) {
	binding, err := f.instances.Get(ref)
	if err != nil {
		return nil, err
	}
	if binding.URL == "" {
		return nil, fmt.Errorf("%w: %s", ErrNotConnected, ref)
	}
	resolve := credentials.Bind(f.creds, ref)
	token := func() string {
		key, err := resolve()
		if err != nil {
			f.log.Warn().Err(err).Str("credential", ref.String()).Msg("relay: reading the API key failed")
			return ""
		}
		return key
	}
	return NewClient(binding.URL, token, f.version, f.log), nil
}

// InvalidateAll drops every node's cursor and item set and the cached account
// limits, so the next tick refetches the relay's whole window.
func (f *Fetchers) InvalidateAll() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.nodes = map[string]*nodeState{}
	f.limits = map[credentials.Ref]Limits{}
}

func (f *Fetchers) state(nodeID string) *nodeState {
	f.mu.Lock()
	defer f.mu.Unlock()
	st, ok := f.nodes[nodeID]
	if !ok {
		st = &nodeState{items: map[string]item{}}
		f.nodes[nodeID] = st
	}
	return st
}

// window sizes prune from the account's limits. A failed /v1/me falls back to
// the defaults without caching them, so the next tick asks again.
func (f *Fetchers) window(ctx context.Context, ref credentials.Ref, client *Client) (time.Duration, int) {
	f.mu.Lock()
	limits, ok := f.limits[ref]
	f.mu.Unlock()
	if !ok {
		me, err := client.Me(ctx)
		if err != nil {
			f.log.Debug().Err(err).Str("credential", ref.String()).Msg("relay: reading account limits failed; using defaults")
			return defaultRetention, defaultMaxKeys
		}
		limits = me.Limits
		f.mu.Lock()
		f.limits[ref] = limits
		f.mu.Unlock()
	}
	retention := time.Duration(limits.RetentionSeconds) * time.Second
	if retention <= 0 {
		retention = defaultRetention
	}
	maxKeys := limits.DeliveriesPerSource
	if maxKeys <= 0 {
		maxKeys = defaultMaxKeys
	}
	return retention, maxKeys
}

type nodeState struct {
	mu          sync.Mutex
	cursor      string
	items       map[string]item
	order       []string
	retention   time.Duration
	maxKeys     int
	lastSuccess map[string]time.Time
}

type item struct {
	payload    map[string]any
	receivedAt time.Time
}

// merge keeps the newest delivery per key and the order keys were first seen.
// A healthchecks /start or /log ping reports no outcome, so it keeps the
// stored item's state.
func (n *nodeState) merge(key string, it item) {
	prev, ok := n.items[key]
	if !ok {
		n.order = append(n.order, key)
	}
	if ping, _ := it.payload["ping"].(string); ping != "" && ok {
		if state, has := prev.payload["state"]; has {
			it.payload["state"] = state
		}
	}
	n.recordSuccess(key, it)
	n.items[key] = it
}

// prune drops items whose newest delivery is older than the relay's retention
// and, past maxKeys, the oldest ones, so the set mirrors the relay's window.
func (n *nodeState) prune(now time.Time) {
	if n.retention > 0 {
		cutoff := now.Add(-n.retention)
		for key, it := range n.items {
			if it.receivedAt.Before(cutoff) {
				delete(n.items, key)
			}
		}
	}
	if n.maxKeys > 0 && len(n.items) > n.maxKeys {
		keys := make([]string, 0, len(n.items))
		for key := range n.items {
			keys = append(keys, key)
		}
		slices.SortFunc(keys, func(a, b string) int {
			return n.items[a].receivedAt.Compare(n.items[b].receivedAt)
		})
		for _, key := range keys[:len(keys)-n.maxKeys] {
			delete(n.items, key)
		}
	}
	n.order = slices.DeleteFunc(n.order, func(key string) bool {
		_, ok := n.items[key]
		return !ok
	})
	for key := range n.lastSuccess {
		if _, ok := n.items[key]; !ok {
			delete(n.lastSuccess, key)
		}
	}
}
