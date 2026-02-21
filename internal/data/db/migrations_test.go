package db

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

func TestMigrateUp_FreshDB(t *testing.T) {
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

	migrations, err := loadMigrations()
	require.NoError(t, err)

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

func TestMigrateUp_Idempotent(t *testing.T) {
	database := openTestDB(t)
	ctx := context.Background()

	// Running migrateUp again should be a no-op.
	err := migrateUp(ctx, database.Conn())
	assert.NoError(t, err, "second migrateUp should be idempotent")
}

func TestMigrateUp_LegacyBootstrap(t *testing.T) {
	conn := openRawConn(t)
	ctx := context.Background()

	migrations, err := loadMigrations()
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(migrations), 6, "need at least 6 migrations for legacy test")

	// Simulate a legacy database: create all tables manually and set schema_version=6.
	for _, m := range migrations[:6] {
		_, err := conn.ExecContext(ctx, m.UpSQL)
		require.NoError(t, err, "applying migration %d SQL", m.Version)
	}

	_, err = conn.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS schema_version (version INTEGER PRIMARY KEY);
		INSERT INTO schema_version (version) VALUES (6);
	`)
	require.NoError(t, err, "creating legacy schema_version")

	// Run migrateUp — should bootstrap schema_migrations from legacy version.
	err = migrateUp(ctx, conn)
	require.NoError(t, err, "migrateUp with legacy DB")

	// Verify versions 1-6 are in schema_migrations.
	applied, err := appliedVersions(ctx, conn)
	require.NoError(t, err)
	for i := 1; i <= 6; i++ {
		assert.True(t, applied[i], "version %d should be marked as applied", i)
	}
}

func TestMigrateDown(t *testing.T) {
	database := openTestDB(t)
	ctx := context.Background()
	conn := database.Conn()

	// Insert a session row so we can verify the table still works.
	_, err := conn.ExecContext(ctx, `
		INSERT INTO sessions (id, name, slug, path, remote, state, created_at, updated_at)
		VALUES ('test-1', 'Test', 'test', '/tmp/test', 'https://example.com', 'active', 1, 1)
	`)
	require.NoError(t, err)

	// Revert the last migration (kv_store).
	err = MigrateDown(ctx, conn, 1)
	require.NoError(t, err)

	// kv_store table should be gone.
	_, err = conn.ExecContext(ctx, "SELECT 1 FROM kv_store LIMIT 0")
	require.Error(t, err, "kv_store should not exist after down migration")

	// sessions table should still exist.
	var count int
	err = conn.QueryRowContext(ctx, "SELECT COUNT(*) FROM sessions").Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 1, count, "session row should be preserved")
}

func TestMigrateDown_InvalidN(t *testing.T) {
	conn := openRawConn(t)
	ctx := context.Background()

	err := MigrateDown(ctx, conn, 0)
	require.Error(t, err, "n=0 should fail")

	err = MigrateDown(ctx, conn, -1)
	require.Error(t, err, "n=-1 should fail")
}

func TestMigrateDown_TooMany(t *testing.T) {
	database := openTestDB(t)
	ctx := context.Background()

	migrations, err := loadMigrations()
	require.NoError(t, err)

	err = MigrateDown(ctx, database.Conn(), len(migrations)+1)
	assert.Error(t, err, "requesting more down migrations than applied should fail")
}

func TestLoadMigrations_Valid(t *testing.T) {
	migrations, err := loadMigrations()
	require.NoError(t, err)
	require.NotEmpty(t, migrations)

	// Verify ascending version order.
	for i := 1; i < len(migrations); i++ {
		assert.Greater(t, migrations[i].Version, migrations[i-1].Version,
			"migrations should be in ascending version order")
	}

	// Every migration must have both up and down SQL.
	for _, m := range migrations {
		assert.NotEmpty(t, m.UpSQL, "migration %d up SQL should not be empty", m.Version)
		assert.NotEmpty(t, m.DownSQL, "migration %d down SQL should not be empty", m.Version)
		assert.NotEmpty(t, m.Name, "migration %d name should not be empty", m.Version)
	}
}

func TestParseFilename(t *testing.T) {
	tests := []struct {
		filename      string
		wantVersion   int
		wantName      string
		wantDirection string
		wantErr       bool
	}{
		{"0001_initial.up.sql", 1, "initial", "up", false},
		{"0001_initial.down.sql", 1, "initial", "down", false},
		{"0007_add_clone_strategy.up.sql", 7, "add_clone_strategy", "up", false},
		{"0100_big_version.down.sql", 100, "big_version", "down", false},
		{"bad.sql", 0, "", "", true},
		{"0001_initial.sql", 0, "", "", true},
		{"0000_zero.up.sql", 0, "", "", true},
		{"-1_negative.up.sql", 0, "", "", true},
		{"abc_notnumber.up.sql", 0, "", "", true},
		{"0001_.up.sql", 0, "", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.filename, func(t *testing.T) {
			version, name, direction, err := parseFilename(tt.filename)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantVersion, version)
			assert.Equal(t, tt.wantName, name)
			assert.Equal(t, tt.wantDirection, direction)
		})
	}
}
