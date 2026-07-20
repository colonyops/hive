package pipelinedb

import (
	"context"
	"database/sql"
	"fmt"
	"os"
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

func seedPipelineDBAtMigration(t *testing.T, dir string, version int) *sql.DB {
	t.Helper()
	require.NoError(t, os.MkdirAll(dir, 0o700))

	dbPath := filepath.Join(dir, "desktop-pipeline.db")
	conn, err := sql.Open("sqlite", fmt.Sprintf("file:%s?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(ON)", dbPath))
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })

	sub, err := migrationsSub()
	require.NoError(t, err)
	migrations, err := migrate.Load(sub)
	require.NoError(t, err)

	var selected []migrate.Migration
	for _, migration := range migrations {
		if migration.Version <= version {
			selected = append(selected, migration)
		}
	}
	require.NotEmpty(t, selected)
	require.NoError(t, migrate.Apply(context.Background(), conn, selected))

	return conn
}

func TestOpen_FreshDB_AppliesMigrations(t *testing.T) {
	database := openTestDB(t)
	ctx := context.Background()

	for _, table := range []string{"event_log", "consumer_offset", "source_head", "feed_item", "output_command", "node_run"} {
		_, err := database.Conn().ExecContext(ctx, "SELECT 1 FROM "+table+" LIMIT 0")
		require.NoError(t, err, "%s table should exist", table)
	}

	sub, err := migrationsSub()
	require.NoError(t, err)
	migrations, err := migrate.Load(sub)
	require.NoError(t, err)
	require.NotEmpty(t, migrations)

	applied, err := migrate.AppliedVersions(ctx, database.Conn())
	require.NoError(t, err)
	assert.Len(t, applied, len(migrations))
}

func TestOpen_UpgradeToSourceSnapshots_ClearsLegacyFeedItems(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()

	// Start at schema version 5, when feed rows had no source/snapshot
	// provenance, then seed a row as an existing installation would have.
	conn := seedPipelineDBAtMigration(t, dir, 5)
	_, err := conn.ExecContext(ctx, `
		INSERT INTO feed_item (feed_id, item_id, payload, updated_at, unread)
		VALUES ('feed', 'legacy-item', X'7B7D', 1, 1)
	`)
	require.NoError(t, err)
	require.NoError(t, conn.Close())

	upgraded, err := Open(dir, DefaultOpenOptions())
	require.NoError(t, err)
	t.Cleanup(func() { _ = upgraded.Close() })

	var count int
	require.NoError(t, upgraded.Conn().QueryRowContext(ctx, "SELECT COUNT(*) FROM feed_item").Scan(&count))
	assert.Zero(t, count, "migration must remove rows whose provenance cannot be reconstructed")
}

func TestOpen_UpgradeMigratesAwaitingConfirmationToPending(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()

	conn := seedPipelineDBAtMigration(t, dir, 7)
	_, err := conn.ExecContext(ctx, `
		INSERT INTO output_command (action_id, payload, status, created_at, "key", attempts)
		VALUES ('review', X'7B7D', 'awaiting_confirmation', 1, 'item-1', 0)`)
	require.NoError(t, err)
	require.NoError(t, conn.Close())

	upgraded, err := Open(dir, DefaultOpenOptions())
	require.NoError(t, err)
	t.Cleanup(func() { _ = upgraded.Close() })
	var status string
	require.NoError(t, upgraded.Conn().QueryRowContext(ctx, `SELECT status FROM output_command WHERE action_id = 'review'`).Scan(&status))
	assert.Equal(t, "pending", status)
}

func TestOpen_RecoversInterruptedRunningCommandWithoutRetry(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()
	first, err := Open(dir, DefaultOpenOptions())
	require.NoError(t, err)
	enqueueTestCommand(t, first, "review", "item-1")
	_, created, err := first.ConfirmOutputCommand(ctx, "review", "item-1", []byte(`{}`))
	require.NoError(t, err)
	require.True(t, created)
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
}

func TestOpen_Idempotent(t *testing.T) {
	dir := t.TempDir()

	first, err := Open(dir, DefaultOpenOptions())
	require.NoError(t, err)

	ctx := context.Background()
	appliedFirst, err := migrate.AppliedVersions(ctx, first.Conn())
	require.NoError(t, err)
	require.NoError(t, first.Close())

	// Re-opening the same directory should be a no-op: migrations are
	// already applied, so this just re-attaches to the existing database.
	second, err := Open(dir, DefaultOpenOptions())
	require.NoError(t, err, "second Open on the same dir should succeed")
	t.Cleanup(func() { _ = second.Close() })

	appliedSecond, err := migrate.AppliedVersions(ctx, second.Conn())
	require.NoError(t, err)
	assert.Equal(t, appliedFirst, appliedSecond, "applied migration set should be unchanged")
}

// TestOpen_CreatesMissingParentDir is a regression test for the fresh-install
// startup crash: desktop.StateDir() does not exist until the feed store's
// first save (see feed/store.go's writeFileAtomic), but main.go calls
// pipelinedb.Open before anything else has a chance to create it. SQLite
// does not create a missing parent directory on its own, so Open must.
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
