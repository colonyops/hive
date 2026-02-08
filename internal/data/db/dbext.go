package db

import (
	"context"
	"database/sql"
	_ "embed"
	"fmt"
	"path/filepath"
	"strings"

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

// initSchema creates the database schema if it doesn't exist and runs migrations.
func (db *DB) initSchema(ctx context.Context) error {
	// Execute base schema (idempotent)
	_, err := db.conn.ExecContext(ctx, schemaSQL)
	if err != nil {
		return fmt.Errorf("failed to execute schema: %w", err)
	}

	// Get current schema version
	var currentVersion int
	err = db.conn.QueryRowContext(ctx, "SELECT version FROM schema_version").Scan(&currentVersion)
	if err != nil {
		return fmt.Errorf("failed to read schema version: %w", err)
	}

	// Run migrations
	if err := db.runMigrations(ctx, currentVersion); err != nil {
		return fmt.Errorf("failed to run migrations: %w", err)
	}

	return nil
}

// runMigrations applies schema migrations from the current version to the latest.
func (db *DB) runMigrations(ctx context.Context, currentVersion int) error {
	const targetVersion = 5

	if currentVersion >= targetVersion {
		return nil
	}

	// Wrap all migrations in a single transaction
	return db.WithTx(ctx, func(q *Queries) error {
		// Migration from version 4 to 5: Add diff context support
		if currentVersion < 5 {
			// Add columns to review_sessions
			if _, err := db.conn.ExecContext(ctx, `
				ALTER TABLE review_sessions ADD COLUMN session_name TEXT DEFAULT '' NOT NULL;
			`); err != nil && !isColumnExistsError(err) {
				return fmt.Errorf("failed to add session_name column: %w", err)
			}

			if _, err := db.conn.ExecContext(ctx, `
				ALTER TABLE review_sessions ADD COLUMN diff_context TEXT DEFAULT '' NOT NULL;
			`); err != nil && !isColumnExistsError(err) {
				return fmt.Errorf("failed to add diff_context column: %w", err)
			}

			// Add side column to review_comments
			if _, err := db.conn.ExecContext(ctx, `
				ALTER TABLE review_comments ADD COLUMN side TEXT DEFAULT '' NOT NULL CHECK(side IN ('', 'old', 'new'));
			`); err != nil && !isColumnExistsError(err) {
				return fmt.Errorf("failed to add side column: %w", err)
			}

			// Update schema version (DELETE + INSERT because version is PRIMARY KEY)
			if _, err := db.conn.ExecContext(ctx, `
				DELETE FROM schema_version;
			`); err != nil {
				return fmt.Errorf("failed to clear schema version: %w", err)
			}
			if _, err := db.conn.ExecContext(ctx, `
				INSERT INTO schema_version (version) VALUES (5);
			`); err != nil {
				return fmt.Errorf("failed to set schema version to 5: %w", err)
			}
		}

		return nil
	})
}

// isColumnExistsError checks if the error is due to a column already existing.
// SQLite error message contains "duplicate column name"
func isColumnExistsError(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "duplicate column name")
}
