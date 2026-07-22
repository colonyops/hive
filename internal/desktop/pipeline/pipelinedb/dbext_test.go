package pipelinedb

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/internal/data/migrate"
)

func openTestDB(t *testing.T) *DB {
	t.Helper()
	database, err := Open(t.TempDir(), DefaultOpenOptions())
	require.NoError(t, err)
	t.Cleanup(func() { _ = database.Close() })
	return database
}

func TestOpen_FreshDB_AppliesBaseline(t *testing.T) {
	database := openTestDB(t)
	ctx := context.Background()

	for _, table := range []string{
		"activity_event", "consumer_offset", "event_log", "feed_membership_claim",
		"inbox_event", "inbox_item", "job", "node_run", "output_command", "source_head",
	} {
		_, err := database.Conn().ExecContext(ctx, "SELECT 1 FROM "+table+" LIMIT 0")
		require.NoError(t, err, "%s table should exist", table)
	}

	sub, err := migrationsSub()
	require.NoError(t, err)
	migrations, err := migrate.Load(sub)
	require.NoError(t, err)
	require.Len(t, migrations, 1)

	applied, err := migrate.AppliedVersions(ctx, database.Conn())
	require.NoError(t, err)
	assert.Equal(t, map[int]bool{1: true}, applied)
}

func TestOpen_RecoversInterruptedRunningCommandWithoutRetry(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()
	first, err := Open(dir, DefaultOpenOptions())
	require.NoError(t, err)
	enqueueTestCommand(t, first, "review", "item-1")
	command, created, err := first.ConfirmOutputCommand(ctx, "review", "item-1", []byte(`{}`))
	require.NoError(t, err)
	require.True(t, created)
	job, err := first.InsertJob(ctx, JobRecord{CreatedAt: 1, UpdatedAt: 1, Status: "queued", Label: "Review"})
	require.NoError(t, err)
	_, err = first.SetJobRunning(ctx, job.ID, 2, "Running…", command.ID)
	require.NoError(t, err)
	_, err = first.InsertJob(ctx, JobRecord{CreatedAt: 3, UpdatedAt: 3, Status: "queued", Label: "Interrupted before link"})
	require.NoError(t, err)
	require.NoError(t, first.Close())

	reopened, err := Open(dir, DefaultOpenOptions())
	require.NoError(t, err)
	t.Cleanup(func() { _ = reopened.Close() })
	row, err := reopened.OutputCommand(ctx, 1)
	require.NoError(t, err)
	assert.Equal(t, "failed", row.Status)
	assert.Equal(t, int64(1), row.Attempts)
	assert.Contains(t, row.LastError.String, "interrupted")
	rows, err := reopened.ListRunnableOutputCommands(ctx, 10)
	require.NoError(t, err)
	assert.Empty(t, rows)
	jobs, err := reopened.ListJobs(ctx, 0, 10)
	require.NoError(t, err)
	require.Len(t, jobs, 2)
	for _, job := range jobs {
		assert.Equal(t, "failed", job.Status)
		assert.Equal(t, "Failed", job.Step)
		assert.Contains(t, job.Error, "interrupted")
	}
}

func TestOpen_Idempotent(t *testing.T) {
	dir := t.TempDir()

	first, err := Open(dir, DefaultOpenOptions())
	require.NoError(t, err)

	ctx := context.Background()
	appliedFirst, err := migrate.AppliedVersions(ctx, first.Conn())
	require.NoError(t, err)
	require.NoError(t, first.Close())

	second, err := Open(dir, DefaultOpenOptions())
	require.NoError(t, err, "second Open on the same dir should succeed")
	t.Cleanup(func() { _ = second.Close() })

	appliedSecond, err := migrate.AppliedVersions(ctx, second.Conn())
	require.NoError(t, err)
	assert.Equal(t, appliedFirst, appliedSecond, "applied migration set should be unchanged")
}

func TestOpen_CreatesMissingParentDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "does", "not", "exist")

	database, err := Open(dir, DefaultOpenOptions())
	require.NoError(t, err, "Open should create the target directory when it does not exist")
	t.Cleanup(func() { _ = database.Close() })

	ctx := context.Background()
	offset, err := database.Append(ctx, "source:test", "key-1", []byte(`{"v":1}`))
	require.NoError(t, err)

	msgs, next, err := database.ReadFrom(ctx, 0, 10)
	require.NoError(t, err)
	require.Len(t, msgs, 1)
	assert.Equal(t, "key-1", msgs[0].Key)
	assert.Equal(t, offset, next)
}
