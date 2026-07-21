package pipeline

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/internal/desktop/pipeline/actions"
	"github.com/colonyops/hive/internal/desktop/pipeline/pipelinedb"
)

// enqueueTestCommand enqueues one output_command row via CommitBatch (the
// only production path that ever writes one), so tests exercise the real
// dedup/enqueue behavior rather than inserting rows by hand.
func enqueueTestCommand(t *testing.T, db *pipelinedb.DB, actionID, key, payload string) {
	t.Helper()
	require.NoError(t, db.CommitBatch(context.Background(), pipelinedb.CommitBatch{
		Consumer:   "test-consumer-" + actionID + "-" + key,
		UpToOffset: "1",
		Outputs: []pipelinedb.Output{
			{
				Sink:    pipelinedb.Sink{Kind: pipelinedb.SinkKindAction, TargetID: actionID},
				Key:     key,
				Payload: []byte(payload),
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
	worker := NewWorker(db, fakeActionLister{"spawn-review": launchSessionAction("spawn-review", true)},
		NewDispatcher(map[string]Executor{"launch-session": exec}), 0, zerolog.Nop())

	worker.Tick(t.Context())

	require.Equal(t, 1, exec.callCount())
	assert.Equal(t, "item-1", exec.calls[0].Key)
	assert.Equal(t, "Fix bug", exec.calls[0].Payload["title"])

	rows, err := db.ListRunnableOutputCommands(t.Context(), 10)
	require.NoError(t, err)
	assert.Empty(t, rows, "a successfully executed command is no longer runnable")
}

func TestWorker_HeadlessActionExecutesAndConsumesQueue(t *testing.T) {
	t.Parallel()
	db := openTestPipelineDB(t)
	enqueueTestCommand(t, db, "headless-action", "item-1", `{"title":"Fix bug"}`)

	exec := &fakeExecutor{}
	worker := NewWorker(db, fakeActionLister{"headless-action": launchSessionAction("headless-action", false)},
		NewDispatcher(map[string]Executor{"launch-session": exec}), 0, zerolog.Nop())
	worker.Tick(t.Context())

	assert.Equal(t, 1, exec.callCount())
	rows, err := db.ListRunnableOutputCommands(t.Context(), 10)
	require.NoError(t, err)
	assert.Empty(t, rows)
	var status string
	require.NoError(t, db.Conn().QueryRowContext(t.Context(), `SELECT status FROM output_command WHERE action_id = ? AND key = ?`, "headless-action", "item-1").Scan(&status))
	assert.Equal(t, "done", status)
}

func TestWorker_ConfirmCannotReplayAlreadyCompletedDeployedFlowCommand(t *testing.T) {
	t.Parallel()
	db := openTestPipelineDB(t)
	enqueueTestCommand(t, db, "review-action", "item-1", `{"title":"Fix bug"}`)

	exec := &fakeExecutor{}
	worker := NewWorker(db, fakeActionLister{"review-action": launchSessionAction("review-action", false)},
		NewDispatcher(map[string]Executor{"launch-session": exec}), 0, zerolog.Nop())
	worker.Tick(t.Context())

	_, err := worker.Confirm(t.Context(), "review-action", "item-1", []byte(`{"title":"changed"}`), ActionInvocationInput{})
	require.Error(t, err, "the worker already consumed the queued command")
	assert.Equal(t, 1, exec.callCount())
	assert.Equal(t, "Fix bug", exec.calls[0].Payload["title"], "the worker preserves the queued command payload")

	var status string
	require.NoError(t, db.Conn().QueryRowContext(t.Context(),
		`SELECT status FROM output_command WHERE action_id = ? AND key = ?`,
		"review-action", "item-1",
	).Scan(&status))
	assert.Equal(t, "done", status)
	_, err = worker.Confirm(t.Context(), "review-action", "item-1", []byte(`{}`), ActionInvocationInput{})
	assert.Error(t, err, "completed actions remain deduped")
}

func TestWorker_ConfirmRecordsJobLifecycleWithoutReplay(t *testing.T) {
	t.Parallel()
	db := openTestPipelineDB(t)
	exec := &fakeExecutor{}
	recorder := &fakeJobRecorder{}
	worker := NewWorker(db, fakeActionLister{"review-action": launchSessionAction("review-action", false)},
		NewDispatcher(map[string]Executor{"launch-session": exec}), 0, zerolog.Nop())
	worker.SetJobRecorder(recorder)

	_, err := worker.Confirm(t.Context(), "review-action", "item-1", []byte(`{"title":"Fix bug"}`), ActionInvocationInput{})
	require.NoError(t, err)
	assert.Equal(t, []string{"Begin", "Running", "Done"}, recorder.calls)
	assert.Equal(t, "Test action", recorder.label)
	assert.Equal(t, "review-action", recorder.actionID)
	assert.Equal(t, "item-1", recorder.target)
	assert.Positive(t, recorder.commandID)

	recorder.calls = nil
	_, err = worker.Confirm(t.Context(), "review-action", "item-1", []byte(`{}`), ActionInvocationInput{})
	require.Error(t, err)
	assert.Empty(t, recorder.calls, "idempotent replay must not create a phantom job")
}

func TestWorker_ConfirmCreatesCommandWhenNoActionNodeProducedOne(t *testing.T) {
	t.Parallel()
	db := openTestPipelineDB(t)
	exec := &fakeExecutor{}
	worker := NewWorker(db, fakeActionLister{"review-action": launchSessionAction("review-action", false)},
		NewDispatcher(map[string]Executor{"launch-session": exec}), 0, zerolog.Nop())

	_, err := worker.Confirm(t.Context(), "review-action", "item-1", []byte(`{"title":"Fix bug"}`), ActionInvocationInput{})
	require.NoError(t, err)
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
	worker := NewWorker(db, fakeActionLister{"headless-action": launchSessionAction("headless-action", true)},
		NewDispatcher(map[string]Executor{"launch-session": exec}), 0, zerolog.Nop())
	worker.Tick(t.Context())
	assert.Equal(t, DefaultOutputWorkerBatch, exec.callCount(), "one tick is bounded")
	rows, err := db.ListRunnableOutputCommands(t.Context(), 10)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, "item-after-bound", rows[0].Key)

	worker.Tick(t.Context())
	assert.Equal(t, DefaultOutputWorkerBatch+1, exec.callCount())
	rows, err = db.ListRunnableOutputCommands(t.Context(), 10)
	require.NoError(t, err)
	assert.Empty(t, rows)
}

func TestWorker_ActionCatalogChangeStillExecutesHeadlessCommand(t *testing.T) {
	t.Parallel()
	db := openTestPipelineDB(t)
	enqueueTestCommand(t, db, "review-action", "item-1", `{}`)

	actionsByID := fakeActionLister{"review-action": launchSessionAction("review-action", true)}
	exec := &fakeExecutor{}
	worker := NewWorker(db, actionsByID, NewDispatcher(map[string]Executor{"launch-session": exec}), 0, zerolog.Nop())
	actionsByID["review-action"] = launchSessionAction("review-action", false)
	worker.Tick(t.Context())
	assert.Equal(t, 1, exec.callCount())
}

func TestWorker_UnknownActionRecordsRetryableError(t *testing.T) {
	t.Parallel()
	db := openTestPipelineDB(t)
	enqueueTestCommand(t, db, "does-not-exist", "item-1", `{}`)

	exec := &fakeExecutor{}
	recorder := &fakeJobRecorder{}
	worker := NewWorker(db, fakeActionLister{}, NewDispatcher(map[string]Executor{"launch-session": exec}), 0, zerolog.Nop())
	worker.SetJobRecorder(recorder)
	worker.Tick(t.Context())
	assert.Equal(t, 0, exec.callCount())
	assert.Equal(t, []string{"Begin", "Running"}, recorder.calls)
	rows, err := db.ListRunnableOutputCommands(t.Context(), 10)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, int64(1), rows[0].Attempts)
	assert.Contains(t, rows[0].LastError.String, "unknown action")

	for range MaxOutputCommandAttempts - 1 {
		worker.Tick(t.Context())
	}
	assert.Equal(t, []string{"Begin", "Running", "Fail"}, recorder.calls)
}

func TestWorker_FailingExecutor_KeepsOneRunningJobUntilTerminalFailure(t *testing.T) {
	t.Parallel()
	db := openTestPipelineDB(t)
	enqueueTestCommand(t, db, "spawn-review", "item-1", `{"title":"Fix bug"}`)

	exec := &fakeExecutor{err: fmt.Errorf("boom")}
	recorder := &fakeJobRecorder{}
	worker := NewWorker(db, fakeActionLister{"spawn-review": launchSessionAction("spawn-review", true)},
		NewDispatcher(map[string]Executor{"launch-session": exec}), 0, zerolog.Nop())
	worker.SetJobRecorder(recorder)

	for attempt := range MaxOutputCommandAttempts - 1 {
		worker.Tick(t.Context())
		assert.Equal(t, []string{"Begin", "Running"}, recorder.calls, "retryable failures leave the existing job running")
		if attempt == 0 {
			worker = NewWorker(db, fakeActionLister{"spawn-review": launchSessionAction("spawn-review", true)},
				NewDispatcher(map[string]Executor{"launch-session": exec}), 0, zerolog.Nop())
			worker.SetJobRecorder(recorder)
		}
	}
	worker.Tick(t.Context())
	assert.Equal(t, []string{"Begin", "Running", "Fail"}, recorder.calls)
	assert.Equal(t, "boom", recorder.reason)
}

func TestWorker_FailingExecutor_RetriesThenMarksFailed(t *testing.T) {
	t.Parallel()
	db := openTestPipelineDB(t)
	enqueueTestCommand(t, db, "spawn-review", "item-1", `{"title":"Fix bug"}`)

	exec := &fakeExecutor{err: fmt.Errorf("boom")}
	worker := NewWorker(db, fakeActionLister{"spawn-review": launchSessionAction("spawn-review", true)},
		NewDispatcher(map[string]Executor{"launch-session": exec}), 0, zerolog.Nop())

	for i := 0; i < MaxOutputCommandAttempts-1; i++ {
		worker.Tick(t.Context())
		rows, err := db.ListRunnableOutputCommands(t.Context(), 10)
		require.NoError(t, err)
		require.Len(t, rows, 1, "still pending before the retry cap is reached (attempt %d)", i+1)
		assert.Equal(t, int64(i+1), rows[0].Attempts)
	}

	// One more failing tick reaches the cap and marks the command failed.
	worker.Tick(t.Context())
	rows, err := db.ListRunnableOutputCommands(t.Context(), 10)
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
	worker := NewWorker(db, fakeActionLister{"review-action": launchSessionAction("review-action", false)}, NewDispatcher(map[string]Executor{"launch-session": exec}), 0, zerolog.Nop())
	view, err := worker.Confirm(t.Context(), "review-action", "item-1", []byte(`{"title":"Fix bug"}`), ActionInvocationInput{})
	require.NoError(t, err, "an attempted side-effect failure is returned as a persisted view")
	assert.Equal(t, "failed", view.Status)
	assert.Equal(t, "boom", view.Error)
	assert.Equal(t, "partial output", view.Stdout)
	assert.Equal(t, "failure output", view.Stderr)
}

func TestWorker_DoesNotRetryInterruptedInteractiveCommandAfterReopen(t *testing.T) {
	dir := t.TempDir()
	db, err := pipelinedb.Open(dir, pipelinedb.DefaultOpenOptions())
	require.NoError(t, err)
	enqueueTestCommand(t, db, "review-action", "item-1", `{"title":"Fix bug"}`)
	_, created, err := db.ConfirmOutputCommand(t.Context(), "review-action", "item-1", []byte(`{}`))
	require.NoError(t, err)
	require.True(t, created)
	require.NoError(t, db.Close())

	reopened, err := pipelinedb.Open(dir, pipelinedb.DefaultOpenOptions())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, reopened.Close()) })
	exec := &fakeExecutor{}
	worker := NewWorker(reopened, fakeActionLister{"review-action": launchSessionAction("review-action", false)}, NewDispatcher(map[string]Executor{"launch-session": exec}), 0, zerolog.Nop())
	worker.Tick(t.Context())

	assert.Zero(t, exec.callCount(), "recovery must never retry an interactive command without its input")
	row, err := reopened.OutputCommand(t.Context(), 1)
	require.NoError(t, err)
	assert.Equal(t, "failed", row.Status)
	assert.Contains(t, row.LastError.String, "interrupted")
}

func TestWorker_BoundsExecutorDiagnosticsBeforePersistence(t *testing.T) {
	db := openTestPipelineDB(t)
	noisy := strings.Repeat("x", maxExecutionStreamBytes+1)
	exec := &fakeExecutor{result: ExecutionResult{Log: ExecutionLog{Stdout: noisy, Stderr: noisy}}}
	worker := NewWorker(db, fakeActionLister{"review-action": launchSessionAction("review-action", false)}, NewDispatcher(map[string]Executor{"launch-session": exec}), 0, zerolog.Nop())
	view, err := worker.Confirm(t.Context(), "review-action", "item-1", []byte(`{"title":"Fix bug"}`), ActionInvocationInput{})
	require.NoError(t, err)
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
	worker := NewWorker(db, fakeActionLister{"spawn-review": launchSessionAction("spawn-review", true)},
		NewDispatcher(map[string]Executor{"launch-session": exec}), 0, zerolog.Nop())

	worker.Tick(t.Context())

	assert.Equal(t, 0, exec.callCount())
	rows, err := db.ListRunnableOutputCommands(t.Context(), 10)
	require.NoError(t, err)
	require.Len(t, rows, 1, "still retryable, not silently dropped")
	assert.Equal(t, int64(1), rows[0].Attempts)
}

func TestDispatcher_UnknownType_IsError(t *testing.T) {
	t.Parallel()
	d := NewDispatcher(map[string]Executor{})
	_, err := d.Execute(t.Context(), actions.Action{ID: "x", Type: "no-such-type"}, OutputData{}, ActionInvocationInput{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no-such-type")
}
