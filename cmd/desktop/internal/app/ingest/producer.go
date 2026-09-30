package ingest

import (
	"context"
	"fmt"
	"maps"
	"sync"
	"time"

	"github.com/hay-kot/appkit/concurrency"
	"github.com/rs/zerolog"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/colonyops/hive/cmd/desktop/internal/app/activity"
	"github.com/colonyops/hive/cmd/desktop/internal/app/data/models"
	"github.com/colonyops/hive/cmd/desktop/internal/app/data/stores"
	"github.com/colonyops/hive/cmd/desktop/internal/app/observe"
	"github.com/colonyops/hive/cmd/desktop/internal/app/sources/connector"
)

// Producer is the poll loop that turns configured source connectors into
// event_log rows. On each tick it resolves the current pull-mode instances,
// drains each one through Produce, and appends every emitted Msg to the log.
//
// There is one ticker for every source, at settings.polling.interval. An
// instance that wants to run less often than that declares a MinInterval and
// the tick skips it until it is due — a floor quantized to the tick, not a
// second schedule. Not drained is not the same as drained empty: Produce is
// never called, so nothing about the source's tracked set changes.
//
// Connectors may re-emit unchanged items. IngestObservation atomically
// deduplicates against durable source heads, so dedup survives restarts and
// failed appends remain retryable. Successful ticks append an authoritative
// snapshot for feed reconciliation.
//
// Nothing here branches on which connector it is holding. What a source
// supports beyond producing messages — its classifier, its absence confirmer,
// its batched prefetch — arrives already wired on the instance, because a
// capability discovered by type assertion fails silently as generic
// ingestion, and generic ingestion of a GitHub item is wrong rather than
// merely plain.
type Producer struct {
	ingester    Ingester
	snapshots   SnapshotAppender
	heads       SourceHeads
	sources     Sources
	intervalMu  sync.Mutex
	interval    time.Duration
	intervalCh  chan time.Duration
	notifier    LogAppendNotifier
	logger      zerolog.Logger
	recorder    activity.Recorder
	pauseIngest time.Duration
	now         func() time.Time

	// scheduleMu guards the per-source state a tick keeps between ticks. A
	// manual refresh runs a tick on the caller's goroutine while the loop may
	// be running one of its own.
	scheduleMu  sync.Mutex
	lastRun     map[string]time.Time
	lastFailure map[string]time.Time
	lastTick    time.Time

	// writeSlot holds concurrent drains to one write at a time. SQLite admits
	// one writer anyway; racing for it only parks the losers in the busy
	// handler, each holding one of the two pooled connections the UI reads
	// through. It is a channel rather than a sync.Mutex because synctest
	// counts a goroutine waiting on a channel as durably blocked, and not one
	// waiting on a mutex.
	writeSlot chan struct{}

	stopOnce sync.Once
	stop     chan struct{}
}

// SetRecorder attaches an activity recorder so refresh failures surface in the
// Activity view. Successful periodic refreshes are intentionally omitted to
// avoid flooding the activity log. Set once at wiring time, before Start.
func (pr *Producer) SetRecorder(r activity.Recorder) { pr.recorder = r }

// SetDebugPause injects the development-only post-hydration pause.
func (pr *Producer) SetDebugPause(duration time.Duration) { pr.pauseIngest = duration }

// LogAppendNotifier must wake the flow engine synchronously. A bus event is
// insufficient because subscribers coalesce bursts and could delay routing.
type LogAppendNotifier interface {
	PublishLogAppended(nextOffset int64)
}

type ProducerDeps struct {
	Ingester  Ingester
	Snapshots SnapshotAppender
	Heads     SourceHeads
	Sources   Sources
	Interval  time.Duration
	Notifier  LogAppendNotifier
	Logger    zerolog.Logger
}

// NewProducer requires a positive Interval.
func NewProducer(d ProducerDeps) *Producer {
	return &Producer{
		ingester:    d.Ingester,
		snapshots:   d.Snapshots,
		heads:       d.Heads,
		sources:     d.Sources,
		interval:    d.Interval,
		intervalCh:  make(chan time.Duration, 1),
		notifier:    d.Notifier,
		logger:      d.Logger,
		now:         time.Now,
		lastRun:     map[string]time.Time{},
		lastFailure: map[string]time.Time{},
		writeSlot:   make(chan struct{}, 1),
		stop:        make(chan struct{}),
	}
}

// Start runs the poll loop in a goroutine until Stop.
func (pr *Producer) Start(ctx context.Context) {
	pr.intervalMu.Lock()
	interval := pr.interval
	pr.intervalMu.Unlock()
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-pr.stop:
				return
			case interval := <-pr.intervalCh:
				ticker.Reset(interval)
			case <-ticker.C:
				pr.Tick(ctx)
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

// TickSummary reports what one tick did, so a caller that forced the tick (a
// manual refresh) can tell "nothing changed" from "every source failed".
type TickSummary struct {
	Sources  int
	Appended int
	Failed   int
}

// Tick resolves the current sources and drains each one whose own cadence is
// due, appending every emitted Msg to the log. It is exported so tests can
// drive a deterministic tick instead of waiting on the ticker. A source whose
// Produce call fails is logged and skipped — one source's fetch failure
// (e.g. an offline stretch) must not block the others.
func (pr *Producer) Tick(ctx context.Context) TickSummary { return pr.tick(ctx, false) }

// Refresh drains every source now, ignoring the per-instance cadence floors: a
// user who asked for a refresh gets one, including from the source that only
// wanted to run hourly.
func (pr *Producer) Refresh(ctx context.Context) TickSummary { return pr.tick(ctx, true) }

// drainConcurrency is capped because GitHub penalizes concurrent requests on
// one token.
const drainConcurrency = 4

func (pr *Producer) tick(ctx context.Context, forced bool) TickSummary {
	// A trigger, so a root span: everything below hangs off it, which is what
	// makes an otherwise orphan client span readable.
	ctx, span := tracer.Start(ctx, "ingest.tick", trace.WithAttributes(attribute.Bool(attrForced, forced)))
	defer span.End()

	instances := pr.sources.PullInstances(ctx)

	pr.prefetch(ctx, instances)

	pr.pruneSchedule(instances)

	due := make([]connector.Instance, 0, len(instances))
	for _, instance := range instances {
		if forced || pr.claimRun(instance) {
			due = append(due, instance)
		}
	}

	results := make([]drained, len(due))
	completed := make([]bool, len(due))
	err := concurrency.ForEach(ctx, len(due), drainConcurrency, func(ctx context.Context, i int) error {
		rows, err := pr.drain(ctx, due[i])
		results[i], completed[i] = rows, err == nil
		// Never returned: ForEach cancels every in-flight drain on the first
		// error, and one source's failure must not cost the others their tick.
		return nil
	})
	if err != nil {
		observe.RecordError(span, err)
	}

	summary := TickSummary{Sources: len(instances)}
	var lastOffset int64
	for i, rows := range results {
		// Includes a source that cancellation skipped before it started.
		if !completed[i] {
			summary.Failed++
			continue
		}
		summary.Appended += rows.appended
		lastOffset = max(lastOffset, rows.lastOffset)
	}

	span.SetAttributes(
		attribute.Int(attrSources, summary.Sources),
		attribute.Int(attrDrained, len(due)),
		attribute.Int(attrFailed, summary.Failed),
		attribute.Int(attrAppended, summary.Appended),
	)

	pr.scheduleMu.Lock()
	pr.lastTick = pr.now()
	pr.scheduleMu.Unlock()

	if summary.Appended > 0 && pr.notifier != nil {
		pr.notifier.PublishLogAppended(lastOffset)
	}
	return summary
}

// LastTick advances after every completed tick, even when it finds nothing.
// It is zero before the first tick.
func (pr *Producer) LastTick() time.Time {
	pr.scheduleMu.Lock()
	defer pr.scheduleMu.Unlock()
	return pr.lastTick
}

// A kind is overridable per message and so can be absent; a trailing space in
// a search key helps nobody.
func sourceSpanName(kind string) string {
	if kind == "" {
		return "ingest.source"
	}
	return "ingest.source " + kind
}

// One batched round trip per tick. It has a span because without one its time
// lands under ingest.tick as a bare HTTP call, and it can be most of the tick.
func (pr *Producer) prefetch(ctx context.Context, instances []connector.Instance) {
	ctx, span := tracer.Start(ctx, "ingest.prefetch", trace.WithAttributes(
		attribute.Int(attrSources, len(instances)),
	))
	defer span.End()

	if err := pr.sources.Prefetch(ctx, instances); err != nil {
		observe.RecordError(span, err)
		pr.logger.Debug().Ctx(ctx).Err(err).Msg("pipeline producer: source prefetch failed")
	}
}

// pruneSchedule drops the per-source state of sources that are no longer
// configured. Both maps are keyed by a flow-qualified node id, which the user
// renames freely, so without this an edited flow leaks an entry per rename for
// the life of the process. It also means a node that is deleted and recreated
// starts clean, which is what re-adding a source should do.
func (pr *Producer) pruneSchedule(instances []connector.Instance) {
	current := make(map[string]struct{}, len(instances))
	for _, instance := range instances {
		current[instance.Node.ID()] = struct{}{}
	}

	stale := func(id string, _ time.Time) bool {
		_, present := current[id]
		return !present
	}

	pr.scheduleMu.Lock()
	defer pr.scheduleMu.Unlock()
	maps.DeleteFunc(pr.lastRun, stale)
	maps.DeleteFunc(pr.lastFailure, stale)
}

// claimRun reports whether instance is due, recording the attempt when it is.
//
// The floor rate-limits *running* the source, not succeeding at it: a failed
// run still claims its slot, so a command asking to run hourly is not retried
// every tick because it is broken. Nothing here persists — a restart runs every
// source once, which is the behaviour a user expects from launching the app.
func (pr *Producer) claimRun(instance connector.Instance) bool {
	if instance.MinInterval <= 0 {
		return true
	}
	id := instance.Node.ID()
	now := pr.now()

	pr.scheduleMu.Lock()
	defer pr.scheduleMu.Unlock()
	if last, ok := pr.lastRun[id]; ok && now.Sub(last) < instance.MinInterval {
		return false
	}
	pr.lastRun[id] = now
	return true
}

// drained is what one source's tick was worth: how many rows it appended and
// the offset of the last one.
type drained struct {
	appended   int
	lastOffset int64
}

// drain runs one source's Produce, confirms whatever left its snapshot, and
// appends the topic's authoritative snapshot. The returned error means the
// source did not complete — its snapshot is not authoritative, so neither
// absence confirmation nor the snapshot append may run.
func (pr *Producer) drain(ctx context.Context, instance connector.Instance) (out drained, err error) {
	id := instance.Node.ID()
	topic := instance.Node.Topic()
	meta := instance.Metadata
	if meta.Policy == "" {
		meta.Policy = models.ResurfacePolicyStateChanges
	}

	// Named by kind, which is bounded; the id rides as an attribute.
	ctx, span := tracer.Start(ctx, sourceSpanName(meta.SourceKind), trace.WithAttributes(
		attribute.String(attrSourceID, id),
		attribute.String(attrSourceKnd, meta.SourceKind),
		attribute.String(attrTopic, topic),
	))
	// A failed source is the question this span answers; the tick only counts it.
	defer func() {
		if err != nil {
			observe.RecordError(span, err)
		}
		span.SetAttributes(attribute.Int(attrAppended, out.appended))
		span.End()
	}()
	// A connector that declared no classifier gets the generic one, which
	// records that something was observed or updated and nothing more.
	classifier := instance.Classifier
	if classifier == nil {
		classifier = genericClassifier{}
	}

	items := make([]models.SnapshotItem, 0)
	observed := make(map[string]struct{})
	err = instance.Pull.Produce(ctx, func(msg Msg) error {
		if msg.Topic != topic {
			return fmt.Errorf("source %q emitted topic %q, expected %q", id, msg.Topic, topic)
		}
		items = append(items, models.SnapshotItem{Key: msg.Key, Payload: msg.Payload})
		if msg.Key == "" {
			return nil
		}
		observed[msg.Key] = struct{}{}
		kind := meta.SourceKind
		if msg.SourceKind != "" {
			kind = msg.SourceKind
		}
		pr.writeSlot <- struct{}{}
		result, err := pr.ingester.IngestObservation(ctx, classifier, stores.IngestObservationParams{ProfileID: meta.ProfileID, Topic: topic, Policy: meta.Policy, Current: observationFromMsg(msg, kind, meta.SourceScope)})
		<-pr.writeSlot
		if err != nil {
			return err
		}
		if result.Wrote {
			out.appended++
			out.lastOffset = result.Offset
		}
		return nil
	})
	if err != nil {
		pr.logger.Debug().Ctx(ctx).Err(err).Str("source", id).Msg("pipeline producer: source fetch failed")
		pr.recordFailure(ctx, id, err)
		return out, err
	}

	if instance.Absence != nil {
		pr.confirmAbsent(ctx, instance, meta, classifier, observed, &out)
	}

	pr.writeSlot <- struct{}{}
	offset, err := pr.snapshots.AppendSnapshot(ctx, topic, meta.SourceKind, meta.SourceScope, items)
	<-pr.writeSlot
	if err != nil {
		pr.logger.Debug().Ctx(ctx).Err(err).Str("source", id).Msg("pipeline producer: appending source snapshot failed")
		pr.recordFailure(ctx, id, err)
		return out, err
	}
	pr.clearFailure(id)
	out.appended++
	out.lastOffset = offset
	return out, nil
}

// confirmAbsent asks the connector what happened to each item that was in the
// source head but not in this tick's snapshot. Only connectors that declared
// CapConfirmAbsence get here; for the rest an item that stops appearing is
// left to the resurface policy.
func (pr *Producer) confirmAbsent(ctx context.Context, instance connector.Instance, meta connector.Metadata, classifier models.Classifier, observed map[string]struct{}, out *drained) {
	id := instance.Node.ID()
	topic := instance.Node.Topic()

	keys, err := pr.heads.ListActiveKeys(ctx, stores.SourceIdentity{Topic: topic, ProfileID: meta.ProfileID, SourceKind: meta.SourceKind, SourceScope: meta.SourceScope})
	if err != nil {
		pr.logger.Debug().Ctx(ctx).Err(err).Str("source", id).Msg("pipeline producer: listing source head failed")
		return
	}

	prevs := make([]models.Observation, 0, len(keys))
	for _, key := range keys {
		if _, present := observed[key]; present {
			continue
		}
		payload, err := pr.heads.Payload(ctx, topic, key)
		if err != nil {
			pr.logger.Debug().Ctx(ctx).Err(err).Str("source", id).Str("key", key).Msg("pipeline producer: reading source head failed")
			continue
		}
		// source_head persists the source payload, not presentation metadata.
		// Reconstruct the prior observation from that payload so an absence
		// confirmer that starts from prev retains the item's title, URL, and
		// upstream observation time when it returns a hydrated Current.
		prevs = append(prevs, observationFromMsg(Msg{Key: key, Payload: payload}, meta.SourceKind, meta.SourceScope))
	}
	if len(prevs) == 0 {
		return
	}

	verdicts, err := instance.Absence.ConfirmAbsence(ctx, prevs)
	debugPause(ctx, pr.pauseIngest)
	if err != nil {
		// A partial failure still resolves some verdicts; those are ingested
		// below rather than discarded.
		pr.logger.Debug().Ctx(ctx).Err(err).Str("source", id).Msg("pipeline producer: absence confirmation failed")
	}
	for _, prev := range prevs {
		v, ok := verdicts[prev.ExternalID]
		if !ok || v.Current == nil {
			continue
		}
		pr.writeSlot <- struct{}{}
		result, err := pr.ingester.IngestObservation(ctx, classifier, stores.IngestObservationParams{ProfileID: meta.ProfileID, Topic: topic, Policy: meta.Policy, Current: *v.Current})
		<-pr.writeSlot
		if err != nil {
			pr.logger.Debug().Ctx(ctx).Err(err).Str("source", id).Str("key", prev.ExternalID).Msg("pipeline producer: absence ingestion failed")
			continue
		}
		if result.Wrote {
			out.appended++
			out.lastOffset = result.Offset
		}
		// Evict after IngestObservation, regardless of Wrote: the dedup
		// short-circuit still leaves the head row in place, and deleting
		// before the ingest would be undone by its UpsertSourceHead.
		if v.Terminal {
			pr.writeSlot <- struct{}{}
			err := pr.heads.Delete(ctx, topic, prev.ExternalID)
			<-pr.writeSlot
			if err != nil {
				pr.logger.Debug().Ctx(ctx).Err(err).Str("source", id).Str("key", prev.ExternalID).Msg("pipeline producer: evicting source head failed")
			}
		}
	}
}

func debugPause(ctx context.Context, duration time.Duration) {
	if duration <= 0 {
		return
	}
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
	case <-timer.C:
	}
}

// failureNoticeInterval is how often one source's continuing failure is
// re-announced in Activity. A source that stays broken is one condition, not
// one per tick: at the 60s floor, recording every failure would bury a day of
// real events under 1440 copies of the same line, which is how an audit log
// stops being read.
const failureNoticeInterval = time.Hour

// recordFailure announces a source's failure, at most once per
// failureNoticeInterval until it succeeds again. The first failure after a
// success always records — a transition is news; a continuation is not.
//
// Suppression is by source rather than by message: a failure reason routinely
// carries a timestamp or a request id from the tool that produced it, so
// comparing reasons would defeat itself on exactly the persistently broken
// source this exists for.
func (pr *Producer) recordFailure(ctx context.Context, id string, cause error) {
	now := pr.now()

	pr.scheduleMu.Lock()
	last, announced := pr.lastFailure[id]
	if announced && now.Sub(last) < failureNoticeInterval {
		pr.scheduleMu.Unlock()
		return
	}
	pr.lastFailure[id] = now
	pr.scheduleMu.Unlock()

	pr.record(ctx, activity.RefreshFailed(id, cause.Error()))
}

// clearFailure re-arms the notice for a source that completed a tick, so the
// next failure is recorded immediately rather than waiting out the interval.
func (pr *Producer) clearFailure(id string) {
	pr.scheduleMu.Lock()
	defer pr.scheduleMu.Unlock()
	delete(pr.lastFailure, id)
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
func observationFromMsg(msg Msg, sourceKind, sourceScope string) models.Observation {
	title, url, updatedAt := models.FeedFields(msg.Payload)
	if title == "" {
		title = msg.Key
	}
	if updatedAt == 0 {
		updatedAt = time.Now().UnixMilli()
	}
	return models.Observation{ExternalID: msg.Key, Title: title, URL: url, SourceKind: sourceKind, SourceScope: sourceScope, ObservedAt: updatedAt, Payload: msg.Payload}
}

type genericClassifier struct{}

func (genericClassifier) Classify(previous *models.Observation, current models.Observation) models.Classification {
	if previous == nil {
		return models.Classification{Kind: "observed", Transition: models.TransitionNone, Attention: models.AttentionActivity, Lifecycle: models.LifecycleUnknown, Summary: current.Title}
	}
	return models.Classification{Kind: "updated", Transition: models.TransitionNone, Attention: models.AttentionTrivial, Lifecycle: models.LifecycleUnknown, Summary: current.Title}
}
