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
		"0001_widgets.up.sql": {Data: []byte("CREATE TABLE widgets (id INTEGER PRIMARY KEY);")},
		"0002_gadgets.up.sql": {Data: []byte("CREATE TABLE gadgets (id INTEGER PRIMARY KEY);")},
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
	}
}

func TestLoad_Errors(t *testing.T) {
	tests := map[string]fstest.MapFS{
		"duplicate version": {
			"0001_x.up.sql": {Data: []byte("SELECT 1;")},
			"0001_y.up.sql": {Data: []byte("SELECT 2;")},
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

func TestParseFilename(t *testing.T) {
	tests := []struct {
		filename    string
		wantVersion int
		wantName    string
		wantErr     bool
	}{
		{"0001_initial.up.sql", 1, "initial", false},
		{"0008_add_clone_strategy.up.sql", 8, "add_clone_strategy", false},
		{"bad.sql", 0, "", true},
		{"0001_initial.sql", 0, "", true},
		{"0000_zero.up.sql", 0, "", true},
		{"-1_negative.up.sql", 0, "", true},
		{"abc_notnumber.up.sql", 0, "", true},
		{"0001_.up.sql", 0, "", true},
	}

	for _, tt := range tests {
		t.Run(tt.filename, func(t *testing.T) {
			version, name, err := parseFilename(tt.filename)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantVersion, version)
			assert.Equal(t, tt.wantName, name)
		})
	}
}
