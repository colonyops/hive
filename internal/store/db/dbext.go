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
//
// *Queries is embedded so a store can call a generated query on the DB it
// holds, including the transaction-bound DB that Ctx returns.
type DB struct {
	*Queries

	conn *sql.DB
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
		Queries: New(conn),
		conn:    conn,
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

// Conn returns the underlying connection pool, even on a transaction-bound
// DB. Use WithinTx and the generated queries for transactional work rather
// than running raw SQL through this.
func (db *DB) Conn() *sql.DB {
	return db.conn
}

// Ctx returns a DB bound to the ambient transaction if the context carries
// one for this database, and the receiver otherwise. Store methods begin with
// db.Ctx(ctx) so they join a unit of work their caller opened.
func (db *DB) Ctx(ctx context.Context) *DB {
	tx, ok := sqlite.AmbientTx(ctx, db.conn)
	if !ok {
		return db
	}
	return db.boundTo(tx)
}

func (db *DB) boundTo(tx *sql.Tx) *DB {
	bound := *db
	bound.Queries = db.WithTx(tx)
	return &bound
}

// WithinTx runs fn inside a transaction, joining an ambient one on this
// database rather than opening a second (see sqlite.WithinTx). Only the
// outermost caller commits or rolls back; an inner fn that fails returns its
// error up to that caller, which rolls the whole unit back.
func (db *DB) WithinTx(ctx context.Context, fn func(context.Context, *DB) error) error {
	return sqlite.WithinTx(ctx, db.conn, func(ctx context.Context, tx *sql.Tx) error {
		return fn(ctx, db.boundTo(tx))
	})
}

// initSchema runs all pending up migrations.
func (db *DB) initSchema(ctx context.Context) error {
	return runMigrations(ctx, db.conn)
}
