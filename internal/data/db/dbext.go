package db

import (
	"context"
	"database/sql"
	_ "embed"
	"fmt"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

//go:embed schema/schema.sql
var schemaSQL string

const (
	maxRetries   = 5
	initialWait  = 100 * time.Millisecond
	maxOpenConns = 10
	maxIdleConns = 5
	busyTimeout  = 5000 // milliseconds
)

// DB wraps a SQL database connection with retry logic and sqlc queries.
type DB struct {
	conn    *sql.DB
	queries *Queries
}

// Open creates a new database connection with connection pooling and retry logic.
// The database file is created in the specified data directory.
func Open(dataDir string) (*DB, error) {
	dbPath := filepath.Join(dataDir, "hive.db")

	// Open with pragmas for WAL mode and busy timeout
	dsn := fmt.Sprintf("file:%s?_pragma=journal_mode(WAL)&_pragma=busy_timeout(%d)", dbPath, busyTimeout)
	conn, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Configure connection pool
	conn.SetMaxOpenConns(maxOpenConns)
	conn.SetMaxIdleConns(maxIdleConns)
	conn.SetConnMaxLifetime(0) // Connections live forever

	db := &DB{
		conn:    conn,
		queries: New(conn),
	}

	// Verify connectivity with retry
	if err := db.pingWithRetry(context.Background()); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	// Initialize schema
	if err := db.initSchema(context.Background()); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("failed to initialize schema: %w", err)
	}

	return db, nil
}

// Close closes the database connection.
func (db *DB) Close() error {
	return db.conn.Close()
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
		_ = tx.Rollback()
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// pingWithRetry attempts to ping the database with exponential backoff.
func (db *DB) pingWithRetry(ctx context.Context) error {
	wait := initialWait
	for i := 0; i < maxRetries; i++ {
		if err := db.conn.PingContext(ctx); err == nil {
			return nil
		}

		if i < maxRetries-1 {
			time.Sleep(wait)
			wait *= 2
		}
	}

	return fmt.Errorf("failed to ping database after %d retries", maxRetries)
}

// initSchema creates the database schema if it doesn't exist.
func (db *DB) initSchema(ctx context.Context) error {
	_, err := db.conn.ExecContext(ctx, schemaSQL)
	if err != nil {
		return fmt.Errorf("failed to execute schema: %w", err)
	}
	return nil
}
