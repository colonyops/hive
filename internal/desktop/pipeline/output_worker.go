package pipeline

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/rs/zerolog"

	"github.com/colonyops/hive/internal/desktop/pipeline/actions"
	"github.com/colonyops/hive/internal/desktop/pipeline/pipelinedb"
)

// DefaultOutputWorkerInterval is how often the Worker drains pending
// output_command rows. Unlike Producer's poll interval (bounded by GitHub
// rate limits), this only touches the local pipeline DB and the configured
// executors, so it ticks much faster.
const DefaultOutputWorkerInterval = 5 * time.Second

// DefaultOutputWorkerBatch bounds automatic command executions per tick, so
// one very deep queue can't starve the ticker loop indefinitely.
const DefaultOutputWorkerBatch = 50

// outputWorkerScanMultiplier bounds how many runnable rows a tick inspects
// while classifying manual work. It lets one batch of manual rows be moved out
// of the way while still leaving room to execute a full automatic batch.
const outputWorkerScanMultiplier = 2

// MaxOutputCommandAttempts bounds how many failed executions the worker
// retries before giving up and marking a command "failed" for good.
const MaxOutputCommandAttempts = 5

// OutputData is the template/execution context for one output_command
// dispatch: the triggering msg's parsed JSON payload (for template field
// access, e.g. "{{ .Payload.title }}") plus its dedup key. Raw carries the
// exact bytes originally enqueued, for executors (publish-event) that want
// to pass the payload through unchanged rather than re-marshal a
// round-tripped map.
type OutputData struct {
	Key     string
	Payload map[string]any
	Raw     json.RawMessage
}

// Executor executes one action invocation with its rendered template data.
type Executor interface {
	Execute(ctx context.Context, action actions.Action, data OutputData) error
}

// Dispatcher routes an action invocation to the Executor registered for its
// Type. It is not itself an Executor to keep the "no executor for this
// type" failure mode explicit at construction call sites (NewDispatcher),
// rather than silently swallowed behind the Executor interface.
type Dispatcher struct {
	executors map[string]Executor
}

// NewDispatcher builds a Dispatcher over executors, keyed by action type
// ("launch-session", "shell", "publish-event").
func NewDispatcher(executors map[string]Executor) *Dispatcher {
	return &Dispatcher{executors: executors}
}

// Execute dispatches to the executor registered for action.Type. A type
// with no registered executor is a configuration gap (e.g. actions.yml was
// edited to use a type this build doesn't wire up) — it surfaces as an
// execution failure, retried/failed like any other, rather than a panic.
func (d *Dispatcher) Execute(ctx context.Context, action actions.Action, data OutputData) error {
	ex, ok := d.executors[action.Type]
	if !ok {
		return fmt.Errorf("dispatcher: no executor registered for action type %q", action.Type)
	}
	return ex.Execute(ctx, action, data)
}

// ActionLister resolves actions. *actions.ActionStore satisfies this; the
// narrower interface lets tests inject a fake action set without touching
// disk. List is used to promote confirmation-gated commands when an action
// changes from manual to auto-apply.
type ActionLister interface {
	Get(id string) (actions.Action, bool)
	List() []actions.Action
}

// OutputCommandStore is the subset of *pipelinedb.DB the Worker needs.
// Tests can substitute a fake, though most still use a real pipelinedb.DB
// via t.TempDir(), matching Producer's own test posture (see Appender).
type OutputCommandStore interface {
	ListRunnableOutputCommandsAfter(ctx context.Context, afterID int64, limit int) ([]pipelinedb.OutputCommand, error)
	MarkOutputCommandAwaitingConfirmation(ctx context.Context, id int64) error
	PromoteOutputCommandsAwaitingConfirmation(ctx context.Context, actionID string) error
	MarkOutputCommandDone(ctx context.Context, id int64) error
	MarkOutputCommandFailed(ctx context.Context, id int64, lastErr string) error
	RetryOutputCommand(ctx context.Context, id int64, lastErr string) error
}

// Worker is the output side of the desktop pipeline: it drains runnable
// output_command rows enqueued by CommitBatch (see pipelinedb.CommitBatch)
// and executes each one against its Action definition. Commands for manual
// actions move to "awaiting_confirmation", so they cannot head-of-line block
// automatic work. Every tick promotes those commands when their action's
// auto_apply setting becomes true; changing it to false is safe because the
// action is rechecked immediately before execution.
type Worker struct {
	db       OutputCommandStore
	actions  ActionLister
	dispatch *Dispatcher
	interval time.Duration
	batch    int
	logger   zerolog.Logger

	stopOnce sync.Once
	stop     chan struct{}
}

// NewWorker builds a Worker. interval <= 0 is rejected by the caller's
// choice of default (main.go passes DefaultOutputWorkerInterval); Worker
// itself has no opinion on the default, mirroring Producer.
func NewWorker(db OutputCommandStore, actionStore ActionLister, dispatch *Dispatcher, interval time.Duration, logger zerolog.Logger) *Worker {
	return &Worker{
		db:       db,
		actions:  actionStore,
		dispatch: dispatch,
		interval: interval,
		batch:    DefaultOutputWorkerBatch,
		logger:   logger,
		stop:     make(chan struct{}),
	}
}

// Start runs the drain loop in a goroutine until Stop, mirroring
// Producer's Start/Stop lifecycle.
func (w *Worker) Start() {
	go func() {
		ticker := time.NewTicker(w.interval)
		defer ticker.Stop()
		for {
			select {
			case <-w.stop:
				return
			case <-ticker.C:
				w.Tick(context.Background())
			}
		}
	}()
}

// Stop halts the drain loop. Idempotent, like Producer.Stop.
func (w *Worker) Stop() {
	w.stopOnce.Do(func() { close(w.stop) })
}

// Tick promotes commands whose actions now auto-apply, then scans runnable
// commands in ID order. Manual commands leave the runnable queue without
// consuming the automatic execution batch. The scan itself is bounded so an
// unbounded manual queue cannot hold the ticker loop forever. It is exported
// so tests can drive a deterministic tick instead of waiting on the ticker.
func (w *Worker) Tick(ctx context.Context) {
	for _, action := range w.actions.List() {
		if !action.AutoApply {
			continue
		}
		if err := w.db.PromoteOutputCommandsAwaitingConfirmation(ctx, action.ID); err != nil {
			w.logger.Warn().Err(err).Str("action_id", action.ID).
				Msg("output worker: promoting confirmation-gated commands failed")
		}
	}

	maxScanned := w.batch * outputWorkerScanMultiplier
	var afterID int64
	for scanned, executed := 0, 0; scanned < maxScanned && executed < w.batch; {
		limit := min(w.batch, maxScanned-scanned)
		rows, err := w.db.ListRunnableOutputCommandsAfter(ctx, afterID, limit)
		if err != nil {
			w.logger.Warn().Err(err).Msg("output worker: listing runnable commands failed")
			return
		}
		if len(rows) == 0 {
			return
		}

		for _, row := range rows {
			afterID = row.ID
			scanned++
			if w.process(ctx, row) {
				executed++
			}
			if scanned == maxScanned || executed == w.batch {
				return
			}
		}
	}
}

// process resolves row's action, applies the auto_apply gate, renders and
// dispatches its templates, and records the outcome. It reports whether the
// row consumed an automatic execution slot; manual rows do not. A single
// row's failure (unknown action, bad payload, executor error) never aborts
// the rest of the batch.
func (w *Worker) process(ctx context.Context, row pipelinedb.OutputCommand) bool {
	action, ok := w.actions.Get(row.ActionID)
	if !ok {
		w.logger.Warn().Int64("id", row.ID).Str("action_id", row.ActionID).
			Msg("output worker: unknown action, marking command failed")
		if err := w.db.MarkOutputCommandFailed(ctx, row.ID, fmt.Sprintf("unknown action %q", row.ActionID)); err != nil {
			w.logger.Error().Err(err).Int64("id", row.ID).Msg("output worker: marking unknown-action command failed")
		}
		return true
	}

	if !action.AutoApply {
		if err := w.db.MarkOutputCommandAwaitingConfirmation(ctx, row.ID); err != nil {
			w.logger.Error().Err(err).Int64("id", row.ID).
				Msg("output worker: marking command awaiting confirmation failed")
		}
		return false
	}

	var payload map[string]any
	if err := json.Unmarshal(row.Payload, &payload); err != nil {
		w.fail(ctx, row, fmt.Errorf("decode payload: %w", err))
		return true
	}

	data := OutputData{Key: row.Key, Payload: payload, Raw: json.RawMessage(row.Payload)}
	if err := w.dispatch.Execute(ctx, action, data); err != nil {
		w.fail(ctx, row, err)
		return true
	}

	if err := w.db.MarkOutputCommandDone(ctx, row.ID); err != nil {
		w.logger.Error().Err(err).Int64("id", row.ID).Msg("output worker: marking command done failed")
	}
	return true
}

// fail records a failed execution attempt: retried (status stays pending)
// while row.Attempts remains under MaxOutputCommandAttempts, or marked
// permanently failed once the cap is reached. Either way the failure is
// logged — an action execution failure is never silently dropped.
func (w *Worker) fail(ctx context.Context, row pipelinedb.OutputCommand, execErr error) {
	w.logger.Warn().Err(execErr).Int64("id", row.ID).Str("action_id", row.ActionID).
		Int64("attempts", row.Attempts).Msg("output worker: action execution failed")

	if row.Attempts+1 >= MaxOutputCommandAttempts {
		if err := w.db.MarkOutputCommandFailed(ctx, row.ID, execErr.Error()); err != nil {
			w.logger.Error().Err(err).Int64("id", row.ID).Msg("output worker: marking command failed after max attempts")
		}
		return
	}
	if err := w.db.RetryOutputCommand(ctx, row.ID, execErr.Error()); err != nil {
		w.logger.Error().Err(err).Int64("id", row.ID).Msg("output worker: recording retry attempt failed")
	}
}
