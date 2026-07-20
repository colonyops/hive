// Package pipelinedb is the dedicated desktop-pipeline SQLite store: the
// event log (event_log/consumer_offset), persisted pipeline outputs
// (feed_item/output_command), and per-node run metrics (node_run). It is
// isolated from hive's shared hive.db so desktop pipeline write traffic
// never contends with the CLI/TUI data path. It has its own sqlc-generated
// queries (queries.sql.go, models.go, db.go) but shares the migration
// runner in internal/data/migrate.
package pipelinedb

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/colonyops/hive/internal/data/migrate"

	_ "modernc.org/sqlite"
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
	MaxOpenConns int // max open connections (default: 2)
	MaxIdleConns int // max idle connections (default: 2)
	BusyTimeout  int // busy timeout in milliseconds (default: 5000)
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
type DB struct {
	conn    *sql.DB
	queries *Queries
}

// Open creates a new desktop-pipeline.db connection in dir, applying all
// pending migrations. Unlike internal/data/db, there is no legacy bootstrap
// step here: this is a new database with no pre-migration history, so Open
// calls migrate.Up directly.
func Open(dir string, opts OpenOptions) (*DB, error) {
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

	dbPath := filepath.Join(dir, "desktop-pipeline.db")

	// Open with pragmas for WAL mode, busy timeout, and foreign keys, plus
	// _txlock=immediate so write transactions begin with BEGIN IMMEDIATE.
	//
	// Several goroutines write this DB concurrently through WithTx (the
	// producer's AppendIfChanged, the frontend runtime's CommitBatch, the
	// output worker, retention). Each reads before it writes. With the driver
	// default (BEGIN, deferred) two such transactions can both hold a read
	// lock and then both try to upgrade to the write lock — a deadlock SQLite
	// resolves by returning SQLITE_BUSY *immediately*, ignoring busy_timeout,
	// because waiting could never succeed. IMMEDIATE takes the write lock up
	// front, so a second writer waits on busy_timeout and retries cleanly
	// instead of failing. Read-only transactions still begin deferred, so WAL
	// read concurrency is preserved.
	dsn := fmt.Sprintf("file:%s?_txlock=immediate&_pragma=journal_mode(WAL)&_pragma=busy_timeout(%d)&_pragma=foreign_keys(ON)", dbPath, opts.BusyTimeout)
	conn, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Configure connection pool - minimal connections for SQLite.
	conn.SetMaxOpenConns(opts.MaxOpenConns)
	conn.SetMaxIdleConns(opts.MaxIdleConns)
	conn.SetConnMaxLifetime(0) // Connections live forever.

	db := &DB{
		conn:    conn,
		queries: New(conn),
	}

	ctx := context.Background()

	// Verify connectivity - fail fast for SQLite.
	if err := conn.PingContext(ctx); err != nil {
		if closeErr := conn.Close(); closeErr != nil {
			return nil, fmt.Errorf("failed to connect to database: %w (close also failed: %w)", err, closeErr)
		}
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	if err := db.initSchema(ctx); err != nil {
		if closeErr := conn.Close(); closeErr != nil {
			return nil, fmt.Errorf("failed to initialize schema: %w (close also failed: %w)", err, closeErr)
		}
		return nil, fmt.Errorf("failed to initialize schema: %w", err)
	}

	return db, nil
}

// Close closes the database connection.
func (db *DB) Close() error {
	return db.conn.Close()
}

// Conn returns the underlying *sql.DB connection.
func (db *DB) Conn() *sql.DB {
	return db.conn
}

// Queries returns the sqlc queries interface.
func (db *DB) Queries() *Queries {
	return db.queries
}

// WithTx executes a function within a transaction.
// If the function returns an error, the transaction is rolled back.
func (db *DB) WithTx(ctx context.Context, fn func(*Queries) error) error {
	tx, err := db.conn.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}

	queries := db.queries.WithTx(tx)
	if err := fn(queries); err != nil {
		if rbErr := tx.Rollback(); rbErr != nil {
			return fmt.Errorf("transaction failed: %w (rollback also failed: %w)", err, rbErr)
		}
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
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
