package pipeline

import (
	"context"
	"fmt"
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

// fakeExecutor records every Execute call and returns a scripted error (or
// nil) each time, so tests can assert exactly what the worker dispatched.
type fakeExecutor struct {
	mu    sync.Mutex
	calls []OutputData
	err   error
}

func (f *fakeExecutor) Execute(_ context.Context, _ actions.Action, data OutputData) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, data)
	return f.err
}

func (f *fakeExecutor) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.calls)
}

func launchSessionAction(id string, autoApply bool) actions.Action {
	return actions.Action{
		ID:        id,
		Label:     "Test action",
		Type:      "launch-session",
		AutoApply: autoApply,
		Config:    &actions.LaunchSessionConfig{PromptTemplate: "Review {{ .Payload.title }}"},
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

	rows, err := db.ListPendingOutputCommands(t.Context(), 10)
	require.NoError(t, err)
	assert.Empty(t, rows, "a successfully executed command is no longer pending")
}

func TestWorker_NonAutoApplyAction_NeverExecuted(t *testing.T) {
	t.Parallel()
	db := openTestPipelineDB(t)
	enqueueTestCommand(t, db, "manual-action", "item-1", `{"title":"Fix bug"}`)

	exec := &fakeExecutor{}
	worker := NewWorker(db, fakeActionLister{"manual-action": launchSessionAction("manual-action", false)},
		NewDispatcher(map[string]Executor{"launch-session": exec}), 0, zerolog.Nop())

	worker.Tick(t.Context())

	assert.Equal(t, 0, exec.callCount(), "non-auto-apply actions must not be executed by the worker")

	rows, err := db.ListPendingOutputCommands(t.Context(), 10)
	require.NoError(t, err)
	require.Len(t, rows, 1, "the command stays pending, awaiting manual confirmation")
	assert.Equal(t, int64(0), rows[0].Attempts, "no execution attempt was made")
}

func TestWorker_UnknownAction_MarkedFailedImmediately(t *testing.T) {
	t.Parallel()
	db := openTestPipelineDB(t)
	enqueueTestCommand(t, db, "does-not-exist", "item-1", `{}`)

	exec := &fakeExecutor{}
	worker := NewWorker(db, fakeActionLister{},
		NewDispatcher(map[string]Executor{"launch-session": exec}), 0, zerolog.Nop())

	worker.Tick(t.Context())

	assert.Equal(t, 0, exec.callCount())
	rows, err := db.ListPendingOutputCommands(t.Context(), 10)
	require.NoError(t, err)
	assert.Empty(t, rows)
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
		rows, err := db.ListPendingOutputCommands(t.Context(), 10)
		require.NoError(t, err)
		require.Len(t, rows, 1, "still pending before the retry cap is reached (attempt %d)", i+1)
		assert.Equal(t, int64(i+1), rows[0].Attempts)
	}

	// One more failing tick reaches the cap and marks the command failed.
	worker.Tick(t.Context())
	rows, err := db.ListPendingOutputCommands(t.Context(), 10)
	require.NoError(t, err)
	assert.Empty(t, rows, "command is no longer pending once marked failed")

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

func TestWorker_BadPayload_FailsWithoutCallingExecutor(t *testing.T) {
	t.Parallel()
	db := openTestPipelineDB(t)
	enqueueTestCommand(t, db, "spawn-review", "item-1", `not-json`)

	exec := &fakeExecutor{}
	worker := NewWorker(db, fakeActionLister{"spawn-review": launchSessionAction("spawn-review", true)},
		NewDispatcher(map[string]Executor{"launch-session": exec}), 0, zerolog.Nop())

	worker.Tick(t.Context())

	assert.Equal(t, 0, exec.callCount())
	rows, err := db.ListPendingOutputCommands(t.Context(), 10)
	require.NoError(t, err)
	require.Len(t, rows, 1, "still retryable, not silently dropped")
	assert.Equal(t, int64(1), rows[0].Attempts)
}

func TestDispatcher_UnknownType_IsError(t *testing.T) {
	t.Parallel()
	d := NewDispatcher(map[string]Executor{})
	err := d.Execute(t.Context(), actions.Action{ID: "x", Type: "no-such-type"}, OutputData{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no-such-type")
}
