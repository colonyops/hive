package feed

import (
	"context"
	"sync"
	"time"

	"github.com/rs/zerolog"
)

// DefaultPollInterval matches the settings design's default ("Every 60s");
// a settings surface can make it configurable later.
const DefaultPollInterval = time.Minute

// Poller periodically refreshes the distinct sources referenced across every
// profile of a LiveProvider and calls onUpdate with a profile ID whenever a
// source its feeds read from changed. main.go wires onUpdate to the
// feed:updated event.
type Poller struct {
	provider *LiveProvider
	interval time.Duration
	onUpdate func(profileID string)
	logger   zerolog.Logger

	stopOnce sync.Once
	stop     chan struct{}
}

func NewPoller(provider *LiveProvider, interval time.Duration, onUpdate func(profileID string), logger zerolog.Logger) *Poller {
	if interval <= 0 {
		interval = DefaultPollInterval
	}
	return &Poller{
		provider: provider,
		interval: interval,
		onUpdate: onUpdate,
		logger:   logger,
		stop:     make(chan struct{}),
	}
}

// Start runs the poll loop in a goroutine until Stop.
func (p *Poller) Start() {
	go func() {
		ticker := time.NewTicker(p.interval)
		defer ticker.Stop()
		for {
			select {
			case <-p.stop:
				return
			case <-ticker.C:
				p.PollOnce(context.Background())
			}
		}
	}()
}

func (p *Poller) Stop() {
	p.stopOnce.Do(func() { close(p.stop) })
}

// PollOnce refreshes each distinct source exactly once across all profiles —
// profiles sharing sources do not multiply requests — then notifies every
// profile whose feeds reference a changed source.
func (p *Poller) PollOnce(ctx context.Context) {
	defs, err := p.provider.store.Profiles()
	if err != nil {
		p.logger.Warn().Err(err).Msg("poll: reading profiles failed")
		return
	}
	srcByID, err := p.provider.sourcesByID()
	if err != nil {
		p.logger.Warn().Err(err).Msg("poll: reading sources failed")
		return
	}

	allFeeds := make([]FeedDef, 0)
	for _, def := range defs {
		allFeeds = append(allFeeds, def.Feeds...)
	}

	changedKeys := make(map[string]bool)
	for _, src := range distinctSources(allFeeds, srcByID) {
		changed, err := p.provider.refreshSource(ctx, src)
		if err != nil {
			// Expected during offline stretches; the UI keeps stale data.
			p.logger.Debug().Err(err).Str("source", src.ID).Msg("poll: refresh failed")
			continue
		}
		if changed {
			changedKeys[sourceKey(src)] = true
		}
	}
	if len(changedKeys) == 0 || p.onUpdate == nil {
		return
	}

	for _, def := range defs {
		if profileReferencesChanged(def, srcByID, changedKeys) {
			p.onUpdate(def.ID)
		}
	}
}

// profileReferencesChanged reports whether any feed of the profile reads from
// a source whose data changed this poll.
func profileReferencesChanged(def ProfileDef, srcByID map[string]SourceDef, changedKeys map[string]bool) bool {
	for _, src := range distinctSources(def.Feeds, srcByID) {
		if changedKeys[sourceKey(src)] {
			return true
		}
	}
	return false
}
