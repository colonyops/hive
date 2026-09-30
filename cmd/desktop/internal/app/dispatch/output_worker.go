package dispatch

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/rs/zerolog"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/colonyops/hive/cmd/desktop/internal/app/actions"
	"github.com/colonyops/hive/cmd/desktop/internal/app/activity"
	"github.com/colonyops/hive/cmd/desktop/internal/app/data/models"
	"github.com/colonyops/hive/cmd/desktop/internal/app/data/stores"
	"github.com/colonyops/hive/cmd/desktop/internal/app/jobs"
	"github.com/colonyops/hive/cmd/desktop/internal/app/observe"
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
	// Inputs are the action's declared inputs resolved for this invocation,
	// reachable from every template as `.Inputs.<name>`; nil when the action
	// declares none. Resolution fills a key for every declared name, so
	// referencing an undeclared one is a render error (missingkey=error
	// fires on nil maps too), never a blank.
	Inputs map[string]string
	// Session and Window are the terminal target the action was invoked
	// against, reachable as `.Session.<field>` and `.Window.<field>`. Both are
	// nil for a feed-item invocation and Window is nil for a session one, so a
	// template that reads the wrong surface's data fails loudly rather than
	// rendering a blank command.
	Session *SessionTarget
	Window  *WindowTarget
	// CreatedAt is when the command was enqueued (Unix milliseconds), so an
	// executor whose side effect is time-sensitive can tell a fresh command
	// from one that waited out an app restart. Zero when unknown.
	CreatedAt int64
	CommandID int64
	IsRerun   bool
	// Origin is the inbox item this command was routed from — attribution, not
	// payload, and zero when the command has no inbox item behind it.
	Origin models.ItemRef
}
type Executor interface {
	Execute(context.Context, actions.Action, OutputData, ActionInvocationInput) (ExecutionResult, error)
}
type Dispatcher struct{ executors map[string]Executor }

func NewDispatcher(executors map[string]Executor) *Dispatcher {
	return &Dispatcher{executors: executors}
}

// Execute runs one dispatched action under a root span, with the executors
// opening conditional children for whatever they wait on. The span belongs
// here rather than in the worker because every dispatch path funnels through
// it -- the worker's automatic drain, a detail-pane confirmation, a terminal
// invocation -- and because it pairs with the jobs record rather than
// duplicating it: jobs says what happened to a command across its retries, the
// span says where one attempt spent its time.
func (d *Dispatcher) Execute(ctx context.Context, a actions.Action, data OutputData, input ActionInvocationInput) (result ExecutionResult, err error) {
	ctx, span := tracer.Start(ctx, actionSpanName(a.Type), trace.WithAttributes(
		attribute.String(attrActionID, a.ID),
		attribute.String(attrActionType, a.Type),
		attribute.String(attrTarget, data.Key),
		attribute.Int64(attrCommandID, data.CommandID),
		attribute.Bool(attrRerun, data.IsRerun),
	))
	defer observe.End(span, &err)

	ex, ok := d.executors[a.Type]
	if !ok {
		return ExecutionResult{}, fmt.Errorf("dispatcher: no executor registered for action type %q", a.Type)
	}
	result, err = ex.Execute(ctx, a, data, input)
	// A suppressed notify completes successfully having done nothing, so the
	// span has to say whether the side effect was reached at all.
	span.SetAttributes(attribute.Bool(attrAttempted, result.Attempted))
	return result, err
}

// actionSpanName names the action type, which is the executor registry's
// closed set; the action id rides as an attribute.
func actionSpanName(actionType string) string {
	if actionType == "" {
		return "dispatch.action"
	}
	return "dispatch.action " + actionType
}

type ActionLister interface {
	Get(string) (actions.Action, bool)
}
type OutputCommandStore interface {
	ListRunnableAfter(context.Context, int64, int) ([]stores.OutputCommand, error)
	Confirm(context.Context, string, string, []byte, models.ItemRef) (stores.OutputCommand, bool, error)
	Rerun(context.Context, string, string, []byte, models.ItemRef) (stores.OutputCommand, error)
	Get(context.Context, int64) (stores.OutputCommand, error)
	MarkDone(context.Context, int64, ...string) error
	MarkFailed(context.Context, int64, string, ...string) error
	Retry(context.Context, int64, string, ...string) error
}
type Worker struct {
	db          OutputCommandStore
	actions     ActionLister
	dispatch    *Dispatcher
	interval    time.Duration
	batch       int
	logger      zerolog.Logger
	recorder    activity.Recorder
	jobRecorder jobs.Recorder

	runMu    sync.Mutex
	stopOnce sync.Once
	stop     chan struct{}
}

func NewWorker(db OutputCommandStore, as ActionLister, d *Dispatcher, interval time.Duration, logger zerolog.Logger) *Worker {
	return &Worker{
		db: db, actions: as, dispatch: d, interval: interval,
		batch: DefaultOutputWorkerBatch, logger: logger,
		stop: make(chan struct{}),
	}
}

// SetRecorder attaches an activity recorder so automatic and manual action
// runs (and their permanent failures) surface in the Activity view. Optional:
// a nil recorder (the default) records nothing. Set once at wiring time.
func (w *Worker) SetRecorder(r activity.Recorder) { w.recorder = r }

// SetJobRecorder attaches a jobs recorder so manual and automatic action runs
// surface as live jobs. A nil recorder, the default, records nothing.
func (w *Worker) SetJobRecorder(r jobs.Recorder) { w.jobRecorder = r }

// record forwards an activity event when a recorder is attached; nil is a
// no-op, and the recorder itself logs and swallows write failures.
func (w *Worker) record(ctx context.Context, e activity.Event) {
	if w.recorder != nil {
		w.recorder.Record(ctx, e)
	}
}

func (w *Worker) jobBegin(ctx context.Context, label, actionID, target string) int64 {
	if w.jobRecorder == nil {
		return 0
	}
	return w.jobRecorder.Begin(ctx, label, actionID, target)
}

func (w *Worker) jobRunning(ctx context.Context, id, commandID int64) {
	if w.jobRecorder != nil && id != 0 {
		w.jobRecorder.Running(ctx, id, commandID)
	}
}

func (w *Worker) jobResume(ctx context.Context, commandID int64) int64 {
	if w.jobRecorder == nil {
		return 0
	}
	return w.jobRecorder.Resume(ctx, commandID)
}

func (w *Worker) jobDone(ctx context.Context, id int64) {
	if w.jobRecorder != nil && id != 0 {
		w.jobRecorder.Done(ctx, id)
	}
}

func (w *Worker) jobFail(ctx context.Context, id int64, reason string) {
	if w.jobRecorder != nil && id != 0 {
		w.jobRecorder.Fail(ctx, id, reason)
	}
}

func (w *Worker) jobLogger(id int64, actionID string) zerolog.Logger {
	return w.logger.With().Int64("job_id", id).Str("action_id", actionID).Logger()
}

func (w *Worker) Start(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(w.interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-w.stop:
				return
			case <-ticker.C:
				w.Tick(ctx)
			}
		}
	}()
}
func (w *Worker) Stop() { w.stopOnce.Do(func() { close(w.stop) }) }
func (w *Worker) Confirm(ctx context.Context, actionID, key string, payload []byte, origin models.ItemRef, input ActionInvocationInput) (ActionRunView, error) {
	w.runMu.Lock()
	defer w.runMu.Unlock()
	var row stores.OutputCommand
	var err error
	if input.Rerun {
		row, err = w.db.Rerun(ctx, actionID, key, payload, origin)
	} else {
		var created bool
		row, created, err = w.db.Confirm(ctx, actionID, key, payload, origin)
		if err == nil && !created {
			if row.Status == "pending" || row.Status == "running" {
				return ActionRunView{}, fmt.Errorf("action %q is already running for %q", actionID, key)
			}
			view := w.view(ctx, row.ID)
			view.ConfirmationRequired = true
			return view, nil
		}
	}
	if err != nil {
		return ActionRunView{}, err
	}

	action, ok := w.actions.Get(actionID)
	label := actionID
	if ok {
		label = actionLabel(action)
	}
	jobID := w.jobBegin(ctx, label, actionID, key)
	logger := w.jobLogger(jobID, actionID)
	logger.Debug().Msg("output worker: job queued")

	if !ok {
		err = fmt.Errorf("unknown action %q", actionID)
		if markErr := w.db.MarkFailed(ctx, row.ID, err.Error()); markErr != nil {
			logger.Error().Err(markErr).Msg("output worker: marking command failed")
			return ActionRunView{}, markErr
		}
		w.jobFail(ctx, jobID, err.Error())
		logger.Debug().Err(err).Msg("output worker: job failed")
		return w.view(ctx, row.ID), err
	}

	w.jobRunning(ctx, jobID, row.ID)
	logger.Debug().Int64("command_id", row.ID).Msg("output worker: job running")
	result, err := w.execute(ctx, row, action, input, logger)
	if err != nil {
		// A detail-pane confirmation is an explicit, one-shot attempted side
		// effect. Persist its diagnostics and make it terminal rather than
		// retrying later without the interactive input that authorized it.
		if markErr := w.db.MarkFailed(ctx, row.ID, err.Error(), boundExecutionStream(result.Log.Stdout), boundExecutionStream(result.Log.Stderr)); markErr != nil {
			logger.Error().Err(markErr).Msg("output worker: marking command failed")
			return ActionRunView{}, markErr
		}
		w.jobFail(ctx, jobID, err.Error())
		logger.Debug().Err(err).Msg("output worker: job failed")
		view := w.view(ctx, row.ID)
		if result.Attempted {
			// The side effect was dispatched and failed. Return its durable
			// diagnostics as a normal result so Wails can deliver them.
			return view, nil
		}
		return view, err
	}
	if err = w.done(ctx, row.ID, result); err != nil {
		logger.Error().Err(err).Msg("output worker: marking command done failed")
		return ActionRunView{}, err
	}
	w.jobDone(ctx, jobID)
	logger.Debug().Msg("output worker: job done")
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
		rows, err := w.db.ListRunnableAfter(ctx, after, w.batch-done)
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

func (w *Worker) process(ctx context.Context, row stores.OutputCommand) {
	a, ok := w.actions.Get(row.ActionID)
	label := row.ActionID
	if ok {
		label = actionLabel(a)
	}
	jobID := w.jobResume(ctx, row.ID)
	resumed := jobID != 0
	if !resumed {
		jobID = w.jobBegin(ctx, label, row.ActionID, row.Key)
	}
	logger := w.jobLogger(jobID, row.ActionID)
	if !resumed {
		logger.Debug().Msg("output worker: job queued")
	}

	if !resumed {
		w.jobRunning(ctx, jobID, row.ID)
		logger.Debug().Int64("command_id", row.ID).Msg("output worker: job running")
	}
	if !ok {
		w.fail(ctx, row, ExecutionResult{}, fmt.Errorf("unknown action %q", row.ActionID), jobID, logger)
		return
	}
	result, err := w.execute(ctx, row, a, ActionInvocationInput{}, logger)
	if err != nil {
		w.fail(ctx, row, result, err, jobID, logger)
		return
	}
	if err := w.done(ctx, row.ID, result); err != nil {
		logger.Error().Err(err).Msg("output worker: marking command done failed")
		return
	}
	w.jobDone(ctx, jobID)
	logger.Debug().Msg("output worker: job done")
	// This is the automatic path (process only executes when the action
	// auto-applies). A launch-session run is recorded by the launcher instead,
	// and an executor that deliberately performed no side effect (a notify
	// command suppressed by settings or a cooldown) reports Attempted false —
	// the Activity view records what happened, not what was considered.
	if a.Type != ActionTypeLaunchSession && result.Attempted {
		w.record(ctx, automaticActionActivity(a, row))
	}
}

func automaticActionActivity(action actions.Action, row stores.OutputCommand) activity.Event {
	target := row.Key
	link := activity.Link{}
	if action.Type == ActionTypeNotify {
		target = ""
		var command models.NotifyCommand
		if json.Unmarshal(row.Payload, &command) == nil {
			target = command.ExternalID
			var item struct {
				URL string `json:"url"`
			}
			_ = json.Unmarshal(command.Item, &item)
			link.URL = item.URL
			if command.ProfileID != "" && command.ExternalID != "" {
				link.Item = &activity.ItemLink{
					ProfileID:   command.ProfileID,
					SourceKind:  command.SourceKind,
					SourceScope: command.SourceScope,
					ExternalID:  command.ExternalID,
				}
			}
		}
	}
	return activity.AutoAction(actionLabel(action), action.ID, target).WithLink(link)
}

// actionLabel is the human name for an action in activity copy, falling back
// to the id when a config omits a label.
func actionLabel(action actions.Action) string {
	if action.Label != "" {
		return action.Label
	}
	return action.ID
}

func (w *Worker) execute(
	ctx context.Context,
	row stores.OutputCommand,
	a actions.Action,
	input ActionInvocationInput,
	logger zerolog.Logger,
) (ExecutionResult, error) {
	var payload map[string]any
	if err := json.Unmarshal(row.Payload, &payload); err != nil {
		logger.Warn().Err(err).Msg("output worker: decoding command payload failed")
		return ExecutionResult{}, fmt.Errorf("decode payload: %w", err)
	}
	// Resolving here rather than at the callers is what makes the automatic
	// path work at all: a flow-fired command carries no collected values, so
	// this is where its declared defaults are filled in.
	inputs, err := a.ResolveInputs(input.Inputs)
	if err != nil {
		return ExecutionResult{}, err
	}
	return w.dispatch.Execute(ctx, a, OutputData{
		Key: row.Key, Payload: payload, Raw: json.RawMessage(row.Payload), Inputs: inputs,
		CreatedAt: row.CreatedAt, CommandID: row.ID, IsRerun: row.IsRerun, Origin: row.ItemRef(),
	}, input)
}

func (w *Worker) fail(
	ctx context.Context,
	row stores.OutputCommand,
	result ExecutionResult,
	execErr error,
	jobID int64,
	logger zerolog.Logger,
) {
	if row.Attempts+1 >= MaxOutputCommandAttempts {
		if err := w.db.MarkFailed(ctx, row.ID, execErr.Error(), boundExecutionStream(result.Log.Stdout), boundExecutionStream(result.Log.Stderr)); err != nil {
			logger.Error().Err(err).Msg("output worker: mark failed")
			return
		}
		w.jobFail(ctx, jobID, execErr.Error())
		logger.Debug().Err(execErr).Msg("output worker: job failed")
		// Only the terminal failure reaches the Activity view; retries stay in
		// the logs so a flaky action doesn't spam the feed.
		label := row.ActionID
		if action, ok := w.actions.Get(row.ActionID); ok {
			label = actionLabel(action)
		}
		w.record(ctx, activity.ActionFailed(label, execErr.Error()))
		return
	}
	if err := w.db.Retry(ctx, row.ID, execErr.Error(), boundExecutionStream(result.Log.Stdout), boundExecutionStream(result.Log.Stderr)); err != nil {
		logger.Error().Err(err).Msg("output worker: retry")
		return
	}
	logger.Debug().Err(execErr).Msg("output worker: command scheduled for retry")
}

func (w *Worker) view(ctx context.Context, id int64) ActionRunView {
	row, err := w.db.Get(ctx, id)
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

func actionRunView(row stores.OutputCommand) ActionRunView {
	v := ActionRunView{CommandID: row.ID, Status: row.Status}
	if row.LastError != "" {
		v.Error = row.LastError
	}
	if row.Stdout != "" {
		v.Stdout = row.Stdout
	}
	if row.Stderr != "" {
		v.Stderr = row.Stderr
	}
	if row.ResultJSON != "" {
		_ = json.Unmarshal([]byte(row.ResultJSON), &v.Result)
	}
	return v
}

func (w *Worker) done(ctx context.Context, id int64, result ExecutionResult) error {
	raw, err := json.Marshal(result.Outcome)
	if err != nil {
		return err
	}
	return w.db.MarkDone(ctx, id, string(raw), boundExecutionStream(result.Log.Stdout), boundExecutionStream(result.Log.Stderr))
}
