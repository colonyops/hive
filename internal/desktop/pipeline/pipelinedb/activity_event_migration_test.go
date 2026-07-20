package pipelinedb

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/colonyops/hive/internal/data/migrate"
	"github.com/stretchr/testify/require"
)

// A parallel feature branch claimed migration version 8 for an unrelated table,
// and a build from that branch had already advanced a shared desktop-pipeline.db
// to version 8. The runner keys on version *number*, so the activity_event
// migration must not reuse 8 — it would be recorded as "already applied" and
// skipped, leaving the table missing ("no such table: activity_event"). It is 9.
//
// This reproduces that divergent state (schema_migrations already at 8, no
// activity_event) and asserts Open still creates the table. It would fail if the
// migration were ever renumbered back onto an already-applied version.
func TestActivityEventMigrationAppliesOverDivergentVersion(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "desktop-pipeline.db")
	ctx := context.Background()

	seed, err := sql.Open("sqlite", "file:"+dbPath)
	require.NoError(t, err)
	require.NoError(t, migrate.EnsureTable(ctx, seed))
	for v := 1; v <= 8; v++ {
		_, err := seed.ExecContext(ctx,
			"INSERT INTO schema_migrations(version, name, applied_at) VALUES(?, ?, ?)",
			v, "prior-branch", int64(v))
		require.NoError(t, err)
	}
	require.NoError(t, seed.Close())

	db, err := Open(dir, DefaultOpenOptions())
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	// The table exists and is writable — the migration applied as version 9,
	// despite version 8 already being recorded by the other branch.
	rec, err := db.AppendActivityEvent(ctx, ActivityRecord{
		CreatedAt: time.Now().UnixMilli(),
		Category:  "system",
		Severity:  "info",
		Title:     "migrated over divergent version",
	})
	require.NoError(t, err)
	require.NotZero(t, rec.ID)
}
