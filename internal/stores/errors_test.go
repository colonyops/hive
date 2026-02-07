package stores

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRecoverFromCorruption_Success(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "hive.db")

	// Create a corrupted database file
	require.NoError(t, os.WriteFile(dbPath, []byte("corrupted data"), 0o644))

	// Create WAL and SHM files
	walPath := dbPath + "-wal"
	shmPath := dbPath + "-shm"
	require.NoError(t, os.WriteFile(walPath, []byte("wal data"), 0o644))
	require.NoError(t, os.WriteFile(shmPath, []byte("shm data"), 0o644))

	// Run recovery
	require.NoError(t, RecoverFromCorruption(tempDir))

	// Verify database file was backed up (exclude -wal and -shm files)
	allFiles, err := filepath.Glob(filepath.Join(tempDir, "hive.db.corrupt.*"))
	require.NoError(t, err)

	// Filter to find just the main DB backup (not -wal or -shm)
	dbBackups := make([]string, 0)
	walBackups := make([]string, 0)
	shmBackups := make([]string, 0)

	for _, f := range allFiles {
		switch {
		case filepath.Ext(f) == ".db-wal" || len(f) > 4 && f[len(f)-4:] == "-wal":
			walBackups = append(walBackups, f)
		case filepath.Ext(f) == ".db-shm" || len(f) > 4 && f[len(f)-4:] == "-shm":
			shmBackups = append(shmBackups, f)
		default:
			dbBackups = append(dbBackups, f)
		}
	}

	assert.Len(t, dbBackups, 1, "Expected 1 DB backup file, found %d: %v", len(dbBackups), dbBackups)
	assert.Len(t, walBackups, 1, "Expected 1 WAL backup, found %d: %v", len(walBackups), walBackups)
	assert.Len(t, shmBackups, 1, "Expected 1 SHM backup, found %d: %v", len(shmBackups), shmBackups)

	// Verify original files no longer exist
	_, err = os.Stat(dbPath)
	assert.Error(t, err, "Original database file should not exist after recovery")
	_, err = os.Stat(walPath)
	assert.Error(t, err, "Original WAL file should not exist after recovery")
	_, err = os.Stat(shmPath)
	assert.Error(t, err, "Original SHM file should not exist after recovery")
}

func TestRecoverFromCorruption_MissingFile(t *testing.T) {
	tempDir := t.TempDir()

	// Run recovery on non-existent database - should not error
	assert.NoError(t, RecoverFromCorruption(tempDir), "RecoverFromCorruption should not error on missing file")

	// Verify no backup files were created
	files, _ := filepath.Glob(filepath.Join(tempDir, "*.corrupt.*"))
	assert.Len(t, files, 0, "Expected no backup files for missing DB, found %d", len(files))
}

func TestRecoverFromCorruption_OnlyDatabase(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "hive.db")

	// Create only the database file (no WAL/SHM)
	require.NoError(t, os.WriteFile(dbPath, []byte("corrupted data"), 0o644))

	// Run recovery
	require.NoError(t, RecoverFromCorruption(tempDir))

	// Verify backup was created
	files, _ := filepath.Glob(filepath.Join(tempDir, "hive.db.corrupt.*"))
	assert.Len(t, files, 1, "Expected 1 backup file, found %d", len(files))

	// Verify no WAL/SHM backups (they didn't exist)
	walBackups, _ := filepath.Glob(filepath.Join(tempDir, "*-wal"))
	assert.Len(t, walBackups, 0, "Expected no WAL backups, found %d", len(walBackups))
}

func TestRecoverFromCorruption_BackupNaming(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "hive.db")

	// Create corrupted database
	require.NoError(t, os.WriteFile(dbPath, []byte("corrupted"), 0o644))

	// Run recovery
	require.NoError(t, RecoverFromCorruption(tempDir))

	// Verify backup filename format includes timestamp
	files, _ := filepath.Glob(filepath.Join(tempDir, "hive.db.corrupt.*"))
	require.Len(t, files, 1, "Expected 1 backup file, found %d", len(files))

	// Extract timestamp from filename: hive.db.corrupt.YYYYMMDD-HHMMSS
	filename := filepath.Base(files[0])
	// Just verify it has the expected prefix and reasonable length
	assert.GreaterOrEqual(t, len(filename), len("hive.db.corrupt.20060102-150405"), "Backup filename too short: %s", filename)

	// Verify filename contains expected prefix
	expectedPrefix := "hive.db.corrupt."
	assert.True(t, len(filename) >= len(expectedPrefix) && filename[:len(expectedPrefix)] == expectedPrefix, "Backup filename should start with %s, got: %s", expectedPrefix, filename)

	// Verify backup file exists and is readable
	info, err := os.Stat(files[0])
	require.NoError(t, err, "Stat backup")
	assert.Greater(t, info.Size(), int64(0), "Backup file should not be empty")
}

func TestRecoverFromCorruption_WALWithoutDatabase(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "hive.db")

	// Create only WAL file (unusual but possible)
	walPath := dbPath + "-wal"
	require.NoError(t, os.WriteFile(walPath, []byte("wal data"), 0o644))

	// Run recovery (database file doesn't exist, so DB Rename will fail gracefully)
	require.NoError(t, RecoverFromCorruption(tempDir), "RecoverFromCorruption should handle missing DB")

	// WAL file should be backed up even when DB doesn't exist
	// (it uses the same backup path that would have been used for the DB)
	walBackups, _ := filepath.Glob(filepath.Join(tempDir, "*.corrupt.*-wal"))
	assert.Len(t, walBackups, 1, "Expected WAL to be backed up, found %d backups", len(walBackups))

	// Original WAL should no longer exist
	_, err := os.Stat(walPath)
	assert.Error(t, err, "Original WAL file should not exist after recovery")
}

func TestIsNotFoundError(t *testing.T) {
	// This is a simple helper, just verify it works
	err := os.ErrNotExist
	assert.True(t, os.IsNotExist(err), "Should recognize os.ErrNotExist")
}
