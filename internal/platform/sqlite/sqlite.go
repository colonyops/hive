// Package sqlite opens the SQLite databases hive and Hive Desktop keep, with
// the DSN and pool settings every caller must agree on.
package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

type Options struct {
	MaxOpenConns int
	MaxIdleConns int
	BusyTimeout  time.Duration
}

// Open opens the database at path in WAL mode with foreign keys on, applies
// the pool settings, and pings it. The caller owns migrations and defaults.
//
// Write transactions begin with BEGIN IMMEDIATE (_txlock=immediate). Several
// writers, in one process or across the CLI and the desktop, run
// transactions that read before they write. With the driver default
// (deferred), two such transactions can both hold a read lock and then both
// try to upgrade to the write lock. SQLite resolves that deadlock by
// returning SQLITE_BUSY at once and ignoring busy_timeout, because waiting
// could never succeed. IMMEDIATE takes the write lock up front, so a second
// writer waits on busy_timeout instead of failing.
func Open(ctx context.Context, path string, opts Options) (*sql.DB, error) {
	return open(ctx, dataSourceName(path, "immediate", opts.BusyTimeout), opts)
}

func dataSourceName(path, txlock string, busyTimeout time.Duration) string {
	return fmt.Sprintf(
		"file:%s?_txlock=%s&_pragma=journal_mode(WAL)&_pragma=busy_timeout(%d)&_pragma=foreign_keys(ON)",
		path, txlock, busyTimeout.Milliseconds(),
	)
}

func open(ctx context.Context, dsn string, opts Options) (*sql.DB, error) {
	conn, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	conn.SetMaxOpenConns(opts.MaxOpenConns)
	conn.SetMaxIdleConns(opts.MaxIdleConns)
	conn.SetConnMaxLifetime(0)

	if err := conn.PingContext(ctx); err != nil {
		if closeErr := conn.Close(); closeErr != nil {
			return nil, fmt.Errorf("failed to connect to database: %w (close also failed: %w)", err, closeErr)
		}
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}
	return conn, nil
}
