package db

import (
	"context"
	"database/sql"
	_ "embed"
	"fmt"
	"path/filepath"

	_ "modernc.org/sqlite"
)

//go:embed schema/schema.sql
var schemaSQL string

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
// avoiding unnecessary connection overhead. WAL mode and busy_timeout
// handle concurrent access at the SQLite level.
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

	dbPath := filepath.Join(dataDir, "hive.db")

	// Open with pragmas for WAL mode, busy timeout, and foreign keys
	dsn := fmt.Sprintf("file:%s?_pragma=journal_mode(WAL)&_pragma=busy_timeout(%d)&_pragma=foreign_keys(ON)", dbPath, opts.BusyTimeout)
	conn, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Configure connection pool - minimal connections for SQLite
	conn.SetMaxOpenConns(opts.MaxOpenConns)
	conn.SetMaxIdleConns(opts.MaxIdleConns)
	conn.SetConnMaxLifetime(0) // Connections live forever

	db := &DB{
		conn:    conn,
		queries: New(conn),
	}

	// Verify connectivity - fail fast for SQLite
	if err := conn.PingContext(context.Background()); err != nil {
		if closeErr := conn.Close(); closeErr != nil {
			return nil, fmt.Errorf("failed to connect to database: %w (close also failed: %w)", err, closeErr)
		}
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	// Initialize schema
	if err := db.initSchema(context.Background()); err != nil {
		if closeErr := conn.Close(); closeErr != nil {
			return nil, fmt.Errorf("failed to initialize schema: %w (close also failed: %w)", err, closeErr)
		}
		return nil, fmt.Errorf("failed to initialize schema: %w", err)
	}

	// Run migrations for existing databases
	if err := db.runMigrations(context.Background()); err != nil {
		if closeErr := conn.Close(); closeErr != nil {
			return nil, fmt.Errorf("failed to run migrations: %w (close also failed: %w)", err, closeErr)
		}
		return nil, fmt.Errorf("failed to run migrations: %w", err)
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

// initSchema creates the database schema if it doesn't exist.
func (db *DB) initSchema(ctx context.Context) error {
	_, err := db.conn.ExecContext(ctx, schemaSQL)
	if err != nil {
		return fmt.Errorf("failed to execute schema: %w", err)
	}
	return nil
}

// runMigrations applies incremental schema migrations for existing databases.
func (db *DB) runMigrations(ctx context.Context) error {
	var version int
	err := db.conn.QueryRowContext(ctx, "SELECT version FROM schema_version").Scan(&version)
	if err != nil {
		return fmt.Errorf("failed to read schema version: %w", err)
	}

	if version < 7 {
		if err := db.migrateV6ToV7(ctx); err != nil {
			return fmt.Errorf("migration v6→v7: %w", err)
		}
	}

	return nil
}

// migrateV6ToV7 adds clone_strategy column to sessions table.
func (db *DB) migrateV6ToV7(ctx context.Context) error {
	tx, err := db.conn.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	// Check if column already exists (idempotent)
	var count int
	err = tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM pragma_table_info('sessions') WHERE name = 'clone_strategy'").Scan(&count)
	if err != nil {
		return fmt.Errorf("check column existence: %w", err)
	}
	if count == 0 {
		if _, err := tx.ExecContext(ctx, "ALTER TABLE sessions ADD COLUMN clone_strategy TEXT NOT NULL DEFAULT 'full'"); err != nil {
			return fmt.Errorf("add column: %w", err)
		}
	}

	if _, err := tx.ExecContext(ctx, "UPDATE schema_version SET version = 7"); err != nil {
		return fmt.Errorf("update version: %w", err)
	}

	return tx.Commit()
}
