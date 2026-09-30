package queries

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCompactionPolicyRequiresRatioAndAbsoluteSpace(t *testing.T) {
	policy := DefaultCompactionPolicy()
	require.InDelta(t, 0.20, policy.MinimumReclaimableRatio, 0.0001)
	require.Equal(t, int64(16<<20), policy.MinimumReclaimableBytes)

	tests := []struct {
		name   string
		result CompactionResult
		want   bool
	}{
		{
			name:   "empty database",
			result: CompactionResult{},
		},
		{
			name: "no free pages",
			result: CompactionResult{
				PageCount: 100,
				PageSize:  4096,
			},
		},
		{
			name: "below ratio",
			result: CompactionResult{
				PageCount:        1000,
				FreelistCount:    199,
				PageSize:         1 << 20,
				ReclaimableBytes: 199 << 20,
			},
		},
		{
			name: "below absolute floor",
			result: CompactionResult{
				PageCount:        100,
				FreelistCount:    50,
				PageSize:         4096,
				ReclaimableBytes: 50 * 4096,
			},
		},
		{
			name: "at both thresholds",
			result: CompactionResult{
				PageCount:        100,
				FreelistCount:    20,
				PageSize:         1 << 20,
				ReclaimableBytes: 20 << 20,
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, policy.shouldCompact(tt.result))
		})
	}
}

func TestCompactSkipsBelowAbsoluteFloor(t *testing.T) {
	dir, database := prepareCompactionDatabase(t, 3, 2, DefaultOpenOptions())
	before, err := os.Stat(DatabasePath(dir))
	require.NoError(t, err)

	result, err := database.Compact(t.Context(), CompactionPolicy{
		MinimumReclaimableRatio: 0.20,
		MinimumReclaimableBytes: 4 << 20,
	})
	require.NoError(t, err)

	after, err := os.Stat(DatabasePath(dir))
	require.NoError(t, err)
	assert.False(t, result.Compacted)
	assert.GreaterOrEqual(t, float64(result.FreelistCount)/float64(result.PageCount), 0.20)
	assert.Less(t, result.ReclaimableBytes, int64(4<<20))
	assert.Equal(t, before.Size(), after.Size())
}

func TestCompactRunsAndShrinksDatabase(t *testing.T) {
	dir, database := prepareCompactionDatabase(t, 12, 11, DefaultOpenOptions())
	before, err := os.Stat(DatabasePath(dir))
	require.NoError(t, err)

	result, err := database.Compact(t.Context(), CompactionPolicy{
		MinimumReclaimableRatio: 0.20,
		MinimumReclaimableBytes: 1 << 20,
	})
	require.NoError(t, err)

	after, err := os.Stat(DatabasePath(dir))
	require.NoError(t, err)
	assert.True(t, result.Compacted)
	assert.Greater(t, result.ReclaimableBytes, int64(1<<20))
	assert.Less(t, after.Size(), before.Size())

	var rows int
	require.NoError(t, database.Conn().QueryRowContext(t.Context(), "SELECT COUNT(*) FROM compaction_fixture").Scan(&rows))
	assert.Equal(t, 1, rows)
}

func TestCompactReturnsVacuumFailure(t *testing.T) {
	opts := DefaultOpenOptions()
	opts.BusyTimeout = 10
	_, database := prepareCompactionDatabase(t, 8, 7, opts)

	tx, err := database.Conn().BeginTx(t.Context(), nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = tx.Rollback() })
	_, err = tx.ExecContext(t.Context(), "INSERT INTO compaction_fixture(payload) VALUES (zeroblob(1024))")
	require.NoError(t, err)

	result, err := database.Compact(t.Context(), CompactionPolicy{
		MinimumReclaimableRatio: 0,
		MinimumReclaimableBytes: 0,
	})
	require.ErrorContains(t, err, "vacuum database")
	assert.False(t, result.Compacted)
}

func prepareCompactionDatabase(t *testing.T, rows, deleted int, opts OpenOptions) (string, *DB) {
	t.Helper()
	dir := t.TempDir()
	ctx := t.Context()

	database, err := Open(ctx, dir, opts)
	require.NoError(t, err)
	_, err = database.Conn().ExecContext(ctx, "CREATE TABLE compaction_fixture (id INTEGER PRIMARY KEY, payload BLOB NOT NULL)")
	require.NoError(t, err)
	for range rows {
		_, err = database.Conn().ExecContext(ctx, "INSERT INTO compaction_fixture(payload) VALUES (zeroblob(?))", 1<<20)
		require.NoError(t, err)
	}
	require.NoError(t, database.Close())

	database, err = Open(ctx, dir, opts)
	require.NoError(t, err)
	_, err = database.Conn().ExecContext(ctx, "DELETE FROM compaction_fixture WHERE id <= ?", deleted)
	require.NoError(t, err)
	require.NoError(t, database.Close())

	database, err = Open(t.Context(), dir, opts)
	require.NoError(t, err)
	t.Cleanup(func() { _ = database.Close() })
	return dir, database
}
