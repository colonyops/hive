package queries

import (
	"database/sql"
	"math"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/internal/data/migrate"
)

func openTestDB(t *testing.T) *DB {
	t.Helper()
	database, err := Open(t.Context(), t.TempDir(), DefaultOpenOptions())
	require.NoError(t, err)
	t.Cleanup(func() { _ = database.Close() })
	return database
}

func TestOpen_FreshDB_AppliesBaseline(t *testing.T) {
	database := openTestDB(t)
	ctx := t.Context()

	for _, table := range []string{
		"activity_event", "agent_workspace_session", "consumer_offset", "event_log", "feed_membership_claim",
		"inbox_event", "inbox_item", "item_session", "job", "node_kv", "node_run", "output_command",
		"item_chat", "schedule_cursor", "schedule_run", "source_head", "webhook_capture",
	} {
		_, err := database.Conn().ExecContext(ctx, "SELECT 1 FROM "+table+" LIMIT 0")
		require.NoError(t, err, "%s table should exist", table)
	}

	sub, err := migrationsSub()
	require.NoError(t, err)
	migrations, err := migrate.Load(sub)
	require.NoError(t, err)
	require.Len(t, migrations, 11)

	applied, err := migrate.AppliedVersions(ctx, database.Conn())
	require.NoError(t, err)
	assert.Equal(t, map[int]bool{1: true, 2: true, 3: true, 4: true, 5: true, 6: true, 7: true, 8: true, 9: true, 10: true, 11: true}, applied)
}

func TestMigration10PreservesExistingAgentWorkspaceTmuxNames(t *testing.T) {
	ctx := t.Context()
	conn, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "migration.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })

	sub, err := migrationsSub()
	require.NoError(t, err)
	migrations, err := migrate.Load(sub)
	require.NoError(t, err)
	require.NoError(t, migrate.EnsureTable(ctx, conn))
	require.NoError(t, migrate.Apply(ctx, conn, migrations[:9]))
	_, err = conn.ExecContext(ctx, `INSERT INTO agent_workspace_session
		(workspace, name, agent, agent_session_id, created_at, last_opened_at)
		VALUES ('demo', 'existing', 'claude', 'conversation', 1, 1)`)
	require.NoError(t, err)

	require.NoError(t, migrate.Apply(ctx, conn, migrations[9:]))
	var terminalID string
	require.NoError(t, conn.QueryRowContext(ctx, `SELECT terminal_id FROM agent_workspace_session WHERE id = 1`).Scan(&terminalID))
	assert.Equal(t, "1", terminalID, "the migrated tmux name remains agentws-1")
}

func TestOpen_RecoversInterruptedRunningCommandWithoutRetry(t *testing.T) {
	dir := t.TempDir()
	ctx := t.Context()
	first, err := Open(t.Context(), dir, DefaultOpenOptions())
	require.NoError(t, err)
	command, err := first.ConfirmOutputCommand(ctx, ConfirmOutputCommandParams{ActionID: "review", Key: "item-1", Payload: []byte(`{}`), CreatedAt: 1})
	require.NoError(t, err)
	job, err := first.InsertJob(ctx, InsertJobParams{CreatedAt: 1, UpdatedAt: 1, Status: "queued", Label: "Review"})
	require.NoError(t, err)
	_, err = first.SetJobRunning(ctx, SetJobRunningParams{
		UpdatedAt: 2, Status: "running", Step: "Running…", CommandID: sql.NullInt64{Int64: command.ID, Valid: true}, ID: job.ID,
	})
	require.NoError(t, err)
	_, err = first.InsertJob(ctx, InsertJobParams{CreatedAt: 3, UpdatedAt: 3, Status: "queued", Label: "Interrupted before link"})
	require.NoError(t, err)
	require.NoError(t, first.Close())

	reopened, err := Open(t.Context(), dir, DefaultOpenOptions())
	require.NoError(t, err)
	t.Cleanup(func() { _ = reopened.Close() })
	row, err := reopened.GetOutputCommand(ctx, command.ID)
	require.NoError(t, err)
	assert.Equal(t, "failed", row.Status)
	assert.Equal(t, int64(1), row.Attempts)
	assert.Contains(t, row.LastError.String, "interrupted")
	rows, err := reopened.ListRunnableOutputCommandsAfter(ctx, ListRunnableOutputCommandsAfterParams{ID: 0, Limit: 10})
	require.NoError(t, err)
	assert.Empty(t, rows)
	jobs, err := reopened.ListJobs(ctx, ListJobsParams{ID: math.MaxInt64, Limit: 10})
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

	first, err := Open(t.Context(), dir, DefaultOpenOptions())
	require.NoError(t, err)

	ctx := t.Context()
	appliedFirst, err := migrate.AppliedVersions(ctx, first.Conn())
	require.NoError(t, err)
	require.NoError(t, first.Close())

	second, err := Open(t.Context(), dir, DefaultOpenOptions())
	require.NoError(t, err, "second Open on the same dir should succeed")
	t.Cleanup(func() { _ = second.Close() })

	appliedSecond, err := migrate.AppliedVersions(ctx, second.Conn())
	require.NoError(t, err)
	assert.Equal(t, appliedFirst, appliedSecond, "applied migration set should be unchanged")
}

func TestOpen_CreatesMissingParentDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "does", "not", "exist")

	database, err := Open(t.Context(), dir, DefaultOpenOptions())
	require.NoError(t, err, "Open should create the target directory when it does not exist")
	t.Cleanup(func() { _ = database.Close() })

	ctx := t.Context()
	offset, err := database.Append(ctx, "source:test", "key-1", []byte(`{"v":1}`))
	require.NoError(t, err)

	msgs, next, err := database.ReadFrom(ctx, 0, 10)
	require.NoError(t, err)
	require.Len(t, msgs, 1)
	assert.Equal(t, "key-1", msgs[0].Key)
	assert.Equal(t, offset, next)
}
