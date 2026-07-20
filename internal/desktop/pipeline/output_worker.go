package pipeline

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/colonyops/hive/internal/desktop/activity"
	"github.com/colonyops/hive/internal/desktop/pipeline/actions"
	"github.com/colonyops/hive/internal/desktop/pipeline/pipelinedb"
	"github.com/rs/zerolog"
)

const (
	DefaultOutputWorkerInterval = 5 * time.Second
	DefaultOutputWorkerBatch    = 50
	MaxOutputCommandAttempts    = 5
)

// ActionTypeLaunchSession is the action type whose successful runs are recorded
// as session events by the launcher, so the worker leaves them to it rather
// than double-logging a generic action event.
const ActionTypeLaunchSession = "launch-session"

type OutputData struct {
	Key     string
	Payload map[string]any
	Raw     json.RawMessage
}
type Executor interface {
	Execute(context.Context, actions.Action, OutputData, ActionInvocationInput) (ExecutionResult, error)
}
type Dispatcher struct{ executors map[string]Executor }

func NewDispatcher(executors map[string]Executor) *Dispatcher {
	return &Dispatcher{executors: executors}
}

func (d *Dispatcher) Execute(ctx context.Context, a actions.Action, data OutputData, input ActionInvocationInput) (ExecutionResult, error) {
	ex, ok := d.executors[a.Type]
	if !ok {
		return ExecutionResult{}, fmt.Errorf("dispatcher: no executor registered for action type %q", a.Type)
	}
	return ex.Execute(ctx, a, data, input)
}

type ActionLister interface {
	Get(string) (actions.Action, bool)
}
type OutputCommandStore interface {
	ListRunnableOutputCommandsAfter(context.Context, int64, int) ([]pipelinedb.OutputCommand, error)
	ConfirmOutputCommand(context.Context, string, string, []byte) (pipelinedb.OutputCommand, bool, error)
	OutputCommand(context.Context, int64) (pipelinedb.OutputCommand, error)
	MarkOutputCommandDone(context.Context, int64, ...string) error
	MarkOutputCommandFailed(context.Context, int64, string, ...string) error
	RetryOutputCommand(context.Context, int64, string, ...string) error
}
type Worker struct {
	db       OutputCommandStore
	actions  ActionLister
	dispatch *Dispatcher
	interval time.Duration
	batch    int
	logger   zerolog.Logger
	recorder activity.Recorder

	runMu    sync.Mutex
	stopOnce sync.Once
	stop     chan struct{}
}

func NewWorker(db OutputCommandStore, as ActionLister, d *Dispatcher, interval time.Duration, logger zerolog.Logger) *Worker {
	return &Worker{db: db, actions: as, dispatch: d, interval: interval, batch: DefaultOutputWorkerBatch, logger: logger, stop: make(chan struct{})}
}

// SetRecorder attaches an activity recorder so automatic and manual action
// runs (and their permanent failures) surface in the Activity view. Optional:
// a nil recorder (the default) records nothing. Set once at wiring time.
func (w *Worker) SetRecorder(r activity.Recorder) { w.recorder = r }

// record forwards an activity event when a recorder is attached; nil is a
// no-op, and the recorder itself logs and swallows write failures.
func (w *Worker) record(ctx context.Context, e activity.Event) {
	if w.recorder != nil {
		w.recorder.Record(ctx, e)
	}
}

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
func (w *Worker) Stop() { w.stopOnce.Do(func() { close(w.stop) }) }
func (w *Worker) Confirm(ctx context.Context, actionID, key string, payload []byte, input ActionInvocationInput) (ActionRunView, error) {
	w.runMu.Lock()
	defer w.runMu.Unlock()
	row, created, err := w.db.ConfirmOutputCommand(ctx, actionID, key, payload)
	if err != nil {
		return ActionRunView{}, err
	}
	if !created {
		return ActionRunView{}, fmt.Errorf("action %q has already run for %q", actionID, key)
	}
	action, ok := w.actions.Get(actionID)
	if !ok {
		err = fmt.Errorf("unknown action %q", actionID)
		if markErr := w.db.MarkOutputCommandFailed(ctx, row.ID, err.Error()); markErr != nil {
			return ActionRunView{}, markErr
		}
		return w.view(ctx, row.ID), err
	}
	result, err := w.execute(ctx, row, action, input)
	if err != nil {
		// A detail-pane confirmation is an explicit, one-shot attempted side
		// effect. Persist its diagnostics and make it terminal rather than
		// retrying later without the interactive input that authorized it.
		if markErr := w.db.MarkOutputCommandFailed(ctx, row.ID, err.Error(), boundExecutionStream(result.Log.Stdout), boundExecutionStream(result.Log.Stderr)); markErr != nil {
			return ActionRunView{}, markErr
		}
		view := w.view(ctx, row.ID)
		if result.Attempted {
			// The side effect was dispatched and failed. Return its durable
			// diagnostics as a normal result so Wails can deliver them.
			return view, nil
		}
		return view, err
	}
	if err = w.done(ctx, row.ID, result); err != nil {
		return ActionRunView{}, err
	}
	// A launch-session run is recorded as a session event by the launcher.
	if action.Type != ActionTypeLaunchSession {
		w.record(ctx, activity.ActionRun(actionLabel(action), ""))
	}
	return w.view(ctx, row.ID), nil
}

func (w *Worker) Tick(ctx context.Context) {
	w.runMu.Lock()
	defer w.runMu.Unlock()
	var after int64
	for done := 0; done < w.batch; {
		rows, err := w.db.ListRunnableOutputCommandsAfter(ctx, after, w.batch-done)
		if err != nil {
			w.logger.Warn().Err(err).Msg("output worker: listing runnable commands failed")
			return
		}
		if len(rows) == 0 {
			return
		}
		for _, row := range rows {
			after = row.ID
			w.process(ctx, row)
			done++
			if done == w.batch {
				return
			}
		}
	}
}

func (w *Worker) process(ctx context.Context, row pipelinedb.OutputCommand) {
	a, ok := w.actions.Get(row.ActionID)
	if !ok {
		w.fail(ctx, row, ExecutionResult{}, fmt.Errorf("unknown action %q", row.ActionID))
		return
	}
	result, err := w.execute(ctx, row, a, ActionInvocationInput{})
	if err != nil {
		w.fail(ctx, row, result, err)
		return
	}
	if err := w.done(ctx, row.ID, result); err != nil {
		w.logger.Error().Err(err).Msg("output worker: marking command done failed")
	}
	// This is the automatic path (process only executes when the action
	// auto-applies). A launch-session run is recorded by the launcher instead.
	if a.Type != ActionTypeLaunchSession {
		w.record(ctx, activity.AutoAction(actionLabel(a), a.ID, row.Key))
	}
}

// actionLabel is the human name for an action in activity copy, falling back
// to the id when a config omits a label.
func actionLabel(action actions.Action) string {
	if action.Label != "" {
		return action.Label
	}
	return action.ID
}

func (w *Worker) execute(ctx context.Context, row pipelinedb.OutputCommand, a actions.Action, input ActionInvocationInput) (ExecutionResult, error) {
	var payload map[string]any
	if err := json.Unmarshal(row.Payload, &payload); err != nil {
		return ExecutionResult{}, fmt.Errorf("decode payload: %w", err)
	}
	return w.dispatch.Execute(ctx, a, OutputData{Key: row.Key, Payload: payload, Raw: json.RawMessage(row.Payload)}, input)
}

func (w *Worker) fail(ctx context.Context, row pipelinedb.OutputCommand, result ExecutionResult, execErr error) {
	if row.Attempts+1 >= MaxOutputCommandAttempts {
		if err := w.db.MarkOutputCommandFailed(ctx, row.ID, execErr.Error(), boundExecutionStream(result.Log.Stdout), boundExecutionStream(result.Log.Stderr)); err != nil {
			w.logger.Error().Err(err).Msg("output worker: mark failed")
		}
		// Only the terminal failure reaches the Activity view; retries stay in
		// the logs so a flaky action doesn't spam the feed.
		label := row.ActionID
		if action, ok := w.actions.Get(row.ActionID); ok {
			label = actionLabel(action)
		}
		w.record(ctx, activity.ActionFailed(label, execErr.Error()))
		return
	}
	if err := w.db.RetryOutputCommand(ctx, row.ID, execErr.Error(), boundExecutionStream(result.Log.Stdout), boundExecutionStream(result.Log.Stderr)); err != nil {
		w.logger.Error().Err(err).Msg("output worker: retry")
	}
}

func (w *Worker) view(ctx context.Context, id int64) ActionRunView {
	row, err := w.db.OutputCommand(ctx, id)
	if err != nil {
		return ActionRunView{CommandID: id, Status: "unknown", Error: err.Error()}
	}
	return actionRunView(row)
}

// boundExecutionStream is the worker boundary for all executor implementations,
// including fakes and third-party executors that do not capture output safely.
func boundExecutionStream(stream string) string {
	if len(stream) <= maxExecutionStreamBytes {
		return stream
	}
	return stream[:maxExecutionStreamBytes-len(truncatedStreamMarker)] + truncatedStreamMarker
}

func actionRunView(row pipelinedb.OutputCommand) ActionRunView {
	v := ActionRunView{CommandID: row.ID, Status: row.Status}
	if row.LastError.Valid {
		v.Error = row.LastError.String
	}
	if row.Stdout.Valid {
		v.Stdout = row.Stdout.String
	}
	if row.Stderr.Valid {
		v.Stderr = row.Stderr.String
	}
	if row.ResultJson.Valid {
		_ = json.Unmarshal([]byte(row.ResultJson.String), &v.Result)
	}
	return v
}

func (w *Worker) done(ctx context.Context, id int64, result ExecutionResult) error {
	raw, err := json.Marshal(result.Outcome)
	if err != nil {
		return err
	}
	return w.db.MarkOutputCommandDone(ctx, id, string(raw), boundExecutionStream(result.Log.Stdout), boundExecutionStream(result.Log.Stderr))
}
