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

// Poller periodically refreshes every profile of a LiveProvider and calls
// onUpdate with a profile ID whenever its feed state changed. main.go wires
// onUpdate to the feed:updated event.
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

// PollOnce refreshes every profile once, notifying per changed profile.
func (p *Poller) PollOnce(ctx context.Context) {
	defs, err := p.provider.store.Profiles()
	if err != nil {
		p.logger.Warn().Err(err).Msg("poll: reading profiles failed")
		return
	}
	for _, def := range defs {
		changed, err := p.provider.Refresh(ctx, def.ID)
		if err != nil {
			// Expected during offline stretches; the UI keeps stale data.
			p.logger.Debug().Err(err).Str("profile", def.Name).Msg("poll: refresh failed")
			continue
		}
		if changed && p.onUpdate != nil {
			p.onUpdate(def.ID)
		}
	}
}
