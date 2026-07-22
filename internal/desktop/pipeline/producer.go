package pipeline

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/colonyops/hive/internal/desktop/activity"
	"github.com/colonyops/hive/internal/desktop/feed"
	"github.com/colonyops/hive/internal/desktop/pipeline/pipelinedb"
	"github.com/rs/zerolog"
)

// Producer is the poll loop that turns configured sources into event_log
// rows. On each tick it resolves the current sources (via SourceLister),
// drains each source's current items through Produce, and appends every
// emitted Msg to the log. After a tick appends at least one row, onAppended
// fires with the offset of the last row appended, so main.go can wake the
// frontend (the Wails "log:appended" event).
//
// Source deduplication: a source's Produce re-emits every current item on
// every tick, even when nothing changed upstream (githubSource's fetch
// layer may itself be cache-hit, but the cached items are still emitted).
// Producer delegates to pipelinedb.AppendIfChanged, which stores the last
// payload by (topic, key) in the database and atomically appends a changed
// event with its new head, so deduplication survives restarts and a failed
// append never suppresses a retry. Successful ticks also append a source
// snapshot event for downstream feed reconciliation.
type Producer struct {
	db         Appender
	sources    SourceLister
	intervalMu sync.Mutex
	interval   time.Duration
	intervalCh chan time.Duration
	onAppended func(nextOffset int64)
	logger     zerolog.Logger
	recorder   activity.Recorder
	prefetcher SearchPrefetcher
	adapters   map[string]SourceAdapter

	stopOnce sync.Once
	stop     chan struct{}
}

// SetRecorder attaches an activity recorder so refresh failures surface in the
// Activity view. Successful periodic refreshes are intentionally omitted to
// avoid flooding the activity log. Set once at wiring time, before Start.
func (pr *Producer) SetRecorder(r activity.Recorder) { pr.recorder = r }

// SearchPrefetcher batch-fetches all search-kind source definitions before
// the producer drains their individual topics.
type SearchPrefetcher interface {
	PrefetchSearch(ctx context.Context, defs []feed.SourceDef) error
}

// SetPrefetcher attaches the optional batch prefetcher. It is set once while
// wiring the producer, before Start.
func (pr *Producer) SetPrefetcher(p SearchPrefetcher) { pr.prefetcher = p }

// SetSourceAdapter registers classification and absence behavior by source kind.
func (pr *Producer) SetSourceAdapter(adapter SourceAdapter) {
	pr.adapters[adapter.SourceKind] = adapter
}

// searchDefSource is implemented by sources backed by a feed.SourceDef that
// want inclusion in the search prefetch. Non-search sources return ok false.
type searchDefSource interface {
	searchDef() (feed.SourceDef, bool)
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
		intervalCh: make(chan time.Duration, 1),
		onAppended: onAppended,
		logger:     logger,
		adapters:   make(map[string]SourceAdapter),
		stop:       make(chan struct{}),
	}
}

// Start runs the poll loop in a goroutine until Stop.
func (pr *Producer) Start() {
	pr.intervalMu.Lock()
	interval := pr.interval
	pr.intervalMu.Unlock()
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-pr.stop:
				return
			case interval := <-pr.intervalCh:
				ticker.Reset(interval)
			case <-ticker.C:
				pr.Tick(context.Background())
			}
		}
	}()
}

// SetInterval changes the poll cadence at runtime. The running ticker resets
// to the new interval, with the next tick occurring one new interval from
// now. Values <= 0 are ignored. It is safe before Start and concurrent with
// the poll loop.
func (pr *Producer) SetInterval(interval time.Duration) {
	if interval <= 0 {
		return
	}
	pr.intervalMu.Lock()
	pr.interval = interval
	select {
	case pr.intervalCh <- interval:
	default:
		// Keep only the latest pending update. The poll loop is the ticker's
		// sole owner, so it performs Reset itself.
		select {
		case <-pr.intervalCh:
		default:
		}
		select {
		case pr.intervalCh <- interval:
		default:
		}
	}
	pr.intervalMu.Unlock()
}

// Stop halts the poll loop. Idempotent.
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

	if pr.prefetcher != nil {
		defs := make([]feed.SourceDef, 0, len(sources))
		for _, src := range sources {
			if searchSource, ok := src.(searchDefSource); ok {
				if def, ok := searchSource.searchDef(); ok {
					defs = append(defs, def)
				}
			}
		}
		if err := pr.prefetcher.PrefetchSearch(ctx, defs); err != nil {
			pr.logger.Debug().Err(err).Msg("pipeline producer: search prefetch failed")
		}
	}

	var (
		lastOffset int64
		appended   int
	)
	for id, src := range sources {
		topic := "source:" + id
		meta := sourceMetadata{ProfileID: id, SourceKind: "generic", Policy: pipelinedb.ResurfacePolicyStateChanges}
		if described, ok := src.(metadataSource); ok {
			meta = described.ingestMetadata()
		}
		if meta.Policy == "" {
			meta.Policy = pipelinedb.ResurfacePolicyStateChanges
		}
		adapter, ok := pr.adapters[meta.SourceKind]
		if !ok {
			adapter = SourceAdapter{SourceKind: meta.SourceKind, Classifier: genericClassifier{}}
		}
		items := make([]pipelinedb.SnapshotItem, 0)
		observed := make(map[string]struct{})
		err := src.Produce(ctx, func(msg Msg) error {
			if msg.Topic != topic {
				return fmt.Errorf("source %q emitted topic %q, expected %q", id, msg.Topic, topic)
			}
			items = append(items, pipelinedb.SnapshotItem{Key: msg.Key, Payload: msg.Payload})
			if msg.Key == "" {
				return nil
			}
			observed[msg.Key] = struct{}{}
			kind := meta.SourceKind
			if msg.SourceKind != "" {
				kind = msg.SourceKind
			}
			result, err := pr.db.IngestObservation(ctx, adapter.Classifier, pipelinedb.IngestObservationParams{ProfileID: meta.ProfileID, Topic: topic, Policy: meta.Policy, Current: observationFromMsg(msg, kind, meta.SourceScope)})
			if err != nil {
				return err
			}
			if result.Wrote {
				appended++
				lastOffset = result.Offset
			}
			return nil
		})
		if err != nil {
			pr.logger.Debug().Err(err).Str("source", id).Msg("pipeline producer: source fetch failed")
			pr.record(ctx, activity.RefreshFailed(id, err.Error()))
			continue
		}

		keys, err := pr.db.ListSourceHeadKeys(ctx, topic)
		if err != nil {
			pr.logger.Debug().Err(err).Str("source", id).Msg("pipeline producer: listing source head failed")
			continue
		}
		if adapter.AbsenceConfirmer != nil {
			for _, key := range keys {
				if _, present := observed[key]; present {
					continue
				}
				payload, err := pr.db.SourceHeadPayload(ctx, topic, key)
				if err != nil {
					pr.logger.Debug().Err(err).Str("source", id).Str("key", key).Msg("pipeline producer: reading source head failed")
					continue
				}
				// source_head persists the source payload, not presentation metadata.
				// Reconstruct the prior observation from that payload so an absence
				// confirmer that starts from prev retains the item's title, URL, and
				// upstream observation time when it returns a hydrated Current.
				prev := observationFromMsg(Msg{Key: key, Payload: payload}, meta.SourceKind, meta.SourceScope)
				verdict, err := adapter.ConfirmAbsence(ctx, prev)
				pipelinedb.DebugPauseIngest(ctx)
				if err != nil {
					pr.logger.Debug().Err(err).Str("source", id).Str("key", key).Msg("pipeline producer: absence hydration failed")
					continue
				}
				if verdict.Current == nil {
					continue
				}
				result, err := pr.db.IngestObservation(ctx, adapter.Classifier, pipelinedb.IngestObservationParams{ProfileID: meta.ProfileID, Topic: topic, Policy: meta.Policy, Current: *verdict.Current})
				if err != nil {
					pr.logger.Debug().Err(err).Str("source", id).Str("key", key).Msg("pipeline producer: absence ingestion failed")
					continue
				}
				if result.Wrote {
					appended++
					lastOffset = result.Offset
				}
			}
		}
		offset, err := pr.db.AppendSnapshot(ctx, topic, meta.SourceKind, meta.SourceScope, items)
		if err != nil {
			pr.logger.Debug().Err(err).Str("source", id).Msg("pipeline producer: appending source snapshot failed")
			pr.record(ctx, activity.RefreshFailed(id, err.Error()))
			continue
		}
		appended++
		lastOffset = offset
	}

	if appended > 0 && pr.onAppended != nil {
		pr.onAppended(lastOffset)
	}
}

// record forwards an activity event when a recorder is attached. Recording is
// best-effort: the recorder itself logs and swallows failures, and a nil
// recorder (no wiring) is a no-op.
func (pr *Producer) record(ctx context.Context, e activity.Event) {
	if pr.recorder != nil {
		pr.recorder.Record(ctx, e)
	}
}

// genericClassifier keeps non-GitHub/test sources ingestible while adapters
// supply richer semantics for real source kinds.
func observationFromMsg(msg Msg, sourceKind, sourceScope string) pipelinedb.Observation {
	var wire struct {
		Title     string `json:"title"`
		URL       string `json:"url"`
		UpdatedAt int64  `json:"updatedAt"`
	}
	_ = json.Unmarshal(msg.Payload, &wire)
	if wire.Title == "" {
		wire.Title = msg.Key
	}
	if wire.UpdatedAt == 0 {
		wire.UpdatedAt = time.Now().UnixMilli()
	}
	return pipelinedb.Observation{ExternalID: msg.Key, Title: wire.Title, URL: wire.URL, SourceKind: sourceKind, SourceScope: sourceScope, ObservedAt: wire.UpdatedAt, Payload: msg.Payload}
}

type genericClassifier struct{}

func (genericClassifier) Classify(previous *pipelinedb.Observation, current pipelinedb.Observation) pipelinedb.Classification {
	if previous == nil {
		return pipelinedb.Classification{Kind: "observed", Transition: pipelinedb.TransitionNone, Attention: pipelinedb.AttentionActivity, Lifecycle: pipelinedb.LifecycleUnknown, Summary: current.Title}
	}
	return pipelinedb.Classification{Kind: "updated", Transition: pipelinedb.TransitionNone, Attention: pipelinedb.AttentionTrivial, Lifecycle: pipelinedb.LifecycleUnknown, Summary: current.Title}
}
