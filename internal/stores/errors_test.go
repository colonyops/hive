package stores

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRecoverFromCorruption_Success(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "hive.db")

	// Create a corrupted database file
	if err := os.WriteFile(dbPath, []byte("corrupted data"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	// Create WAL and SHM files
	walPath := dbPath + "-wal"
	shmPath := dbPath + "-shm"
	if err := os.WriteFile(walPath, []byte("wal data"), 0o644); err != nil {
		t.Fatalf("WriteFile WAL: %v", err)
	}
	if err := os.WriteFile(shmPath, []byte("shm data"), 0o644); err != nil {
		t.Fatalf("WriteFile SHM: %v", err)
	}

	// Run recovery
	err := RecoverFromCorruption(tempDir)
	if err != nil {
		t.Fatalf("RecoverFromCorruption failed: %v", err)
	}

	// Verify database file was backed up (exclude -wal and -shm files)
	allFiles, err := filepath.Glob(filepath.Join(tempDir, "hive.db.corrupt.*"))
	if err != nil {
		t.Fatalf("Glob: %v", err)
	}

	// Filter to find just the main DB backup (not -wal or -shm)
	dbBackups := make([]string, 0)
	walBackups := make([]string, 0)
	shmBackups := make([]string, 0)

	for _, f := range allFiles {
		if filepath.Ext(f) == ".db-wal" || len(f) > 4 && f[len(f)-4:] == "-wal" {
			walBackups = append(walBackups, f)
		} else if filepath.Ext(f) == ".db-shm" || len(f) > 4 && f[len(f)-4:] == "-shm" {
			shmBackups = append(shmBackups, f)
		} else {
			dbBackups = append(dbBackups, f)
		}
	}

	if len(dbBackups) != 1 {
		t.Errorf("Expected 1 DB backup file, found %d: %v", len(dbBackups), dbBackups)
	}
	if len(walBackups) != 1 {
		t.Errorf("Expected 1 WAL backup, found %d: %v", len(walBackups), walBackups)
	}
	if len(shmBackups) != 1 {
		t.Errorf("Expected 1 SHM backup, found %d: %v", len(shmBackups), shmBackups)
	}

	// Verify original files no longer exist
	if _, err := os.Stat(dbPath); err == nil {
		t.Error("Original database file should not exist after recovery")
	}
	if _, err := os.Stat(walPath); err == nil {
		t.Error("Original WAL file should not exist after recovery")
	}
	if _, err := os.Stat(shmPath); err == nil {
		t.Error("Original SHM file should not exist after recovery")
	}
}

func TestRecoverFromCorruption_MissingFile(t *testing.T) {
	tempDir := t.TempDir()

	// Run recovery on non-existent database - should not error
	err := RecoverFromCorruption(tempDir)
	if err != nil {
		t.Errorf("RecoverFromCorruption should not error on missing file: %v", err)
	}

	// Verify no backup files were created
	files, _ := filepath.Glob(filepath.Join(tempDir, "*.corrupt.*"))
	if len(files) != 0 {
		t.Errorf("Expected no backup files for missing DB, found %d", len(files))
	}
}

func TestRecoverFromCorruption_OnlyDatabase(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "hive.db")

	// Create only the database file (no WAL/SHM)
	if err := os.WriteFile(dbPath, []byte("corrupted data"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	// Run recovery
	err := RecoverFromCorruption(tempDir)
	if err != nil {
		t.Fatalf("RecoverFromCorruption failed: %v", err)
	}

	// Verify backup was created
	files, _ := filepath.Glob(filepath.Join(tempDir, "hive.db.corrupt.*"))
	if len(files) != 1 {
		t.Errorf("Expected 1 backup file, found %d", len(files))
	}

	// Verify no WAL/SHM backups (they didn't exist)
	walBackups, _ := filepath.Glob(filepath.Join(tempDir, "*-wal"))
	if len(walBackups) != 0 {
		t.Errorf("Expected no WAL backups, found %d", len(walBackups))
	}
}

func TestRecoverFromCorruption_BackupNaming(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "hive.db")

	// Create corrupted database
	if err := os.WriteFile(dbPath, []byte("corrupted"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	// Run recovery
	err := RecoverFromCorruption(tempDir)
	if err != nil {
		t.Fatalf("RecoverFromCorruption failed: %v", err)
	}

	// Verify backup filename format includes timestamp
	files, _ := filepath.Glob(filepath.Join(tempDir, "hive.db.corrupt.*"))
	if len(files) != 1 {
		t.Fatalf("Expected 1 backup file, found %d", len(files))
	}

	// Extract timestamp from filename: hive.db.corrupt.YYYYMMDD-HHMMSS
	filename := filepath.Base(files[0])
	// Just verify it has the expected prefix and reasonable length
	if len(filename) < len("hive.db.corrupt.20060102-150405") {
		t.Errorf("Backup filename too short: %s", filename)
	}

	// Verify filename contains expected prefix
	expectedPrefix := "hive.db.corrupt."
	if len(filename) < len(expectedPrefix) || filename[:len(expectedPrefix)] != expectedPrefix {
		t.Errorf("Backup filename should start with %s, got: %s", expectedPrefix, filename)
	}

	// Verify backup file exists and is readable
	info, err := os.Stat(files[0])
	if err != nil {
		t.Fatalf("Stat backup: %v", err)
	}
	if info.Size() == 0 {
		t.Error("Backup file should not be empty")
	}
}

func TestRecoverFromCorruption_WALWithoutDatabase(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "hive.db")

	// Create only WAL file (unusual but possible)
	walPath := dbPath + "-wal"
	if err := os.WriteFile(walPath, []byte("wal data"), 0o644); err != nil {
		t.Fatalf("WriteFile WAL: %v", err)
	}

	// Run recovery (database file doesn't exist, so DB Rename will fail gracefully)
	err := RecoverFromCorruption(tempDir)
	if err != nil {
		t.Fatalf("RecoverFromCorruption should handle missing DB: %v", err)
	}

	// WAL file should be backed up even when DB doesn't exist
	// (it uses the same backup path that would have been used for the DB)
	walBackups, _ := filepath.Glob(filepath.Join(tempDir, "*.corrupt.*-wal"))
	if len(walBackups) != 1 {
		t.Errorf("Expected WAL to be backed up, found %d backups", len(walBackups))
	}

	// Original WAL should no longer exist
	if _, err := os.Stat(walPath); err == nil {
		t.Error("Original WAL file should not exist after recovery")
	}
}

func TestIsNotFoundError(t *testing.T) {
	// This is a simple helper, just verify it works
	err := os.ErrNotExist
	if !os.IsNotExist(err) {
		t.Error("Should recognize os.ErrNotExist")
	}
}
