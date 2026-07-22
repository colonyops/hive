package pipelinedb

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func enqueueTestCommand(t *testing.T, db *DB, actionID, key string) {
	t.Helper()
	require.NoError(t, db.CommitBatch(context.Background(), CommitBatch{
		Consumer:   "flow-" + actionID + "-" + key,
		UpToOffset: "1",
		Outputs: []Output{
			{
				Sink:          Sink{Kind: SinkKindAction, TargetID: actionID},
				OccurrenceKey: key,
				Payload:       []byte(`{"v":1}`),
			},
		},
	}))
}

func TestListRunnableOutputCommands_ReturnsOldestIDFirst(t *testing.T) {
	database := openTestDB(t)
	ctx := context.Background()

	enqueueTestCommand(t, database, "action-a", "k1")
	enqueueTestCommand(t, database, "action-a", "k2")

	rows, err := database.ListRunnableOutputCommands(ctx, 10)
	require.NoError(t, err)
	require.Len(t, rows, 2)
	assert.Equal(t, "k1", rows[0].Key)
	assert.Equal(t, "k2", rows[1].Key)
	assert.Equal(t, "pending", rows[0].Status)
	assert.Equal(t, int64(0), rows[0].Attempts)
	assert.False(t, rows[0].LastError.Valid)
}

func TestListRunnableOutputCommands_RespectsLimit(t *testing.T) {
	database := openTestDB(t)
	ctx := context.Background()

	enqueueTestCommand(t, database, "action-a", "k1")
	enqueueTestCommand(t, database, "action-a", "k2")

	rows, err := database.ListRunnableOutputCommands(ctx, 1)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, "k1", rows[0].Key)
}

func TestConfirmOutputCommandDeduplicatesExistingCommand(t *testing.T) {
	database := openTestDB(t)
	ctx := context.Background()

	enqueueTestCommand(t, database, "action-a", "k1")
	row, created, err := database.ConfirmOutputCommand(ctx, "action-a", "k1", []byte(`{"v":2}`))
	require.NoError(t, err)
	assert.True(t, created, "confirmation atomically claims the pending command")
	assert.Equal(t, "running", row.Status)
	assert.JSONEq(t, `{"v":1}`, string(row.Payload), "the queued payload remains authoritative")
	_, created, err = database.ConfirmOutputCommand(ctx, "action-a", "k1", []byte(`{"v":3}`))
	require.NoError(t, err)
	assert.False(t, created, "a running command cannot be claimed twice")
}

func TestMarkOutputCommandDone_ExcludesFromRunnable(t *testing.T) {
	database := openTestDB(t)
	ctx := context.Background()

	enqueueTestCommand(t, database, "action-a", "k1")
	rows, err := database.ListRunnableOutputCommands(ctx, 10)
	require.NoError(t, err)
	require.Len(t, rows, 1)

	require.NoError(t, database.MarkOutputCommandDone(ctx, rows[0].ID))

	rows, err = database.ListRunnableOutputCommands(ctx, 10)
	require.NoError(t, err)
	assert.Empty(t, rows)
}

func TestRetryOutputCommand_IncrementsAttemptsAndStaysRunnable(t *testing.T) {
	database := openTestDB(t)
	ctx := context.Background()

	enqueueTestCommand(t, database, "action-a", "k1")
	rows, err := database.ListRunnableOutputCommands(ctx, 10)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	id := rows[0].ID

	require.NoError(t, database.RetryOutputCommand(ctx, id, "boom"))

	rows, err = database.ListRunnableOutputCommands(ctx, 10)
	require.NoError(t, err)
	require.Len(t, rows, 1, "retried command stays runnable")
	assert.Equal(t, int64(1), rows[0].Attempts)
	require.True(t, rows[0].LastError.Valid)
	assert.Equal(t, "boom", rows[0].LastError.String)

	require.NoError(t, database.RetryOutputCommand(ctx, id, "boom again"))
	rows, err = database.ListRunnableOutputCommands(ctx, 10)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, int64(2), rows[0].Attempts)
}

func TestMarkOutputCommandDoneClearsPreviousFailure(t *testing.T) {
	database := openTestDB(t)
	enqueueTestCommand(t, database, "action-a", "k1")
	rows, err := database.ListRunnableOutputCommands(t.Context(), 1)
	require.NoError(t, err)
	require.NoError(t, database.RetryOutputCommand(t.Context(), rows[0].ID, "first failure", "old stdout", "old stderr"))
	require.NoError(t, database.MarkOutputCommandDone(t.Context(), rows[0].ID, `{"message":{"topic":"agent.inbox"}}`, "new stdout", ""))

	row, err := database.OutputCommand(t.Context(), rows[0].ID)
	require.NoError(t, err)
	assert.Equal(t, "done", row.Status)
	assert.False(t, row.LastError.Valid, "successful retry must not retain stale failure")
	assert.JSONEq(t, `{"message":{"topic":"agent.inbox"}}`, row.ResultJson.String)
	assert.Equal(t, "new stdout", row.Stdout.String)
	assert.False(t, row.Stderr.Valid)
}

func TestOutputCommandPersistenceBoundsStreams(t *testing.T) {
	database := openTestDB(t)
	enqueueTestCommand(t, database, "action-a", "k1")
	rows, err := database.ListRunnableOutputCommands(t.Context(), 1)
	require.NoError(t, err)
	noisy := strings.Repeat("x", maxOutputCommandStreamBytes+1)
	require.NoError(t, database.MarkOutputCommandFailed(t.Context(), rows[0].ID, "failed", noisy, noisy))

	row, err := database.OutputCommand(t.Context(), rows[0].ID)
	require.NoError(t, err)
	assert.Len(t, row.Stdout.String, maxOutputCommandStreamBytes)
	assert.Len(t, row.Stderr.String, maxOutputCommandStreamBytes)
	assert.True(t, strings.HasSuffix(row.Stdout.String, outputCommandTruncatedMarker))
	assert.True(t, strings.HasSuffix(row.Stderr.String, outputCommandTruncatedMarker))
}

func TestExecutionResultAndLogsPersistAcrossReopenBeforeDone(t *testing.T) {
	dir := t.TempDir()
	database, err := Open(dir, DefaultOpenOptions())
	require.NoError(t, err)
	enqueueTestCommand(t, database, "action-a", "k1")
	rows, err := database.ListRunnableOutputCommands(t.Context(), 1)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.NoError(t, database.MarkOutputCommandDone(t.Context(), rows[0].ID, `{"message":{"topic":"agent.inbox"}}`, "stdout", "stderr"))
	require.NoError(t, database.Close())

	database, err = Open(dir, DefaultOpenOptions())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, database.Close()) })
	row, err := database.OutputCommand(t.Context(), rows[0].ID)
	require.NoError(t, err)
	assert.Equal(t, "done", row.Status)
	assert.JSONEq(t, `{"message":{"topic":"agent.inbox"}}`, row.ResultJson.String)
	assert.Equal(t, "stdout", row.Stdout.String)
	assert.Equal(t, "stderr", row.Stderr.String)
}

func TestMarkOutputCommandFailed_ExcludesFromRunnable(t *testing.T) {
	database := openTestDB(t)
	ctx := context.Background()

	enqueueTestCommand(t, database, "action-a", "k1")
	rows, err := database.ListRunnableOutputCommands(ctx, 10)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	id := rows[0].ID

	require.NoError(t, database.MarkOutputCommandFailed(ctx, id, "gave up"))

	rows, err = database.ListRunnableOutputCommands(ctx, 10)
	require.NoError(t, err)
	assert.Empty(t, rows)

	var status string
	var lastErr string
	require.NoError(t, database.Conn().QueryRowContext(ctx,
		`SELECT status, last_error FROM output_command WHERE id = ?`, id,
	).Scan(&status, &lastErr))
	assert.Equal(t, "failed", status)
	assert.Equal(t, "gave up", lastErr)
}
