package dispatch

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/desktop/internal/app/actions"
	"github.com/colonyops/hive/cmd/desktop/internal/app/data/models"
	"github.com/colonyops/hive/cmd/desktop/internal/app/data/queries"
	"github.com/colonyops/hive/cmd/desktop/internal/app/data/stores"
)

// openTestPipelineDB opens a throwaway store on a temp dir, migrated and
// closed with the test.
func openTestPipelineDB(t *testing.T) *queries.DB {
	t.Helper()
	db, err := queries.Open(t.Context(), t.TempDir(), queries.DefaultOpenOptions())
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func testOutputCommands(db *queries.DB) *stores.OutputCommandStore {
	return stores.New(db, stores.Options{}).OutputCommands
}

func waitForWorker(t *testing.T, worker *Worker) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	require.NoError(t, worker.WaitIdle(ctx))
}

// enqueueTestCommand enqueues one output_command row via EventLogStore.Commit
// (the only production path that ever writes one), so tests exercise the
// real dedup/enqueue behavior rather than inserting rows by hand.
func enqueueTestCommand(t *testing.T, db *queries.DB, actionID, key, payload string) {
	t.Helper()
	require.NoError(t, stores.New(db, stores.Options{}).EventLog.Commit(t.Context(), models.CommitBatch{
		Consumer:   "test-consumer-" + actionID + "-" + key,
		UpToOffset: 1,
		Outputs: []models.Output{
			{
				Sink:          models.Sink{Kind: models.SinkKindAction, TargetID: actionID},
				OccurrenceKey: key,
				Payload:       []byte(payload),
			},
		},
	}))
}

// fakeActionLister is an in-memory ActionLister for tests that don't want to
// touch disk via actions.ActionStore.
type fakeActionLister map[string]actions.Action

func (f fakeActionLister) Get(id string) (actions.Action, bool) {
	a, ok := f[id]
	return a, ok
}

func (f fakeActionLister) List() []actions.Action {
	out := make([]actions.Action, 0, len(f))
	for _, action := range f {
		out = append(out, action)
	}
	return out
}

// fakeExecutor records every Execute call and returns a scripted error (or
// nil) each time, so tests can assert exactly what the worker dispatched.
type fakeExecutor struct {
	mu     sync.Mutex
	calls  []OutputData
	err    error
	result ExecutionResult
}

type blockingExecutor struct {
	started chan OutputData
	release chan struct{}
}

func (e *blockingExecutor) Execute(ctx context.Context, _ actions.Action, data OutputData, _ ActionInvocationInput) (ExecutionResult, error) {
	e.started <- data
	select {
	case <-e.release:
		return ExecutionResult{Attempted: true}, nil
	case <-ctx.Done():
		return ExecutionResult{Attempted: true}, ctx.Err()
	}
}

type fakeJobRecorder struct {
	calls     []string
	label     string
	actionID  string
	target    string
	commandID int64
	active    bool
	reason    string
}

func (f *fakeJobRecorder) Begin(_ context.Context, label, actionID, target string) int64 {
	f.calls = append(f.calls, "Begin")
	f.label = label
	f.actionID = actionID
	f.target = target
	return 42
}

func (f *fakeJobRecorder) Running(_ context.Context, _ int64, commandID int64) {
	f.calls = append(f.calls, "Running")
	f.commandID = commandID
	f.active = true
}

func (f *fakeJobRecorder) Resume(_ context.Context, commandID int64) int64 {
	if f.active && f.commandID == commandID {
		return 42
	}
	return 0
}

func (f *fakeJobRecorder) Done(_ context.Context, _ int64) {
	f.calls = append(f.calls, "Done")
	f.active = false
}

func (f *fakeJobRecorder) Fail(_ context.Context, _ int64, reason string) {
	f.calls = append(f.calls, "Fail")
	f.active = false
	f.reason = reason
}

func (f *fakeJobRecorder) Cancel(_ context.Context, _ int64, reason string) {
	f.calls = append(f.calls, "Cancel")
	f.active = false
	f.reason = reason
}

func (f *fakeExecutor) Execute(_ context.Context, _ actions.Action, data OutputData, _ ActionInvocationInput) (ExecutionResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, data)
	return f.result, f.err
}

func (f *fakeExecutor) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.calls)
}

func launchSessionAction(id string, autoApply bool) actions.Action {
	return actions.Action{
		ID:     id,
		Label:  "Test action",
		Type:   "launch-session",
		Config: &actions.LaunchSessionConfig{PromptTemplate: "Review {{ .Payload.title }}", RepoTemplate: "https://github.com/o/r.git"},
	}
}

func TestWorker_AutoApplyAction_ExecutesAndMarksDone(t *testing.T) {
	t.Parallel()
	db := openTestPipelineDB(t)
	enqueueTestCommand(t, db, "spawn-review", "item-1", `{"title":"Fix bug"}`)

	exec := &fakeExecutor{}
	worker := NewWorker(testOutputCommands(db), fakeActionLister{"spawn-review": launchSessionAction("spawn-review", true)},
		NewDispatcher(map[string]Executor{"launch-session": exec}), 0, zerolog.Nop())

	worker.Tick(t.Context())
	waitForWorker(t, worker)

	require.Equal(t, 1, exec.callCount())
	assert.Equal(t, "item-1", exec.calls[0].Key)
	assert.Equal(t, "Fix bug", exec.calls[0].Payload["title"])

	rows, err := testOutputCommands(db).ListRunnableAfter(t.Context(), 0, 10)
	require.NoError(t, err)
	assert.Empty(t, rows, "a successfully executed command is no longer runnable")
}

func TestWorker_HeadlessActionExecutesAndConsumesQueue(t *testing.T) {
	t.Parallel()
	db := openTestPipelineDB(t)
	enqueueTestCommand(t, db, "headless-action", "item-1", `{"title":"Fix bug"}`)

	exec := &fakeExecutor{}
	worker := NewWorker(testOutputCommands(db), fakeActionLister{"headless-action": launchSessionAction("headless-action", false)},
		NewDispatcher(map[string]Executor{"launch-session": exec}), 0, zerolog.Nop())
	worker.Tick(t.Context())
	waitForWorker(t, worker)

	assert.Equal(t, 1, exec.callCount())
	rows, err := testOutputCommands(db).ListRunnableAfter(t.Context(), 0, 10)
	require.NoError(t, err)
	assert.Empty(t, rows)
	var status string
	require.NoError(t, db.Conn().QueryRowContext(t.Context(), `SELECT status FROM output_command WHERE action_id = ? AND key = ?`, "headless-action", "item-1").Scan(&status))
	assert.Equal(t, "done", status)
}

func TestWorker_ConfirmRequiresApprovalBeforeRerunningCompletedCommand(t *testing.T) {
	t.Parallel()
	db := openTestPipelineDB(t)
	enqueueTestCommand(t, db, "review-action", "item-1", `{"title":"Fix bug"}`)

	exec := &fakeExecutor{}
	worker := NewWorker(testOutputCommands(db), fakeActionLister{"review-action": launchSessionAction("review-action", false)},
		NewDispatcher(map[string]Executor{"launch-session": exec}), 0, zerolog.Nop())
	worker.Tick(t.Context())
	waitForWorker(t, worker)

	view, err := worker.Confirm(t.Context(), "review-action", "item-1", []byte(`{"title":"changed"}`), models.ItemRef{}, ActionInvocationInput{})
	require.NoError(t, err)
	assert.True(t, view.ConfirmationRequired)
	assert.Equal(t, 1, exec.callCount(), "the duplicate must not execute before confirmation")
	assert.Equal(t, "Fix bug", exec.calls[0].Payload["title"], "the worker preserves the queued command payload")

	rerun, err := worker.Confirm(t.Context(), "review-action", "item-1", []byte(`{"title":"changed"}`), models.ItemRef{}, ActionInvocationInput{Rerun: true})
	require.NoError(t, err)
	assert.False(t, rerun.ConfirmationRequired)
	assert.Equal(t, "running", rerun.Status)
	waitForWorker(t, worker)
	rerun = worker.view(t.Context(), rerun.CommandID)
	assert.Equal(t, "done", rerun.Status)
	assert.Equal(t, 2, exec.callCount())
	assert.Equal(t, "changed", exec.calls[1].Payload["title"])

	var count int
	require.NoError(t, db.Conn().QueryRowContext(t.Context(),
		`SELECT COUNT(*) FROM output_command WHERE action_id = ? AND key = ?`,
		"review-action", "item-1",
	).Scan(&count))
	assert.Equal(t, 2, count, "a confirmed rerun preserves the original command history")
}

func TestWorker_ConfirmRecordsJobLifecycleWithoutReplay(t *testing.T) {
	t.Parallel()
	db := openTestPipelineDB(t)
	exec := &fakeExecutor{}
	recorder := &fakeJobRecorder{}
	worker := NewWorker(testOutputCommands(db), fakeActionLister{"review-action": launchSessionAction("review-action", false)},
		NewDispatcher(map[string]Executor{"launch-session": exec}), 0, zerolog.Nop())
	worker.SetJobRecorder(recorder)

	_, err := worker.Confirm(t.Context(), "review-action", "item-1", []byte(`{"title":"Fix bug"}`), models.ItemRef{}, ActionInvocationInput{})
	require.NoError(t, err)
	waitForWorker(t, worker)
	assert.Equal(t, []string{"Begin", "Running", "Done"}, recorder.calls)
	assert.Equal(t, "Test action", recorder.label)
	assert.Equal(t, "review-action", recorder.actionID)
	assert.Equal(t, "item-1", recorder.target)
	assert.Positive(t, recorder.commandID)

	recorder.calls = nil
	view, err := worker.Confirm(t.Context(), "review-action", "item-1", []byte(`{}`), models.ItemRef{}, ActionInvocationInput{})
	require.NoError(t, err)
	assert.True(t, view.ConfirmationRequired)
	assert.Empty(t, recorder.calls, "an unconfirmed rerun must not create a phantom job")
}

func TestWorker_ConfirmUnknownActionCreatesNoJob(t *testing.T) {
	t.Parallel()
	db := openTestPipelineDB(t)
	recorder := &fakeJobRecorder{}
	worker := NewWorker(testOutputCommands(db), fakeActionLister{}, NewDispatcher(nil), 0, zerolog.Nop())
	worker.SetJobRecorder(recorder)

	_, err := worker.Confirm(t.Context(), "missing", "item-1", []byte(`{}`), models.ItemRef{}, ActionInvocationInput{})
	require.Error(t, err)
	assert.Empty(t, recorder.calls)
}

func TestWorker_ConfirmCreatesCommandWhenNoActionNodeProducedOne(t *testing.T) {
	t.Parallel()
	db := openTestPipelineDB(t)
	exec := &fakeExecutor{}
	worker := NewWorker(testOutputCommands(db), fakeActionLister{"review-action": launchSessionAction("review-action", false)},
		NewDispatcher(map[string]Executor{"launch-session": exec}), 0, zerolog.Nop())

	_, err := worker.Confirm(t.Context(), "review-action", "item-1", []byte(`{"title":"Fix bug"}`), models.ItemRef{}, ActionInvocationInput{})
	require.NoError(t, err)
	waitForWorker(t, worker)
	assert.Equal(t, 1, exec.callCount())

	var status string
	require.NoError(t, db.Conn().QueryRowContext(t.Context(),
		`SELECT status FROM output_command WHERE action_id = ? AND key = ?`,
		"review-action", "item-1",
	).Scan(&status))
	assert.Equal(t, "done", status)
}

func TestWorker_RespectsBatchBoundAndResumesQueue(t *testing.T) {
	t.Parallel()
	db := openTestPipelineDB(t)
	for i := range DefaultOutputWorkerBatch {
		enqueueTestCommand(t, db, "headless-action", fmt.Sprintf("item-%d", i), `{}`)
	}
	enqueueTestCommand(t, db, "headless-action", "item-after-bound", `{}`)

	exec := &fakeExecutor{}
	worker := NewWorker(testOutputCommands(db), fakeActionLister{"headless-action": launchSessionAction("headless-action", true)},
		NewDispatcher(map[string]Executor{"launch-session": exec}), 0, zerolog.Nop())
	worker.autoSlots = make(chan struct{}, DefaultOutputWorkerBatch+1)
	worker.Tick(t.Context())
	waitForWorker(t, worker)
	assert.Equal(t, DefaultOutputWorkerBatch, exec.callCount(), "one tick is bounded")
	rows, err := testOutputCommands(db).ListRunnableAfter(t.Context(), 0, 10)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, "item-after-bound", rows[0].Key)

	worker.Tick(t.Context())
	waitForWorker(t, worker)
	assert.Equal(t, DefaultOutputWorkerBatch+1, exec.callCount())
	rows, err = testOutputCommands(db).ListRunnableAfter(t.Context(), 0, 10)
	require.NoError(t, err)
	assert.Empty(t, rows)
}

func TestWorker_ActionCatalogChangeStillExecutesHeadlessCommand(t *testing.T) {
	t.Parallel()
	db := openTestPipelineDB(t)
	enqueueTestCommand(t, db, "review-action", "item-1", `{}`)

	actionsByID := fakeActionLister{"review-action": launchSessionAction("review-action", true)}
	exec := &fakeExecutor{}
	worker := NewWorker(testOutputCommands(db), actionsByID, NewDispatcher(map[string]Executor{"launch-session": exec}), 0, zerolog.Nop())
	actionsByID["review-action"] = launchSessionAction("review-action", false)
	worker.Tick(t.Context())
	waitForWorker(t, worker)
	assert.Equal(t, 1, exec.callCount())
}

func TestWorker_UnknownActionRecordsRetryableError(t *testing.T) {
	t.Parallel()
	db := openTestPipelineDB(t)
	enqueueTestCommand(t, db, "does-not-exist", "item-1", `{}`)

	exec := &fakeExecutor{}
	recorder := &fakeJobRecorder{}
	worker := NewWorker(testOutputCommands(db), fakeActionLister{}, NewDispatcher(map[string]Executor{"launch-session": exec}), 0, zerolog.Nop())
	worker.SetJobRecorder(recorder)
	worker.retryDelay = 0
	worker.Tick(t.Context())
	waitForWorker(t, worker)
	assert.Equal(t, 0, exec.callCount())
	assert.Equal(t, []string{"Begin", "Running"}, recorder.calls)
	rows, err := testOutputCommands(db).ListRunnableAfter(t.Context(), 0, 10)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, int64(1), rows[0].Attempts)
	assert.Contains(t, rows[0].LastError, "unknown action")

	for range MaxOutputCommandAttempts - 1 {
		worker.Tick(t.Context())
		waitForWorker(t, worker)
	}
	assert.Equal(t, []string{"Begin", "Running", "Fail"}, recorder.calls)
}

func TestWorker_FailingExecutor_KeepsOneRunningJobUntilTerminalFailure(t *testing.T) {
	t.Parallel()
	db := openTestPipelineDB(t)
	enqueueTestCommand(t, db, "spawn-review", "item-1", `{"title":"Fix bug"}`)

	exec := &fakeExecutor{err: fmt.Errorf("boom")}
	recorder := &fakeJobRecorder{}
	worker := NewWorker(testOutputCommands(db), fakeActionLister{"spawn-review": launchSessionAction("spawn-review", true)},
		NewDispatcher(map[string]Executor{"launch-session": exec}), 0, zerolog.Nop())
	worker.SetJobRecorder(recorder)
	worker.retryDelay = 0

	for attempt := range MaxOutputCommandAttempts - 1 {
		worker.Tick(t.Context())
		waitForWorker(t, worker)
		assert.Equal(t, []string{"Begin", "Running"}, recorder.calls, "retryable failures leave the existing job running")
		if attempt == 0 {
			worker = NewWorker(testOutputCommands(db), fakeActionLister{"spawn-review": launchSessionAction("spawn-review", true)},
				NewDispatcher(map[string]Executor{"launch-session": exec}), 0, zerolog.Nop())
			worker.SetJobRecorder(recorder)
			worker.retryDelay = 0
		}
	}
	worker.Tick(t.Context())
	waitForWorker(t, worker)
	assert.Equal(t, []string{"Begin", "Running", "Fail"}, recorder.calls)
	assert.Equal(t, "boom", recorder.reason)
}

func TestWorker_FailingExecutor_RetriesThenMarksFailed(t *testing.T) {
	t.Parallel()
	db := openTestPipelineDB(t)
	enqueueTestCommand(t, db, "spawn-review", "item-1", `{"title":"Fix bug"}`)

	exec := &fakeExecutor{err: fmt.Errorf("boom")}
	worker := NewWorker(testOutputCommands(db), fakeActionLister{"spawn-review": launchSessionAction("spawn-review", true)},
		NewDispatcher(map[string]Executor{"launch-session": exec}), 0, zerolog.Nop())
	worker.retryDelay = 0

	for i := range MaxOutputCommandAttempts - 1 {
		worker.Tick(t.Context())
		waitForWorker(t, worker)
		rows, err := testOutputCommands(db).ListRunnableAfter(t.Context(), 0, 10)
		require.NoError(t, err)
		require.Len(t, rows, 1, "still pending before the retry cap is reached (attempt %d)", i+1)
		assert.Equal(t, int64(i+1), rows[0].Attempts)
	}

	// One more failing tick reaches the cap and marks the command failed.
	worker.Tick(t.Context())
	waitForWorker(t, worker)
	rows, err := testOutputCommands(db).ListRunnableAfter(t.Context(), 0, 10)
	require.NoError(t, err)
	assert.Empty(t, rows, "command is no longer runnable once marked failed")

	var status string
	var attempts int64
	require.NoError(t, db.Conn().QueryRowContext(t.Context(),
		`SELECT status, attempts FROM output_command WHERE action_id = ? AND key = ?`,
		"spawn-review", "item-1",
	).Scan(&status, &attempts))
	assert.Equal(t, "failed", status)
	assert.Equal(t, int64(MaxOutputCommandAttempts), attempts)

	assert.Equal(t, MaxOutputCommandAttempts, exec.callCount(), "the executor was invoked once per attempt")
}

func TestWorker_ConfirmFailureReturnsPersistedDiagnostics(t *testing.T) {
	db := openTestPipelineDB(t)
	exec := &fakeExecutor{err: fmt.Errorf("boom"), result: ExecutionResult{Attempted: true, Log: ExecutionLog{Stdout: "partial output", Stderr: "failure output"}}}
	recorder := &fakeJobRecorder{}
	worker := NewWorker(testOutputCommands(db), fakeActionLister{"review-action": launchSessionAction("review-action", false)}, NewDispatcher(map[string]Executor{"launch-session": exec}), 0, zerolog.Nop())
	worker.SetJobRecorder(recorder)
	view, err := worker.Confirm(t.Context(), "review-action", "item-1", []byte(`{"title":"Fix bug"}`), models.ItemRef{}, ActionInvocationInput{})
	require.NoError(t, err)
	assert.Equal(t, "running", view.Status)
	waitForWorker(t, worker)
	view = worker.view(t.Context(), view.CommandID)
	assert.Equal(t, "failed", view.Status)
	assert.Equal(t, "boom", view.Error)
	assert.Equal(t, "partial output", view.Stdout)
	assert.Equal(t, "failure output", view.Stderr)
	assert.Equal(t, []string{"Begin", "Running", "Fail"}, recorder.calls)
	assert.Equal(t, "boom", recorder.reason)
}

func TestWorker_DoesNotRetryInterruptedInteractiveCommandAfterReopen(t *testing.T) {
	dir := t.TempDir()
	db, err := queries.Open(t.Context(), dir, queries.DefaultOpenOptions())
	require.NoError(t, err)
	enqueueTestCommand(t, db, "review-action", "item-1", `{"title":"Fix bug"}`)
	_, created, err := testOutputCommands(db).Confirm(t.Context(), "review-action", "item-1", []byte(`{}`), models.ItemRef{}, "claim")
	require.NoError(t, err)
	require.True(t, created)
	require.NoError(t, db.Close())

	reopened, err := queries.Open(t.Context(), dir, queries.DefaultOpenOptions())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, reopened.Close()) })
	exec := &fakeExecutor{}
	worker := NewWorker(testOutputCommands(reopened), fakeActionLister{"review-action": launchSessionAction("review-action", false)}, NewDispatcher(map[string]Executor{"launch-session": exec}), 0, zerolog.Nop())
	worker.Tick(t.Context())

	assert.Zero(t, exec.callCount(), "recovery must never retry an interactive command without its input")
	row, err := testOutputCommands(reopened).Get(t.Context(), 1)
	require.NoError(t, err)
	assert.Equal(t, "failed", row.Status)
	assert.Contains(t, row.LastError, "interrupted")
}

func TestWorker_BoundsExecutorDiagnosticsBeforePersistence(t *testing.T) {
	db := openTestPipelineDB(t)
	noisy := strings.Repeat("x", maxExecutionStreamBytes+1)
	exec := &fakeExecutor{result: ExecutionResult{Log: ExecutionLog{Stdout: noisy, Stderr: noisy}}}
	worker := NewWorker(testOutputCommands(db), fakeActionLister{"review-action": launchSessionAction("review-action", false)}, NewDispatcher(map[string]Executor{"launch-session": exec}), 0, zerolog.Nop())
	view, err := worker.Confirm(t.Context(), "review-action", "item-1", []byte(`{"title":"Fix bug"}`), models.ItemRef{}, ActionInvocationInput{})
	require.NoError(t, err)
	waitForWorker(t, worker)
	view = worker.view(t.Context(), view.CommandID)
	assert.Len(t, view.Stdout, maxExecutionStreamBytes)
	assert.Len(t, view.Stderr, maxExecutionStreamBytes)
	assert.True(t, strings.HasSuffix(view.Stdout, truncatedStreamMarker))
	assert.True(t, strings.HasSuffix(view.Stderr, truncatedStreamMarker))
}

func TestWorker_BadPayload_FailsWithoutCallingExecutor(t *testing.T) {
	t.Parallel()
	db := openTestPipelineDB(t)
	enqueueTestCommand(t, db, "spawn-review", "item-1", `not-json`)

	exec := &fakeExecutor{}
	worker := NewWorker(testOutputCommands(db), fakeActionLister{"spawn-review": launchSessionAction("spawn-review", true)},
		NewDispatcher(map[string]Executor{"launch-session": exec}), 0, zerolog.Nop())

	worker.Tick(t.Context())
	waitForWorker(t, worker)

	assert.Equal(t, 0, exec.callCount())
	rows, err := testOutputCommands(db).ListRunnableAfter(t.Context(), 0, 10)
	require.NoError(t, err)
	require.Len(t, rows, 1, "still retryable, not silently dropped")
	assert.Equal(t, int64(1), rows[0].Attempts)
}

func TestWorker_LongManualActionDoesNotBlockAutomaticDispatch(t *testing.T) {
	db := openTestPipelineDB(t)
	blocker := &blockingExecutor{started: make(chan OutputData, 1), release: make(chan struct{})}
	quick := &fakeExecutor{result: ExecutionResult{Attempted: true}}
	catalog := fakeActionLister{
		"manual": launchSessionAction("manual", false),
		"auto": {
			ID: "auto", Label: "Automatic", Type: "publish-message",
			Config: &actions.PublishMessageConfig{MessageTemplate: "done", Topic: "test"},
		},
	}
	worker := NewWorker(testOutputCommands(db), catalog, NewDispatcher(map[string]Executor{
		"launch-session":  blocker,
		"publish-message": quick,
	}), 0, zerolog.Nop())

	manual, err := worker.Confirm(t.Context(), "manual", "manual-item", []byte(`{}`), models.ItemRef{}, ActionInvocationInput{})
	require.NoError(t, err)
	assert.Equal(t, "running", manual.Status)
	select {
	case <-blocker.started:
	case <-time.After(time.Second):
		t.Fatal("manual action did not start")
	}

	enqueueTestCommand(t, db, "auto", "automatic-item", `{}`)
	worker.Tick(t.Context())
	require.Eventually(t, func() bool { return quick.callCount() == 1 }, time.Second, time.Millisecond)
	row, err := testOutputCommands(db).Get(t.Context(), manual.CommandID)
	require.NoError(t, err)
	assert.Equal(t, "running", row.Status)

	close(blocker.release)
	waitForWorker(t, worker)
}

func TestWorker_CancelStopsRunningActionWithoutRetry(t *testing.T) {
	db := openTestPipelineDB(t)
	blocker := &blockingExecutor{started: make(chan OutputData, 1), release: make(chan struct{})}
	worker := NewWorker(testOutputCommands(db), fakeActionLister{"manual": launchSessionAction("manual", false)},
		NewDispatcher(map[string]Executor{"launch-session": blocker}), 0, zerolog.Nop())

	view, err := worker.Confirm(t.Context(), "manual", "item-1", []byte(`{}`), models.ItemRef{}, ActionInvocationInput{})
	require.NoError(t, err)
	<-blocker.started
	require.True(t, worker.Cancel(view.CommandID))
	waitForWorker(t, worker)

	row, err := testOutputCommands(db).Get(t.Context(), view.CommandID)
	require.NoError(t, err)
	assert.Equal(t, "cancelled", row.Status)
	assert.Equal(t, int64(1), row.Attempts)
}

func TestDispatcher_UnknownType_IsError(t *testing.T) {
	t.Parallel()
	d := NewDispatcher(map[string]Executor{})
	_, err := d.Execute(t.Context(), actions.Action{ID: "x", Type: "no-such-type"}, OutputData{}, ActionInvocationInput{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no-such-type")
}
