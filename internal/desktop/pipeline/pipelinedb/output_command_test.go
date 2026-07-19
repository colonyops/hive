package pipelinedb

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func enqueueTestCommand(t *testing.T, db *DB, actionID, key string) {
	t.Helper()
	require.NoError(t, db.CommitBatch(context.Background(), CommitBatch{
		Consumer:   "flow-" + actionID + "-" + key,
		UpToOffset: 1,
		Outputs: []Output{
			{
				Sink:    Sink{Kind: SinkKindAction, TargetID: actionID},
				Key:     key,
				Payload: []byte(`{"v":1}`),
			},
		},
	}))
}

func TestListPendingOutputCommands_ReturnsOldestFirst(t *testing.T) {
	database := openTestDB(t)
	ctx := context.Background()

	enqueueTestCommand(t, database, "action-a", "k1")
	enqueueTestCommand(t, database, "action-a", "k2")

	rows, err := database.ListPendingOutputCommands(ctx, 10)
	require.NoError(t, err)
	require.Len(t, rows, 2)
	assert.Equal(t, "k1", rows[0].Key)
	assert.Equal(t, "k2", rows[1].Key)
	assert.Equal(t, "pending", rows[0].Status)
	assert.Equal(t, int64(0), rows[0].Attempts)
	assert.False(t, rows[0].LastError.Valid)
}

func TestListPendingOutputCommands_RespectsLimit(t *testing.T) {
	database := openTestDB(t)
	ctx := context.Background()

	enqueueTestCommand(t, database, "action-a", "k1")
	enqueueTestCommand(t, database, "action-a", "k2")

	rows, err := database.ListPendingOutputCommands(ctx, 1)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, "k1", rows[0].Key)
}

func TestMarkOutputCommandDone_ExcludesFromPending(t *testing.T) {
	database := openTestDB(t)
	ctx := context.Background()

	enqueueTestCommand(t, database, "action-a", "k1")
	rows, err := database.ListPendingOutputCommands(ctx, 10)
	require.NoError(t, err)
	require.Len(t, rows, 1)

	require.NoError(t, database.MarkOutputCommandDone(ctx, rows[0].ID))

	rows, err = database.ListPendingOutputCommands(ctx, 10)
	require.NoError(t, err)
	assert.Empty(t, rows)
}

func TestRetryOutputCommand_IncrementsAttemptsAndStaysPending(t *testing.T) {
	database := openTestDB(t)
	ctx := context.Background()

	enqueueTestCommand(t, database, "action-a", "k1")
	rows, err := database.ListPendingOutputCommands(ctx, 10)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	id := rows[0].ID

	require.NoError(t, database.RetryOutputCommand(ctx, id, "boom"))

	rows, err = database.ListPendingOutputCommands(ctx, 10)
	require.NoError(t, err)
	require.Len(t, rows, 1, "retried command stays pending")
	assert.Equal(t, int64(1), rows[0].Attempts)
	require.True(t, rows[0].LastError.Valid)
	assert.Equal(t, "boom", rows[0].LastError.String)

	require.NoError(t, database.RetryOutputCommand(ctx, id, "boom again"))
	rows, err = database.ListPendingOutputCommands(ctx, 10)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, int64(2), rows[0].Attempts)
}

func TestMarkOutputCommandFailed_ExcludesFromPending(t *testing.T) {
	database := openTestDB(t)
	ctx := context.Background()

	enqueueTestCommand(t, database, "action-a", "k1")
	rows, err := database.ListPendingOutputCommands(ctx, 10)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	id := rows[0].ID

	require.NoError(t, database.MarkOutputCommandFailed(ctx, id, "gave up"))

	rows, err = database.ListPendingOutputCommands(ctx, 10)
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
