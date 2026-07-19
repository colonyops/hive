package pipeline

import (
	"bytes"
	"context"
	"sync"
	"time"

	"github.com/rs/zerolog"
)

// Producer is the poll loop that turns configured sources into event_log
// rows. On each tick it resolves the current sources (via SourceLister),
// drains each source's current items through Produce, and appends every
// emitted Msg to the log. After a tick appends at least one row, onAppended
// fires with the offset of the last row appended, so main.go can wake the
// frontend (the Wails "log:appended" event).
//
// Dedup vs. Compact: a source's Produce re-emits every current item on
// every tick, even when nothing changed upstream (githubSource's fetch
// layer may itself be cache-hit, but the cached items are still emitted).
// Rather than appending an unchanged item every tick and relying solely on
// pipelinedb.Compact to collapse it later, Producer keeps an in-memory
// last-emitted-payload map keyed by topic+key and skips the Append when the
// payload is byte-identical to the last one appended for that key. This is
// a soft, non-persisted optimization (a restart forgets it, so at most one
// extra row per key is appended after a restart) — Compact remains the
// source of truth for bounding on-disk growth.
type Producer struct {
	db         Appender
	sources    SourceLister
	interval   time.Duration
	onAppended func(nextOffset int64)
	logger     zerolog.Logger

	seenMu sync.Mutex
	seen   map[string][]byte // topic+"\x00"+key -> last appended payload

	stopOnce sync.Once
	stop     chan struct{}
}

// NewProducer builds a Producer. interval <= 0 is rejected by the caller's
// choice of default (main.go passes feed.DefaultPollInterval); Producer
// itself has no opinion on the default so this package does not need to
// import feed just for a constant.
func NewProducer(db Appender, sources SourceLister, interval time.Duration, onAppended func(nextOffset int64), logger zerolog.Logger) *Producer {
	return &Producer{
		db:         db,
		sources:    sources,
		interval:   interval,
		onAppended: onAppended,
		logger:     logger,
		seen:       make(map[string][]byte),
		stop:       make(chan struct{}),
	}
}

// Start runs the poll loop in a goroutine until Stop, mirroring
// feed.Poller's Start/Stop lifecycle.
func (pr *Producer) Start() {
	go func() {
		ticker := time.NewTicker(pr.interval)
		defer ticker.Stop()
		for {
			select {
			case <-pr.stop:
				return
			case <-ticker.C:
				pr.Tick(context.Background())
			}
		}
	}()
}

// Stop halts the poll loop. Idempotent, like feed.Poller.Stop.
func (pr *Producer) Stop() {
	pr.stopOnce.Do(func() { close(pr.stop) })
}

// Tick resolves the current sources and drains each one once, appending
// every emitted Msg to the log. It is exported so tests can drive a
// deterministic tick instead of waiting on the ticker. A source whose
// Produce call fails is logged and skipped — one source's fetch failure
// (e.g. an offline stretch) must not block the others.
func (pr *Producer) Tick(ctx context.Context) {
	sources, err := pr.sources(ctx)
	if err != nil {
		pr.logger.Warn().Err(err).Msg("pipeline producer: resolving sources failed")
		return
	}

	var (
		lastOffset int64
		appended   int
	)
	for id, src := range sources {
		err := src.Produce(ctx, func(msg Msg) error {
			offset, ok, err := pr.appendIfChanged(ctx, msg)
			if err != nil {
				return err
			}
			if ok {
				appended++
				lastOffset = offset
			}
			return nil
		})
		if err != nil {
			pr.logger.Debug().Err(err).Str("source", id).Msg("pipeline producer: source fetch failed")
			continue
		}
	}

	if appended > 0 && pr.onAppended != nil {
		pr.onAppended(lastOffset)
	}
}

// appendIfChanged appends msg unless its payload is byte-identical to the
// last payload appended for its topic+key (see the dedup-vs-Compact note on
// Producer). It reports the new offset and whether an append happened.
// Empty-key messages are never deduped: pipelinedb.Compact's key-compaction
// exempts them (they have no identity to compact against), so Producer
// mirrors that and always appends them.
func (pr *Producer) appendIfChanged(ctx context.Context, msg Msg) (int64, bool, error) {
	if msg.Key != "" {
		seenKey := msg.Topic + "\x00" + msg.Key

		pr.seenMu.Lock()
		prev, hadPrev := pr.seen[seenKey]
		unchanged := hadPrev && bytes.Equal(prev, msg.Payload)
		if !unchanged {
			pr.seen[seenKey] = append([]byte(nil), msg.Payload...)
		}
		pr.seenMu.Unlock()

		if unchanged {
			return 0, false, nil
		}
	}

	offset, err := pr.db.Append(ctx, msg.Topic, msg.Key, msg.Payload)
	if err != nil {
		return 0, false, err
	}
	return offset, true, nil
}
