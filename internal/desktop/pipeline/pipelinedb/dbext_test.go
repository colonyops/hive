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

func TestOpen_FreshDB_AppliesMigrations(t *testing.T) {
	database := openTestDB(t)
	ctx := context.Background()

	for _, table := range []string{"event_log", "consumer_offset", "feed_item", "output_command", "node_run"} {
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
