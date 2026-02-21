package db

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"sort"
	"strconv"
	"strings"
	"time"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// Migration represents a single versioned migration with up and down SQL.
type Migration struct {
	Version int
	Name    string
	UpSQL   string
	DownSQL string
}

// loadMigrations parses embedded SQL files into a sorted slice of migrations.
// Validates strict naming format, uniqueness, and up/down pairing.
func loadMigrations() ([]Migration, error) {
	entries, err := fs.ReadDir(migrationsFS, "migrations")
	if err != nil {
		return nil, fmt.Errorf("reading migrations directory: %w", err)
	}

	type half struct {
		name string
		sql  string
	}
	ups := make(map[int]half)
	downs := make(map[int]half)

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		fname := entry.Name()

		version, name, direction, err := parseFilename(fname)
		if err != nil {
			return nil, fmt.Errorf("invalid migration filename %q: %w", fname, err)
		}

		content, err := fs.ReadFile(migrationsFS, "migrations/"+fname)
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", fname, err)
		}

		target := ups
		if direction == "down" {
			target = downs
		}

		if _, exists := target[version]; exists {
			return nil, fmt.Errorf("duplicate %s migration for version %04d", direction, version)
		}
		target[version] = half{name: name, sql: string(content)}
	}

	// Validate pairing.
	if len(ups) != len(downs) {
		return nil, fmt.Errorf("migration count mismatch: %d up files, %d down files", len(ups), len(downs))
	}

	var migrations []Migration
	for version, up := range ups {
		down, ok := downs[version]
		if !ok {
			return nil, fmt.Errorf("migration %04d has up file but no down file", version)
		}
		migrations = append(migrations, Migration{
			Version: version,
			Name:    up.name,
			UpSQL:   up.sql,
			DownSQL: down.sql,
		})
	}

	for version := range downs {
		if _, ok := ups[version]; !ok {
			return nil, fmt.Errorf("migration %04d has down file but no up file", version)
		}
	}

	sort.Slice(migrations, func(i, j int) bool {
		return migrations[i].Version < migrations[j].Version
	})

	return migrations, nil
}

// parseFilename extracts version, name, and direction from "NNNN_name.up.sql" or "NNNN_name.down.sql".
func parseFilename(filename string) (int, string, string, error) {
	var direction string
	switch {
	case strings.HasSuffix(filename, ".up.sql"):
		direction = "up"
		filename = strings.TrimSuffix(filename, ".up.sql")
	case strings.HasSuffix(filename, ".down.sql"):
		direction = "down"
		filename = strings.TrimSuffix(filename, ".down.sql")
	default:
		return 0, "", "", fmt.Errorf("expected .up.sql or .down.sql suffix, got %q", filename)
	}

	parts := strings.SplitN(filename, "_", 2)
	if len(parts) != 2 || parts[1] == "" {
		return 0, "", "", fmt.Errorf("expected format NNNN_name.{up,down}.sql")
	}

	version, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, "", "", fmt.Errorf("version %q is not a valid integer: %w", parts[0], err)
	}
	if version <= 0 {
		return 0, "", "", fmt.Errorf("version must be positive, got %d", version)
	}

	return version, parts[1], direction, nil
}

// migrateUp applies all pending up migrations in version order.
// Creates the schema_migrations table if needed and bootstraps from legacy schema_version.
func migrateUp(ctx context.Context, conn *sql.DB) error {
	migrations, err := loadMigrations()
	if err != nil {
		return fmt.Errorf("loading migrations: %w", err)
	}

	if err := ensureMigrationsTable(ctx, conn); err != nil {
		return err
	}

	if err := bootstrapFromLegacy(ctx, conn, migrations); err != nil {
		return err
	}

	applied, err := appliedVersions(ctx, conn)
	if err != nil {
		return err
	}

	for _, m := range migrations {
		if applied[m.Version] {
			continue
		}

		slog.Info("applying migration", "version", m.Version, "name", m.Name)
		if err := applyMigration(ctx, conn, m.Version, m.Name, m.UpSQL); err != nil {
			return fmt.Errorf("migration %04d (%s): %w", m.Version, m.Name, err)
		}
	}

	return nil
}

// MigrateDown reverts the last n applied migrations in reverse version order.
func MigrateDown(ctx context.Context, conn *sql.DB, n int) error {
	if n <= 0 {
		return fmt.Errorf("n must be positive, got %d", n)
	}

	migrations, err := loadMigrations()
	if err != nil {
		return fmt.Errorf("loading migrations: %w", err)
	}

	if err := ensureMigrationsTable(ctx, conn); err != nil {
		return err
	}

	applied, err := appliedVersions(ctx, conn)
	if err != nil {
		return err
	}

	// Build list of applied migrations in reverse order.
	var toRevert []Migration
	for i := len(migrations) - 1; i >= 0; i-- {
		if applied[migrations[i].Version] {
			toRevert = append(toRevert, migrations[i])
		}
	}

	if n > len(toRevert) {
		return fmt.Errorf("requested %d down migrations but only %d are applied", n, len(toRevert))
	}

	for _, m := range toRevert[:n] {
		slog.Info("reverting migration", "version", m.Version, "name", m.Name)
		if err := revertMigration(ctx, conn, m.Version, m.DownSQL); err != nil {
			return fmt.Errorf("revert migration %04d (%s): %w", m.Version, m.Name, err)
		}
	}

	return nil
}

// ensureMigrationsTable creates the schema_migrations tracking table.
func ensureMigrationsTable(ctx context.Context, conn *sql.DB) error {
	_, err := conn.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version    INTEGER PRIMARY KEY,
			name       TEXT NOT NULL,
			applied_at INTEGER NOT NULL
		)
	`)
	if err != nil {
		return fmt.Errorf("creating schema_migrations table: %w", err)
	}
	return nil
}

// bootstrapFromLegacy checks for the legacy schema_version table and seeds
// schema_migrations for all migrations up to that version. This preserves
// backward compatibility with databases created before the migration framework.
func bootstrapFromLegacy(ctx context.Context, conn *sql.DB, migrations []Migration) error {
	// Check if schema_migrations already has rows — if so, bootstrap is done.
	var count int
	err := conn.QueryRowContext(ctx, "SELECT COUNT(*) FROM schema_migrations").Scan(&count)
	if err != nil {
		return fmt.Errorf("checking schema_migrations count: %w", err)
	}
	if count > 0 {
		return nil
	}

	// Check if legacy schema_version table exists.
	var tableName string
	err = conn.QueryRowContext(ctx,
		"SELECT name FROM sqlite_master WHERE type='table' AND name='schema_version'",
	).Scan(&tableName)
	if errors.Is(err, sql.ErrNoRows) {
		// No legacy table — fresh database, nothing to bootstrap.
		return nil
	}
	if err != nil {
		return fmt.Errorf("checking for legacy schema_version table: %w", err)
	}

	// Read the legacy version.
	var legacyVersion int
	err = conn.QueryRowContext(ctx, "SELECT MAX(version) FROM schema_version").Scan(&legacyVersion)
	if err != nil {
		return fmt.Errorf("reading legacy schema_version: %w", err)
	}

	slog.Info("bootstrapping from legacy schema_version", "legacy_version", legacyVersion)

	// Mark all migrations up to legacyVersion as applied.
	now := time.Now().UnixNano()
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin bootstrap transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	for _, m := range migrations {
		if m.Version > legacyVersion {
			break
		}
		_, err := tx.ExecContext(ctx,
			"INSERT OR IGNORE INTO schema_migrations (version, name, applied_at) VALUES (?, ?, ?)",
			m.Version, m.Name, now,
		)
		if err != nil {
			return fmt.Errorf("bootstrap migration %04d: %w", m.Version, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit bootstrap transaction: %w", err)
	}

	return nil
}

// appliedVersions returns a set of already-applied migration versions.
func appliedVersions(ctx context.Context, conn *sql.DB) (map[int]bool, error) {
	rows, err := conn.QueryContext(ctx, "SELECT version FROM schema_migrations")
	if err != nil {
		return nil, fmt.Errorf("querying applied versions: %w", err)
	}
	defer func() { _ = rows.Close() }()

	applied := make(map[int]bool)
	for rows.Next() {
		var v int
		if err := rows.Scan(&v); err != nil {
			return nil, fmt.Errorf("scanning version: %w", err)
		}
		applied[v] = true
	}
	return applied, rows.Err()
}

// applyMigration executes up SQL and records the version in one transaction.
func applyMigration(ctx context.Context, conn *sql.DB, version int, name, sqlStr string) error {
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, sqlStr); err != nil {
		return fmt.Errorf("executing SQL: %w", err)
	}

	_, err = tx.ExecContext(ctx,
		"INSERT INTO schema_migrations (version, name, applied_at) VALUES (?, ?, ?)",
		version, name, time.Now().UnixNano(),
	)
	if err != nil {
		return fmt.Errorf("recording migration: %w", err)
	}

	return tx.Commit()
}

// revertMigration executes down SQL and removes the version record in one transaction.
func revertMigration(ctx context.Context, conn *sql.DB, version int, sqlStr string) error {
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, sqlStr); err != nil {
		return fmt.Errorf("executing SQL: %w", err)
	}

	if _, err := tx.ExecContext(ctx, "DELETE FROM schema_migrations WHERE version = ?", version); err != nil {
		return fmt.Errorf("removing migration record: %w", err)
	}

	return tx.Commit()
}
