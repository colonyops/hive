// Package migrate is a small SQLite migration runner shared across hive's
// databases. It applies embedded, versioned up/down SQL migrations
// (NNNN_name.{up,down}.sql) tracked in a schema_migrations table, one
// transaction per migration.
//
// It is deliberately storage-agnostic: callers pass an fs.FS rooted at their
// migration files (e.g. fs.Sub(embedFS, "migrations")), so each database owns
// its own migration set while sharing this runner. Any bootstrapping specific
// to a database (such as seeding from a legacy version table) is the caller's
// responsibility: load the migrations, EnsureTable, do the bootstrap, then
// Apply.
package migrate

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"
	"log/slog"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Migration is a single versioned migration with its up and down SQL.
type Migration struct {
	Version int
	Name    string
	UpSQL   string
	DownSQL string
}

// Load parses NNNN_name.{up,down}.sql pairs from the root of fsys into a
// version-sorted slice. It validates the naming format, rejects duplicates,
// and requires every version to have a matching up/down pair with equal names.
func Load(fsys fs.FS) ([]Migration, error) {
	entries, err := fs.ReadDir(fsys, ".")
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

		content, err := fs.ReadFile(fsys, fname)
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

	if len(ups) != len(downs) {
		return nil, fmt.Errorf("migration count mismatch: %d up files, %d down files", len(ups), len(downs))
	}

	migrations := make([]Migration, 0, len(ups))
	for version, up := range ups {
		down, ok := downs[version]
		if !ok {
			return nil, fmt.Errorf("migration %04d has up file but no down file", version)
		}
		if up.name != down.name {
			return nil, fmt.Errorf("migration %04d name mismatch: up=%q down=%q", version, up.name, down.name)
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

// parseFilename extracts version, name, and direction from "NNNN_name.up.sql"
// or "NNNN_name.down.sql".
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

// Up loads the migrations from fsys and applies all pending ones in version
// order. It does not perform any legacy bootstrapping; callers needing that
// should Load + EnsureTable + bootstrap + Apply instead.
func Up(ctx context.Context, conn *sql.DB, fsys fs.FS) error {
	migrations, err := Load(fsys)
	if err != nil {
		return fmt.Errorf("loading migrations: %w", err)
	}
	return Apply(ctx, conn, migrations)
}

// Apply ensures the tracking table exists, then applies every migration not
// already recorded, in ascending version order, one transaction each.
func Apply(ctx context.Context, conn *sql.DB, migrations []Migration) error {
	if err := EnsureTable(ctx, conn); err != nil {
		return err
	}

	applied, err := AppliedVersions(ctx, conn)
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

// Down reverts the last n applied migrations from fsys in reverse version order.
func Down(ctx context.Context, conn *sql.DB, fsys fs.FS, n int) error {
	if n <= 0 {
		return fmt.Errorf("n must be positive, got %d", n)
	}

	migrations, err := Load(fsys)
	if err != nil {
		return fmt.Errorf("loading migrations: %w", err)
	}

	if err := EnsureTable(ctx, conn); err != nil {
		return err
	}

	applied, err := AppliedVersions(ctx, conn)
	if err != nil {
		return err
	}

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

// EnsureTable creates the schema_migrations tracking table if it does not exist.
func EnsureTable(ctx context.Context, conn *sql.DB) error {
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

// AppliedVersions returns the set of already-applied migration versions.
func AppliedVersions(ctx context.Context, conn *sql.DB) (map[int]bool, error) {
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

// applyMigration executes upSQL and records the version in one transaction.
func applyMigration(ctx context.Context, conn *sql.DB, version int, name, upSQL string) error {
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, upSQL); err != nil {
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

// revertMigration executes downSQL and removes the version record in one transaction.
func revertMigration(ctx context.Context, conn *sql.DB, version int, downSQL string) error {
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, downSQL); err != nil {
		return fmt.Errorf("executing SQL: %w", err)
	}

	if _, err := tx.ExecContext(ctx, "DELETE FROM schema_migrations WHERE version = ?", version); err != nil {
		return fmt.Errorf("removing migration record: %w", err)
	}

	return tx.Commit()
}
