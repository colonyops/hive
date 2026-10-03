package db

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"time"

	"github.com/colonyops/hive/internal/platform/sqlite"
)

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

// DB wraps a SQL database connection with sqlc queries.
type DB struct {
	conn    *sql.DB
	queries *Queries
}

// Open creates a new database connection with the given options.
// The database file is created in the specified data directory.
// Uses minimal connection pool (default 2) to prevent transaction deadlocks while
// avoiding unnecessary connection overhead.
func Open(dataDir string, opts OpenOptions) (*DB, error) {
	// Apply defaults for zero values
	if opts.MaxOpenConns == 0 {
		opts.MaxOpenConns = DefaultOpenOptions().MaxOpenConns
	}
	if opts.MaxIdleConns == 0 {
		opts.MaxIdleConns = DefaultOpenOptions().MaxIdleConns
	}
	if opts.BusyTimeout == 0 {
		opts.BusyTimeout = DefaultOpenOptions().BusyTimeout
	}

	conn, err := sqlite.Open(context.Background(), filepath.Join(dataDir, "hive.db"), sqlite.Options{
		MaxOpenConns: opts.MaxOpenConns,
		MaxIdleConns: opts.MaxIdleConns,
		BusyTimeout:  time.Duration(opts.BusyTimeout) * time.Millisecond,
	})
	if err != nil {
		return nil, err
	}

	db := &DB{
		conn:    conn,
		queries: New(conn),
	}

	// Initialize schema
	if err := db.initSchema(context.Background()); err != nil {
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

// initSchema runs all pending up migrations.
func (db *DB) initSchema(ctx context.Context) error {
	return runMigrations(ctx, db.conn)
}
