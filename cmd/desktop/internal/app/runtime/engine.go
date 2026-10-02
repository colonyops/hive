package runtime

import (
	"context"
	"fmt"
	"sync"

	"github.com/rs/zerolog"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/colonyops/hive/cmd/desktop/internal/app/activity"
	"github.com/colonyops/hive/cmd/desktop/internal/app/data/models"
	"github.com/colonyops/hive/cmd/desktop/internal/app/data/stores"
	"github.com/colonyops/hive/cmd/desktop/internal/app/events"
	"github.com/colonyops/hive/cmd/desktop/internal/app/flow"
	"github.com/colonyops/hive/cmd/desktop/internal/app/observe"
)

type LogStore interface {
	// ReadForConsumer returns the next page after a consumer's committed
	// offset.
	ReadForConsumer(ctx context.Context, consumer string, limit int) ([]models.Msg, error)
	// TailOffset is the log's high-water mark, the point a replay
	// fast-forwards its consumer to.
	TailOffset(ctx context.Context) (int64, error)
	// ListLatestSnapshots returns each source's newest authoritative snapshot
	// at or before an offset.
	ListLatestSnapshots(ctx context.Context, profileID string, throughOffset int64) ([]models.Msg, error)
}

// InboxReader excludes archived items because replay must preserve their
// frozen memberships.
type InboxReader interface {
	ListUnarchived(ctx context.Context, profileID string) ([]stores.InboxItem, error)
}

type CommitStore interface {
	// Commit applies one run's outputs and advances the offset, atomically.
	Commit(ctx context.Context, batch models.CommitBatch) error
	// ActivateReplay installs a prepared replay: claims, removed structure,
	// node-KV reconciliation, and the consumer checkpoint, in one transaction.
	ActivateReplay(ctx context.Context, profileID string, tail int64, claims []models.FeedClaim, feedIDs, sourceIDs, kvNodeIDs []string) error
}

// Flows is the engine's view of the flow set: whatever loaded successfully,
// which is what it can run.
type Flows interface {
	List() []flow.Flow
}

// DefaultPageSize is how many log records one drain pass reads at a time.
const DefaultPageSize = 500

// EngineOptions configure an Engine. Only Logger and PageSize are optional.
type EngineOptions struct {
	Log     LogStore
	Items   InboxReader
	Commits CommitStore
	KV      KVReader
	Flows   Flows
	Scripts *ScriptRegistry
	Logger  zerolog.Logger
	// PageSize bounds one read. Zero means DefaultPageSize.
	PageSize int
	// Events receives InboxUpdated after a pass commits at least one flow.
	// Appending an unrouted log message does not emit this event.
	Events *events.Bus
	// Recorder reports install failures while the engine keeps the last known-good
	// flow active.
	Recorder activity.Recorder
}

// Engine runs every enabled flow against the event log. One per process: the
// producer and the output worker are singletons for the same reason, and a
// second engine would double-execute every flow.
//
// It is level-triggered. Wake and Reload set a latch rather than queueing, so
// a signal that arrives while a pass is already running is serviced by the
// next pass instead of being dropped or piling up. That is the property the
// browser runtime had to reconstruct by hand, and losing it means a message
// sits unrouted until something else happens to wake the engine.
type Engine struct {
	opts EngineOptions

	// runners holds the installed runner per flow id. A flow is only ever
	// replaced by a runner that was built and replayed successfully, so a
	// broken edit leaves the last-known-good one running.
	runners map[string]*Runner

	wake   chan struct{}
	reload chan struct{}

	cancel   context.CancelFunc
	stopped  chan struct{}
	stopOnce sync.Once
}

// NewEngine builds the engine. Nothing runs until Start.
func NewEngine(opts EngineOptions) *Engine {
	if opts.PageSize <= 0 {
		opts.PageSize = DefaultPageSize
	}
	return &Engine{
		opts:    opts,
		runners: map[string]*Runner{},
		wake:    make(chan struct{}, 1),
		reload:  make(chan struct{}, 1),
		stopped: make(chan struct{}),
	}
}

// Start installs every enabled flow and then runs the drain loop.
//
// Installation is synchronous, before Start returns. Everything that can
// append to the log — the producer, the webhook listener, the e2e harness —
// is started after this, so there is no window in which an append can arrive
// before the engine is able to route it. The browser runtime had exactly that
// window, and closing it needed a readiness latch the frontend published for
// tests to gate on.
func (e *Engine) Start(ctx context.Context) error {
	runCtx, cancel := context.WithCancel(ctx)
	e.cancel = cancel

	e.install(runCtx)

	go func() {
		defer close(e.stopped)
		e.loop(runCtx)
	}()
	return nil
}

// Stop ends the drain loop and releases every runner. An in-flight page
// finishes; no next page starts.
func (e *Engine) Stop() {
	e.stopOnce.Do(func() {
		if e.cancel == nil {
			// Never started. Close is the single teardown path and runs even
			// when Start did not, so this is a normal call, not a misuse.
			return
		}
		e.cancel()
		<-e.stopped
		for id, runner := range e.runners {
			runner.Close()
			delete(e.runners, id)
		}
	})
}

// Wake requests a drain. It never blocks: the request is a latch, so a wake
// arriving mid-pass is serviced by the next one.
func (e *Engine) Wake() {
	select {
	case e.wake <- struct{}{}:
	default:
	}
}

// Reload requests that the flow set be re-read and every runner reinstalled.
// It implies a drain.
func (e *Engine) Reload() {
	select {
	case e.reload <- struct{}{}:
	default:
	}
}

// loop is the engine's only goroutine, which is what makes the runners safe to
// hold without a lock: nothing else touches them.
func (e *Engine) loop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-e.reload:
			e.install(ctx)
			e.drain(ctx, true)
		case <-e.wake:
			e.drain(ctx, false)
		}
	}
}

// install reconciles the installed runners against the current flow set:
// enabled flows get a runner, everything else loses one.
func (e *Engine) install(ctx context.Context) {
	enabled := map[string]flow.Flow{}
	for _, f := range e.opts.Flows.List() {
		if f.Enabled {
			enabled[f.ID] = f
		}
	}

	for id, runner := range e.runners {
		if _, keep := enabled[id]; !keep {
			runner.Close()
			delete(e.runners, id)
		}
	}

	for id, f := range enabled {
		if err := e.installFlow(ctx, f); err != nil {
			// The previous runner keeps running. A flow that cannot be built is
			// an authoring error, and taking a working flow offline for it would
			// lose messages that the last-known-good version handles fine.
			e.opts.Logger.Warn().Err(err).Str("flow", id).Msg("flow could not be installed; keeping the last known-good runtime")
			if e.opts.Recorder != nil {
				e.opts.Recorder.Record(ctx, activity.FlowRuntimeFailed(id, err))
			}
		}
	}
}

// installFlow builds a flow's runner and runs the replay protocol before
// putting it in service.
//
// Replay exists because a flow's structure can change while its inbox does
// not. Recomputing membership from each source's latest snapshot, then
// fast-forwarding the consumer past everything already in the log, is what
// makes a deploy change what a feed contains without re-running the actions
// that were already run for those items. The recompute is a plain Run whose
// result is not committed — only its feed claims are installed.
func (e *Engine) installFlow(ctx context.Context, f flow.Flow) error {
	runner, err := NewRunner(f, Options{Scripts: e.opts.Scripts, KV: e.opts.KV})
	if err != nil {
		return err
	}

	if err := e.replay(ctx, f, runner); err != nil {
		runner.Close()
		return err
	}

	if previous := e.runners[f.ID]; previous != nil {
		previous.Close()
	}
	e.runners[f.ID] = runner
	return nil
}

// replay prepares and installs one flow's membership, then advances its
// consumer to the captured log tail.
func (e *Engine) replay(ctx context.Context, f flow.Flow, runner *Runner) error {
	tail, err := e.opts.Log.TailOffset(ctx)
	if err != nil {
		return fmt.Errorf("reading the event log tail: %w", err)
	}

	feedIDs, sourceIDs := flowTargets(f)

	items, err := e.opts.Items.ListUnarchived(ctx, f.ID)
	if err != nil {
		return fmt.Errorf("reading claimable inbox items: %w", err)
	}
	byIdentity := make(map[string]int64, len(items))
	for _, item := range items {
		byIdentity[identityKey(item.SourceKind, item.SourceScope, item.ExternalID)] = item.ID
	}

	snapshots, err := e.opts.Log.ListLatestSnapshots(ctx, f.ID, tail)
	if err != nil {
		return fmt.Errorf("reading source snapshots: %w", err)
	}
	current := make(map[string]bool, len(sourceIDs))
	for _, id := range sourceIDs {
		current[id] = true
	}
	// A snapshot from a source the flow no longer has must not resurrect its
	// items: the claims it would produce belong to structure that is gone.
	kept := snapshots[:0]
	for _, snapshot := range snapshots {
		if current[snapshot.Topic] {
			kept = append(kept, snapshot)
		}
	}

	result, err := runner.RunReplay(ctx, kept)
	if err != nil {
		return fmt.Errorf("recomputing membership: %w", err)
	}

	claims := make([]models.FeedClaim, 0, len(result.Outputs))
	for _, output := range result.Outputs {
		if output.Sink.Kind != models.SinkKindFeed {
			continue
		}
		itemID, ok := byIdentity[identityKey(output.SourceKind, output.SourceScope, output.Key)]
		if !ok {
			// The item was archived or purged since the snapshot was taken.
			// Archived membership is frozen on purpose, so there is nothing to
			// claim.
			continue
		}
		claims = append(claims, models.FeedClaim{
			ProfileID: f.ID,
			FeedID:    output.Sink.TargetID,
			ItemID:    itemID,
			SourceID:  output.SourceTopic,
		})
	}

	// One transaction installs the claims, removes the structure this flow no
	// longer has, reconciles node KV, and moves the checkpoint. A failure here
	// leaves the previous flow's claims, KV and offset exactly as they were.
	if err := e.opts.Commits.ActivateReplay(ctx, f.ID, tail, claims, feedIDs, sourceIDs, flowKVNodeIDs(f)); err != nil {
		return fmt.Errorf("activating replay: %w", err)
	}
	return nil
}

// drain reads and commits pages for every installed flow until each has no
// more work.
//
// A trigger, so a root span: it is the parent the store's conditional
// db.CommitBatch span fires under. A failed flow is the question its own span
// answers; the pass only counts it, as a tick counts a failed source.
func (e *Engine) drain(ctx context.Context, reload bool) {
	ctx, span := tracer.Start(ctx, "runtime.drain", trace.WithAttributes(attribute.Bool(attrReload, reload)))
	defer span.End()

	var pages, messages, failed int
	for id, runner := range e.runners {
		p, m, err := e.drainFlow(ctx, id, runner)
		pages += p
		messages += m
		if err != nil {
			failed++
		}
	}
	span.SetAttributes(
		attribute.Int(attrFlows, len(e.runners)),
		attribute.Int(attrPages, pages),
		attribute.Int(attrMessages, messages),
		attribute.Int(attrFailed, failed),
	)

	if ctx.Err() != nil {
		return
	}
	if pages > 0 && e.opts.Events != nil {
		e.opts.Events.Publish(ctx, events.InboxUpdated{})
	}
}

// drainFlow pumps one flow until its log is exhausted or a page fails. It
// reports the pages it committed and the messages they held.
func (e *Engine) drainFlow(ctx context.Context, id string, runner *Runner) (pages, messages int, err error) {
	ctx, span := tracer.Start(ctx, "runtime.flow", trace.WithAttributes(attribute.String(attrFlowID, id)))
	defer func() {
		if err != nil {
			observe.RecordError(span, err)
		}
		span.SetAttributes(attribute.Int(attrPages, pages), attribute.Int(attrMessages, messages))
		span.End()
	}()

	for ctx.Err() == nil {
		var n int
		n, err = e.pump(ctx, id, runner)
		if err != nil {
			e.opts.Logger.Warn().Ctx(ctx).Err(err).Str("flow", id).Msg("flow run failed; it will retry on the next wake-up")
			return pages, messages, err
		}
		if n == 0 {
			break
		}
		pages++
		messages += n
	}
	return pages, messages, nil
}

// pump reads one page for a flow, runs it, and commits. It reports how many
// messages the page held; zero means there was nothing to do.
func (e *Engine) pump(ctx context.Context, id string, runner *Runner) (int, error) {
	batch, err := e.opts.Log.ReadForConsumer(ctx, id, e.opts.PageSize)
	if err != nil {
		return 0, fmt.Errorf("reading the log: %w", err)
	}
	if len(batch) == 0 {
		return 0, nil
	}

	result, err := runner.Run(ctx, batch)
	if err != nil {
		return 0, fmt.Errorf("running the flow: %w", err)
	}
	if err := e.opts.Commits.Commit(ctx, result); err != nil {
		return 0, fmt.Errorf("committing: %w", err)
	}
	return len(batch), nil
}

// flowTargets lists a flow's feed ids and the source topics it currently
// ingests, both flow-qualified. Disabled source nodes are excluded: a source
// switched off should have its claims removed, not preserved.
func flowTargets(f flow.Flow) (feedIDs, sourceIDs []string) {
	for i := range f.Nodes {
		node := &f.Nodes[i]
		switch {
		case behaviors[node.Type].snapshotReconciled:
			feedIDs = append(feedIDs, f.ID+"/"+node.ID)
		case behaviors[node.Type].relay && !node.Disabled:
			sourceIDs = append(sourceIDs, "source:"+f.ID+"/"+node.ID)
		}
	}
	return feedIDs, sourceIDs
}

// flowKVNodeIDs lists the ids of the flow's KV-capable nodes, from the same
// declared behavior flags flowTargets reads. Converting a node to a type
// without the capability under the same id drops it from this set, so its
// now-inaccessible KV is reconciled away rather than retained forever.
func flowKVNodeIDs(f flow.Flow) []string {
	var ids []string
	for i := range f.Nodes {
		if behaviors[f.Nodes[i].Type].kvCapable {
			ids = append(ids, f.Nodes[i].ID)
		}
	}
	return ids
}

// identityKey is the source-identity triple an inbox item is found by. The
// separator is a NUL so no component can forge another's boundary.
func identityKey(sourceKind, sourceScope, externalID string) string {
	return sourceKind + "\x00" + sourceScope + "\x00" + externalID
}
