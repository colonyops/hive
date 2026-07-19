package db

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"time"

	"github.com/colonyops/hive/internal/data/migrate"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// migrationsSub returns the embedded migrations rooted at their directory, so
// the storage-agnostic migrate runner sees the files at the FS root.
func migrationsSub() (fs.FS, error) {
	return fs.Sub(migrationsFS, "migrations")
}

// runMigrations applies all pending migrations to the hive database, seeding
// schema_migrations from the legacy schema_version table on first run so
// databases created before the migration framework are not re-applied.
func runMigrations(ctx context.Context, conn *sql.DB) error {
	sub, err := migrationsSub()
	if err != nil {
		return fmt.Errorf("opening migrations fs: %w", err)
	}

	migrations, err := migrate.Load(sub)
	if err != nil {
		return fmt.Errorf("loading migrations: %w", err)
	}

	if err := migrate.EnsureTable(ctx, conn); err != nil {
		return err
	}

	if err := bootstrapFromLegacy(ctx, conn, migrations); err != nil {
		return err
	}

	return migrate.Apply(ctx, conn, migrations)
}

// bootstrapFromLegacy checks for the legacy schema_version table and seeds
// schema_migrations for all migrations up to that version. This preserves
// backward compatibility with databases created before the migration framework.
func bootstrapFromLegacy(ctx context.Context, conn *sql.DB, migrations []migrate.Migration) error {
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

	// Read the legacy version. MAX returns NULL if the table is empty.
	var legacyVersion sql.NullInt64
	err = conn.QueryRowContext(ctx, "SELECT MAX(version) FROM schema_version").Scan(&legacyVersion)
	if err != nil {
		return fmt.Errorf("reading legacy schema_version: %w", err)
	}
	if !legacyVersion.Valid {
		// schema_version table exists but is empty — nothing to bootstrap.
		return nil
	}

	maxVersion := int(legacyVersion.Int64)
	slog.Info("bootstrapping from legacy schema_version", "legacy_version", maxVersion)

	// Mark all migrations up to maxVersion as applied.
	now := time.Now().UnixNano()
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin bootstrap transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	for _, m := range migrations {
		if m.Version > maxVersion {
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
