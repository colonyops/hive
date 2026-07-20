package db

import (
	"context"
	"database/sql"
	"fmt"
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

func openRawConn(t *testing.T) *sql.DB {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "hive.db")
	conn, err := sql.Open("sqlite", fmt.Sprintf("file:%s?_pragma=journal_mode(WAL)&_pragma=foreign_keys(ON)", dbPath))
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

// hiveMigrations loads the embedded hive migration set for assertions.
func hiveMigrations(t *testing.T) []migrate.Migration {
	t.Helper()
	sub, err := migrationsSub()
	require.NoError(t, err)
	migrations, err := migrate.Load(sub)
	require.NoError(t, err)
	return migrations
}

func TestRunMigrations_FreshDB(t *testing.T) {
	database := openTestDB(t)
	ctx := context.Background()

	// Verify schema_migrations has all versions recorded.
	rows, err := database.Conn().QueryContext(ctx, "SELECT version FROM schema_migrations ORDER BY version")
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()

	var versions []int
	for rows.Next() {
		var v int
		require.NoError(t, rows.Scan(&v))
		versions = append(versions, v)
	}
	require.NoError(t, rows.Err())

	migrations := hiveMigrations(t)
	require.Len(t, versions, len(migrations))
	for i, m := range migrations {
		assert.Equal(t, m.Version, versions[i])
	}

	// Verify core tables exist by doing simple queries.
	_, err = database.Conn().ExecContext(ctx, "SELECT 1 FROM sessions LIMIT 0")
	require.NoError(t, err, "sessions table should exist")

	_, err = database.Conn().ExecContext(ctx, "SELECT 1 FROM messages LIMIT 0")
	require.NoError(t, err, "messages table should exist")

	_, err = database.Conn().ExecContext(ctx, "SELECT 1 FROM kv_store LIMIT 0")
	require.NoError(t, err, "kv_store table should exist")
}

func TestRunMigrations_Idempotent(t *testing.T) {
	database := openTestDB(t)
	ctx := context.Background()

	// Running again should be a no-op.
	err := runMigrations(ctx, database.Conn())
	assert.NoError(t, err, "second runMigrations should be idempotent")
}

func TestRunMigrations_LegacyBootstrap(t *testing.T) {
	conn := openRawConn(t)
	ctx := context.Background()

	migrations := hiveMigrations(t)
	require.GreaterOrEqual(t, len(migrations), 6, "need at least 6 migrations for legacy test")

	// Simulate a legacy database: create the first six tables manually and set schema_version=6.
	for _, m := range migrations[:6] {
		_, err := conn.ExecContext(ctx, m.UpSQL)
		require.NoError(t, err, "applying migration %d SQL", m.Version)
	}

	_, err := conn.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS schema_version (version INTEGER PRIMARY KEY);
		INSERT INTO schema_version (version) VALUES (6);
	`)
	require.NoError(t, err, "creating legacy schema_version")

	// runMigrations should bootstrap schema_migrations from the legacy version.
	err = runMigrations(ctx, conn)
	require.NoError(t, err, "runMigrations with legacy DB")

	applied, err := migrate.AppliedVersions(ctx, conn)
	require.NoError(t, err)
	for i := 1; i <= 6; i++ {
		assert.True(t, applied[i], "version %d should be marked as applied", i)
	}
}

func TestRunMigrations_LegacyBootstrap_EmptySchemaVersion(t *testing.T) {
	conn := openRawConn(t)
	ctx := context.Background()

	// Create legacy schema_version table but leave it empty.
	_, err := conn.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_version (version INTEGER PRIMARY KEY)`)
	require.NoError(t, err)

	// runMigrations should handle NULL MAX(version) without error.
	err = runMigrations(ctx, conn)
	require.NoError(t, err, "runMigrations with empty schema_version table")

	// All migrations should be applied normally (bootstrap was a no-op).
	migrations := hiveMigrations(t)
	applied, err := migrate.AppliedVersions(ctx, conn)
	require.NoError(t, err)
	assert.Len(t, applied, len(migrations))
}
