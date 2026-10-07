package dispatch

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/colonyops/hive/cmd/desktop/internal/app/actions"
	"github.com/colonyops/hive/cmd/desktop/internal/app/activity"
	"github.com/colonyops/hive/cmd/desktop/internal/app/data/models"
	"github.com/colonyops/hive/cmd/desktop/internal/app/data/stores"
	"github.com/colonyops/hive/cmd/desktop/internal/app/jobs"
	"github.com/colonyops/hive/internal/platform/observe"
)

const (
	DefaultOutputWorkerInterval = 5 * time.Second
	DefaultOutputWorkerBatch    = 50
	DefaultManualConcurrency    = 4
	DefaultAutomaticConcurrency = 4
	MaxOutputCommandAttempts    = 5
	automaticRetryDelay         = 5 * time.Second
	outputCleanupTimeout        = 3 * time.Second
)

var ErrDispatchBusy = errors.New("action dispatch is busy")

const ActionTypeLaunchSession = "launch-session"

type OutputData struct {
	Key       string
	Payload   map[string]any
	Raw       json.RawMessage
	Inputs    map[string]string
	Session   *SessionTarget
	Window    *WindowTarget
	CreatedAt int64
	CommandID int64
	IsRerun   bool
	Origin    models.ItemRef
}

func (d OutputData) ItemRemote() string {
	repo, _ := d.Payload["repo"].(string)
	itemURL, _ := d.Payload["url"].(string)
	return draftRepository(repo, itemURL)
}

type Executor interface {
	Execute(context.Context, actions.Action, OutputData, ActionInvocationInput) (ExecutionResult, error)
}

type Dispatcher struct{ executors map[string]Executor }

func NewDispatcher(executors map[string]Executor) *Dispatcher {
	return &Dispatcher{executors: executors}
}

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
	span.SetAttributes(attribute.Bool(attrAttempted, result.Attempted))
	return result, err
}

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
	ClaimNextAutomatic(context.Context, int64, string) (stores.OutputCommand, bool, error)
	Confirm(context.Context, string, string, []byte, models.ItemRef, string) (stores.OutputCommand, bool, error)
	Rerun(context.Context, string, string, []byte, models.ItemRef, string) (stores.OutputCommand, error)
	Get(context.Context, int64) (stores.OutputCommand, error)
	Complete(context.Context, int64, string, string) error
	Fail(context.Context, int64, string, string) error
	Requeue(context.Context, int64, string, string, time.Duration) error
	Cancel(context.Context, int64, string, string) error
}

type Worker struct {
	db          OutputCommandStore
	actions     ActionLister
	dispatch    *Dispatcher
	interval    time.Duration
	batch       int
	retryDelay  time.Duration
	logger      zerolog.Logger
	recorder    activity.Recorder
	jobRecorder jobs.Recorder
	runLogs     RunLogSink

	manualSlots chan struct{}
	autoSlots   chan struct{}
	wg          sync.WaitGroup
	mu          sync.Mutex
	active      map[int64]context.CancelFunc
	stopped     bool
	startOnce   sync.Once
	stopOnce    sync.Once
	stop        chan struct{}
}

func NewWorker(db OutputCommandStore, as ActionLister, d *Dispatcher, interval time.Duration, logger zerolog.Logger) *Worker {
	return &Worker{
		db: db, actions: as, dispatch: d, interval: interval,
		batch: DefaultOutputWorkerBatch, retryDelay: automaticRetryDelay, logger: logger,
		manualSlots: make(chan struct{}, DefaultManualConcurrency),
		autoSlots:   make(chan struct{}, DefaultAutomaticConcurrency),
		active:      make(map[int64]context.CancelFunc),
		stop:        make(chan struct{}),
	}
}

func (w *Worker) SetRecorder(r activity.Recorder) { w.recorder = r }
func (w *Worker) SetJobRecorder(r jobs.Recorder)  { w.jobRecorder = r }
func (w *Worker) SetRunLogSink(s RunLogSink)      { w.runLogs = s }

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

func (w *Worker) jobCancel(ctx context.Context, id int64, reason string) {
	if w.jobRecorder != nil && id != 0 {
		w.jobRecorder.Cancel(ctx, id, reason)
	}
}

func (w *Worker) jobLogger(id int64, actionID string) zerolog.Logger {
	return w.logger.With().Int64("job_id", id).Str("action_id", actionID).Logger()
}

func (w *Worker) Start(ctx context.Context) {
	w.startOnce.Do(func() {
		go func() {
			w.Tick(ctx)
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
	})
}

func (w *Worker) Stop(ctx context.Context) error {
	w.stopOnce.Do(func() {
		w.mu.Lock()
		w.stopped = true
		close(w.stop)
		for _, cancel := range w.active {
			cancel()
		}
		w.mu.Unlock()
	})
	done := make(chan struct{})
	go func() {
		w.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (w *Worker) Confirm(ctx context.Context, actionID, key string, payload []byte, origin models.ItemRef, input ActionInvocationInput) (ActionRunView, error) {
	action, ok := w.actions.Get(actionID)
	if !ok {
		return ActionRunView{}, fmt.Errorf("unknown action %q", actionID)
	}
	if _, err := action.ResolveInputs(input.Inputs); err != nil {
		return ActionRunView{}, err
	}
	if !w.reserveManual() {
		return ActionRunView{}, ErrDispatchBusy
	}

	claimToken := uuid.NewString()
	var row stores.OutputCommand
	var err error
	if input.Rerun {
		row, err = w.db.Rerun(ctx, actionID, key, payload, origin, claimToken)
	} else {
		var created bool
		row, created, err = w.db.Confirm(ctx, actionID, key, payload, origin, claimToken)
		if err == nil && !created {
			w.releaseManual()
			view := actionRunView(row)
			if row.Status != "pending" && row.Status != "running" {
				view.ConfirmationRequired = true
			}
			return view, nil
		}
	}
	if err != nil {
		w.releaseManual()
		return ActionRunView{}, err
	}

	jobID := w.jobBegin(ctx, actionLabel(action), actionID, key)
	w.jobRunning(ctx, jobID, row.ID)
	w.launch(ctx, row, action, input, jobID, w.manualSlots, true)
	return ActionRunView{CommandID: row.ID, Status: "running"}, nil
}

func (w *Worker) Tick(ctx context.Context) {
	var afterID int64
	for claimed := 0; claimed < w.batch; claimed++ {
		if !w.reserveAutomatic() {
			return
		}
		claimToken := uuid.NewString()
		row, found, err := w.db.ClaimNextAutomatic(ctx, afterID, claimToken)
		if err != nil {
			w.releaseAutomatic()
			w.logger.Warn().Err(err).Msg("output worker: claiming runnable command failed")
			return
		}
		if !found {
			w.releaseAutomatic()
			return
		}
		afterID = row.ID

		action, ok := w.actions.Get(row.ActionID)
		label := row.ActionID
		if ok {
			label = actionLabel(action)
		}
		jobID := w.jobResume(ctx, row.ID)
		if jobID == 0 {
			jobID = w.jobBegin(ctx, label, row.ActionID, row.Key)
			w.jobRunning(ctx, jobID, row.ID)
		}
		if !ok {
			action = actions.Action{ID: row.ActionID}
		}
		w.launch(ctx, row, action, ActionInvocationInput{}, jobID, w.autoSlots, false)
	}
}

func (w *Worker) Cancel(commandID int64) bool {
	w.mu.Lock()
	cancel, ok := w.active[commandID]
	w.mu.Unlock()
	if ok {
		cancel()
	}
	return ok
}

// WaitIdle waits until every admitted execution has persisted its terminal transition.
func (w *Worker) WaitIdle(ctx context.Context) error {
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	for {
		w.mu.Lock()
		idle := len(w.active) == 0
		w.mu.Unlock()
		if idle {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func (w *Worker) reserveManual() bool {
	w.mu.Lock()
	stopped := w.stopped
	w.mu.Unlock()
	if stopped {
		return false
	}
	select {
	case w.manualSlots <- struct{}{}:
		return true
	default:
		return false
	}
}

func (w *Worker) reserveAutomatic() bool {
	w.mu.Lock()
	stopped := w.stopped
	w.mu.Unlock()
	if stopped {
		return false
	}
	select {
	case w.autoSlots <- struct{}{}:
		return true
	default:
		return false
	}
}

func (w *Worker) releaseManual()    { <-w.manualSlots }
func (w *Worker) releaseAutomatic() { <-w.autoSlots }

func (w *Worker) launch(ctx context.Context, row stores.OutputCommand, action actions.Action, input ActionInvocationInput, jobID int64, slot chan struct{}, manual bool) {
	execCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	w.mu.Lock()
	if w.stopped {
		w.mu.Unlock()
		cancel()
		<-slot
		cleanupCtx, cleanupCancel := context.WithTimeout(context.WithoutCancel(ctx), outputCleanupTimeout)
		defer cleanupCancel()
		const reason = "Action cancelled because Hive Desktop is stopping"
		if err := w.db.Cancel(cleanupCtx, row.ID, row.ClaimToken, reason); err != nil {
			w.logger.Error().Err(err).Int64("command_id", row.ID).Msg("output worker: cancelling unstarted command")
			return
		}
		w.jobCancel(cleanupCtx, jobID, reason)
		return
	}
	w.active[row.ID] = cancel
	w.wg.Add(1)
	w.mu.Unlock()

	go func() {
		defer func() {
			cancel()
			w.mu.Lock()
			delete(w.active, row.ID)
			w.mu.Unlock()
			<-slot
			w.wg.Done()
		}()
		w.run(execCtx, row, action, input, jobID, manual)
	}()
}

func (w *Worker) run(ctx context.Context, row stores.OutputCommand, action actions.Action, input ActionInvocationInput, jobID int64, manual bool) {
	logger := w.jobLogger(jobID, row.ActionID)
	logger.Debug().Int64("command_id", row.ID).Msg("output worker: job running")

	var runLog *RunLog
	if models.IsCatalogActionID(row.ActionID) {
		runLog = newRunLog(ctx, w.runLogs, row.ID, row.Attempts, logger)
	}
	runLog.Systemf("%s", runLogHeader(row, action, manual))
	started := time.Now()

	var result ExecutionResult
	var execErr error
	if action.Type == "" {
		execErr = fmt.Errorf("unknown action %q", row.ActionID)
	} else {
		result, execErr = w.execute(WithRunLog(ctx, runLog), row, action, input, logger)
	}
	elapsed := formatRunDuration(time.Since(started))
	switch {
	case ctx.Err() != nil:
		runLog.Systemf("Cancelled after %s", elapsed)
	case execErr != nil && failureIsTerminal(row, result, manual):
		runLog.Systemf("Failed after %s: %v", elapsed, execErr)
	case execErr != nil:
		runLog.Systemf("Attempt %d failed after %s: %v\nRetrying in %s", row.Attempts, elapsed, execErr, formatRunDuration(w.retryDelay))
	default:
		runLog.Systemf("Completed in %s", elapsed)
	}
	runLog.Close(ctx)

	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), outputCleanupTimeout)
	defer cancel()
	if ctx.Err() != nil {
		reason := "Action cancelled"
		if err := w.db.Cancel(cleanupCtx, row.ID, row.ClaimToken, reason); err != nil {
			logger.Error().Err(err).Msg("output worker: cancelling command")
			return
		}
		w.jobCancel(cleanupCtx, jobID, reason)
		logger.Debug().Msg("output worker: job cancelled")
		return
	}
	if execErr != nil {
		w.finishFailure(cleanupCtx, row, action, result, execErr, jobID, manual, logger)
		return
	}
	if err := w.done(cleanupCtx, row, result); err != nil {
		logger.Error().Err(err).Msg("output worker: marking command done failed")
		return
	}
	w.jobDone(cleanupCtx, jobID)
	logger.Debug().Msg("output worker: job done")
	if action.Type != ActionTypeLaunchSession && (manual || result.Attempted) {
		if manual {
			w.record(cleanupCtx, activity.ActionRun(actionLabel(action), ""))
		} else {
			w.record(cleanupCtx, automaticActionActivity(action, row))
		}
	}
}

func (w *Worker) finishFailure(ctx context.Context, row stores.OutputCommand, action actions.Action, result ExecutionResult, execErr error, jobID int64, manual bool, logger zerolog.Logger) {
	if failureIsTerminal(row, result, manual) {
		if err := w.db.Fail(ctx, row.ID, row.ClaimToken, execErr.Error()); err != nil {
			logger.Error().Err(err).Msg("output worker: mark failed")
			return
		}
		w.jobFail(ctx, jobID, execErr.Error())
		logger.Warn().Err(execErr).Int64("command_id", row.ID).Str("key", row.Key).Int64("attempts", row.Attempts).Msg("output worker: command failed permanently")
		label := row.ActionID
		if action.Type != "" {
			label = actionLabel(action)
		}
		w.record(ctx, activity.ActionFailed(label, execErr.Error()))
		return
	}
	if err := w.db.Requeue(ctx, row.ID, row.ClaimToken, execErr.Error(), w.retryDelay); err != nil {
		logger.Error().Err(err).Msg("output worker: retry")
		return
	}
	logger.Debug().Err(execErr).Msg("output worker: command scheduled for retry")
}

// A manual run and an attempted side effect never retry: the first had a
// person behind it, and the second may already have happened.
func failureIsTerminal(row stores.OutputCommand, result ExecutionResult, manual bool) bool {
	return manual || result.Attempted || row.Attempts >= MaxOutputCommandAttempts
}

func runLogHeader(row stores.OutputCommand, action actions.Action, manual bool) string {
	lane := "automatic"
	if manual {
		lane = "manual"
	}
	kind := action.Type
	if kind == "" {
		kind = "unknown type"
	}
	header := fmt.Sprintf("Started %s (%s, %s, attempt %d)", actionLabel(action), kind, lane, row.Attempts)
	if row.IsRerun {
		header += ", rerun"
	}
	return header
}

func formatRunDuration(d time.Duration) string {
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	return d.Round(100 * time.Millisecond).String()
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
					ProfileID: command.ProfileID, SourceKind: command.SourceKind,
					SourceScope: command.SourceScope, ExternalID: command.ExternalID,
				}
			}
		}
	}
	return activity.AutoAction(actionLabel(action), action.ID, target).WithLink(link)
}

func actionLabel(action actions.Action) string {
	if action.Label != "" {
		return action.Label
	}
	return action.ID
}

func (w *Worker) execute(ctx context.Context, row stores.OutputCommand, action actions.Action, input ActionInvocationInput, logger zerolog.Logger) (ExecutionResult, error) {
	var payload map[string]any
	if err := json.Unmarshal(row.Payload, &payload); err != nil {
		logger.Warn().Err(err).Msg("output worker: decoding command payload failed")
		return ExecutionResult{}, fmt.Errorf("decode payload: %w", err)
	}
	inputs, err := action.ResolveInputs(input.Inputs)
	if err != nil {
		return ExecutionResult{}, err
	}
	return w.dispatch.Execute(ctx, action, OutputData{
		Key: row.Key, Payload: payload, Raw: json.RawMessage(row.Payload), Inputs: inputs,
		CreatedAt: row.CreatedAt, CommandID: row.ID, IsRerun: row.IsRerun, Origin: row.ItemRef(),
	}, input)
}

func (w *Worker) view(ctx context.Context, id int64) ActionRunView {
	row, err := w.db.Get(ctx, id)
	if err != nil {
		return ActionRunView{CommandID: id, Status: "unknown", Error: err.Error()}
	}
	return actionRunView(row)
}

func actionRunView(row stores.OutputCommand) ActionRunView {
	view := ActionRunView{CommandID: row.ID, Status: row.Status}
	if row.LastError != "" {
		view.Error = row.LastError
	}
	if row.ResultJSON != "" {
		_ = json.Unmarshal([]byte(row.ResultJSON), &view.Result)
	}
	return view
}

func (w *Worker) done(ctx context.Context, row stores.OutputCommand, result ExecutionResult) error {
	raw, err := json.Marshal(result.Outcome)
	if err != nil {
		return err
	}
	return w.db.Complete(ctx, row.ID, row.ClaimToken, string(raw))
}
