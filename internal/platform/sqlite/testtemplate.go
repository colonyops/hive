package sqlite

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// TestingT is the part of testing.TB that a store's OpenForTest needs. The store
// packages ship in production binaries, so they do not import testing.
type TestingT interface {
	Helper()
	Context() context.Context
	TempDir() string
	Fatalf(format string, args ...any)
	Cleanup(func())
}

// TestTemplate gives each test its own copy of a database that was migrated
// once per process. Tests spend most of their database setup in migrations, and
// parallel migrations contend inside modernc.org/sqlite.
type TestTemplate struct {
	// Filename is the database file's name in a directory.
	Filename string
	// Migrate creates the migrated database in dir and closes it, so that the
	// main file holds everything and no WAL remains.
	Migrate func(ctx context.Context, dir string) error

	once     sync.Once
	contents []byte
	err      error
}

// WriteTo writes a copy of the template to Filename in dir.
func (tt *TestTemplate) WriteTo(ctx context.Context, dir string) error {
	tt.once.Do(func() { tt.contents, tt.err = tt.build(ctx) })
	if tt.err != nil {
		return tt.err
	}
	return os.WriteFile(filepath.Join(dir, tt.Filename), tt.contents, 0o600)
}

func (tt *TestTemplate) build(ctx context.Context) ([]byte, error) {
	dir, err := os.MkdirTemp("", "hive-db-template-")
	if err != nil {
		return nil, fmt.Errorf("create template dir: %w", err)
	}
	defer func() { _ = os.RemoveAll(dir) }()
	if err := tt.Migrate(ctx, dir); err != nil {
		return nil, fmt.Errorf("migrate template: %w", err)
	}
	return os.ReadFile(filepath.Join(dir, tt.Filename))
}
