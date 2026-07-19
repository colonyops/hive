package migrate

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	_ "modernc.org/sqlite"
)

// twoMigrations is a synthetic, self-contained migration set used to exercise
// the runner without depending on any real database's schema.
func twoMigrations() fstest.MapFS {
	return fstest.MapFS{
		"0001_widgets.up.sql":   {Data: []byte("CREATE TABLE widgets (id INTEGER PRIMARY KEY);")},
		"0001_widgets.down.sql": {Data: []byte("DROP TABLE widgets;")},
		"0002_gadgets.up.sql":   {Data: []byte("CREATE TABLE gadgets (id INTEGER PRIMARY KEY);")},
		"0002_gadgets.down.sql": {Data: []byte("DROP TABLE gadgets;")},
	}
}

func openTestConn(t *testing.T) *sql.DB {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	conn, err := sql.Open("sqlite", fmt.Sprintf("file:%s?_pragma=journal_mode(WAL)&_pragma=foreign_keys(ON)", dbPath))
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

func TestLoad_Valid(t *testing.T) {
	migrations, err := Load(twoMigrations())
	require.NoError(t, err)
	require.Len(t, migrations, 2)

	// Ascending version order.
	assert.Equal(t, 1, migrations[0].Version)
	assert.Equal(t, "widgets", migrations[0].Name)
	assert.Equal(t, 2, migrations[1].Version)
	assert.Equal(t, "gadgets", migrations[1].Name)

	for _, m := range migrations {
		assert.NotEmpty(t, m.UpSQL, "version %d up SQL", m.Version)
		assert.NotEmpty(t, m.DownSQL, "version %d down SQL", m.Version)
	}
}

func TestLoad_Errors(t *testing.T) {
	tests := map[string]fstest.MapFS{
		"missing down": {
			"0001_x.up.sql": {Data: []byte("SELECT 1;")},
		},
		"missing up": {
			"0001_x.down.sql": {Data: []byte("SELECT 1;")},
		},
		"duplicate up": {
			"0001_x.up.sql":   {Data: []byte("SELECT 1;")},
			"0001_x.down.sql": {Data: []byte("SELECT 1;")},
			"0001_y.up.sql":   {Data: []byte("SELECT 2;")},
		},
		"name mismatch": {
			"0001_x.up.sql":   {Data: []byte("SELECT 1;")},
			"0001_y.down.sql": {Data: []byte("SELECT 1;")},
		},
		"bad filename": {
			"nope.sql": {Data: []byte("SELECT 1;")},
		},
	}

	for name, fsys := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := Load(fsys)
			assert.Error(t, err)
		})
	}
}

func TestUp_AppliesAllAndIsIdempotent(t *testing.T) {
	conn := openTestConn(t)
	ctx := context.Background()
	fsys := twoMigrations()

	require.NoError(t, Up(ctx, conn, fsys))

	applied, err := AppliedVersions(ctx, conn)
	require.NoError(t, err)
	assert.Len(t, applied, 2)
	assert.True(t, applied[1] && applied[2])

	// Tables from both migrations exist.
	_, err = conn.ExecContext(ctx, "SELECT 1 FROM widgets LIMIT 0")
	require.NoError(t, err)
	_, err = conn.ExecContext(ctx, "SELECT 1 FROM gadgets LIMIT 0")
	require.NoError(t, err)

	// Re-running is a no-op.
	require.NoError(t, Up(ctx, conn, fsys), "second Up should be idempotent")
	applied, err = AppliedVersions(ctx, conn)
	require.NoError(t, err)
	assert.Len(t, applied, 2)
}

func TestDown_RevertsLastN(t *testing.T) {
	conn := openTestConn(t)
	ctx := context.Background()
	fsys := twoMigrations()

	require.NoError(t, Up(ctx, conn, fsys))
	// Row in the first-migration table; it must survive reverting only the second.
	_, err := conn.ExecContext(ctx, "INSERT INTO widgets (id) VALUES (1)")
	require.NoError(t, err)

	require.NoError(t, Down(ctx, conn, fsys, 1))

	applied, err := AppliedVersions(ctx, conn)
	require.NoError(t, err)
	assert.Len(t, applied, 1)
	assert.True(t, applied[1])
	assert.False(t, applied[2], "version 2 should be reverted")

	// gadgets table dropped, widgets row preserved.
	_, err = conn.ExecContext(ctx, "SELECT 1 FROM gadgets LIMIT 0")
	require.Error(t, err, "gadgets table should be gone")

	var count int
	require.NoError(t, conn.QueryRowContext(ctx, "SELECT COUNT(*) FROM widgets").Scan(&count))
	assert.Equal(t, 1, count)
}

func TestDown_InvalidN(t *testing.T) {
	conn := openTestConn(t)
	ctx := context.Background()
	fsys := twoMigrations()

	require.Error(t, Down(ctx, conn, fsys, 0))
	require.Error(t, Down(ctx, conn, fsys, -1))
}

func TestDown_TooMany(t *testing.T) {
	conn := openTestConn(t)
	ctx := context.Background()
	fsys := twoMigrations()

	require.NoError(t, Up(ctx, conn, fsys))
	assert.Error(t, Down(ctx, conn, fsys, 3), "requesting more than applied should fail")
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
		{"0008_add_clone_strategy.up.sql", 8, "add_clone_strategy", "up", false},
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
