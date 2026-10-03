// Package queries is the desktop app's dedicated SQLite store: the
// event log (event_log/consumer_offset), durable inbox substrate
// (inbox_item/inbox_event), feed membership claims, output commands, activity
// events and jobs, and per-node run metrics. It is isolated from hive's shared
// hive.db so desktop pipeline write traffic never contends with the CLI/TUI
// data path. It shares the migration runner in internal/data/migrate.
package queries

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/rs/zerolog"

	"github.com/colonyops/hive/internal/data/migrate"
	"github.com/colonyops/hive/internal/platform/sqlite"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// migrationsSub returns the embedded migrations rooted at their directory, so
// the storage-agnostic migrate runner sees the files at the FS root.
func migrationsSub() (fs.FS, error) {
	return fs.Sub(migrationsFS, "migrations")
}

// OpenOptions configures database connection settings.
type OpenOptions struct {
	MaxOpenConns int            // max open connections (default: 2)
	MaxIdleConns int            // max idle connections (default: 2)
	BusyTimeout  int            // busy timeout in milliseconds (default: 5000)
	PauseCommit  time.Duration  // development-only crash-window widening
	Logger       zerolog.Logger // where the store reports recoverable anomalies; zero value discards
}

// DefaultOpenOptions returns the recommended defaults for SQLite.
func DefaultOpenOptions() OpenOptions {
	return OpenOptions{
		MaxOpenConns: 2,
		MaxIdleConns: 2,
		BusyTimeout:  5000,
	}
}

// DB wraps a SQL database connection with sqlc queries plus the hand-written
// event log API (see log.go).
//
// *Queries is embedded so a store in a sibling package can call a generated
// query on the DB it holds, including the transaction-bound DB that Ctx
// returns.
//
// A DB is either pool-backed or bound to one transaction. Ctx (see ext.go)
// produces the bound form from an ambient transaction on the context; every
// query a bound DB runs joins that transaction.
type DB struct {
	*Queries

	conn        *sql.DB
	tx          *sql.Tx
	pauseCommit time.Duration
	logger      zerolog.Logger
}

// querier is what hand-written SQL in this package must run against: the
// ambient transaction when this DB is bound to one, and the pool otherwise.
// Reaching for db.conn directly in a bound DB would silently escape the
// transaction.
func (db *DB) querier() DBTX {
	if db.tx != nil {
		return db.tx
	}
	return db.conn
}

// DatabasePath returns the desktop-pipeline.db file path within dir. It is the
// single source of truth for the filename, shared by Open and by callers (the
// System settings screen) that need to display or reveal the database.
func DatabasePath(dir string) string {
	return filepath.Join(dir, "desktop-pipeline.db")
}

// Open creates a new desktop-pipeline.db connection in dir, applying all
// pending migrations. Unlike internal/data/db, there is no legacy bootstrap
// step here: this is a new database with no pre-migration history, so Open
// calls migrate.Up directly.
//
// ctx covers connectivity, migration and interrupted-command recovery, so a
// cancelled startup does not leave a half-migrated database behind.
func Open(ctx context.Context, dir string, opts OpenOptions) (*DB, error) {
	// Apply defaults for zero values.
	if opts.MaxOpenConns == 0 {
		opts.MaxOpenConns = DefaultOpenOptions().MaxOpenConns
	}
	if opts.MaxIdleConns == 0 {
		opts.MaxIdleConns = DefaultOpenOptions().MaxIdleConns
	}
	if opts.BusyTimeout == 0 {
		opts.BusyTimeout = DefaultOpenOptions().BusyTimeout
	}

	// SQLite will not create a missing parent directory itself, and on a
	// fresh install dir (desktop.StateDir()) does not exist yet — only the
	// feed store's first save lazily creates it. Ensure it exists here so
	// Open is self-contained and robust for any caller, not just main.go.
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("failed to create database directory: %w", err)
	}

	conn, err := sqlite.Open(ctx, DatabasePath(dir), sqlite.Options{
		MaxOpenConns: opts.MaxOpenConns,
		MaxIdleConns: opts.MaxIdleConns,
		BusyTimeout:  time.Duration(opts.BusyTimeout) * time.Millisecond,
	})
	if err != nil {
		return nil, err
	}

	db := &DB{
		Queries:     New(conn),
		conn:        conn,
		pauseCommit: opts.PauseCommit,
		logger:      opts.Logger,
	}

	if err := db.initSchema(ctx); err != nil {
		if closeErr := conn.Close(); closeErr != nil {
			return nil, fmt.Errorf("failed to initialize schema: %w (close also failed: %w)", err, closeErr)
		}
		return nil, fmt.Errorf("failed to initialize schema: %w", err)
	}
	if err := db.RecoverInterruptedOutputCommands(ctx); err != nil {
		if closeErr := conn.Close(); closeErr != nil {
			return nil, fmt.Errorf("failed to recover interrupted commands: %w (close also failed: %w)", err, closeErr)
		}
		return nil, fmt.Errorf("failed to recover interrupted commands: %w", err)
	}

	return db, nil
}

// Close closes the database connection.
func (db *DB) Close() error {
	return db.conn.Close()
}

// Conn returns the underlying connection pool. It is the pool even on a
// transaction-bound DB, because a pool is what its type promises — use
// WithinTx and the generated queries for transactional work rather than
// running raw SQL through this.
func (db *DB) Conn() *sql.DB {
	return db.conn
}

// initSchema applies all pending migrations. No legacy bootstrap: this
// database has no history predating the migration framework.
func (db *DB) initSchema(ctx context.Context) error {
	sub, err := migrationsSub()
	if err != nil {
		return fmt.Errorf("opening migrations fs: %w", err)
	}
	return migrate.Up(ctx, db.conn, sub)
}
