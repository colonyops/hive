package stores

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"
)

// IsBusyError returns true if the error is a SQLITE_BUSY error.
func IsBusyError(err error) bool {
	var sqliteErr *sqlite.Error
	if errors.As(err, &sqliteErr) {
		return sqliteErr.Code() == sqlite3.SQLITE_BUSY
	}
	return false
}

// IsCorruptionError returns true if the error indicates database corruption.
func IsCorruptionError(err error) bool {
	var sqliteErr *sqlite.Error
	if errors.As(err, &sqliteErr) {
		code := sqliteErr.Code()
		return code == sqlite3.SQLITE_CORRUPT ||
			code == sqlite3.SQLITE_NOTADB ||
			code == sqlite3.SQLITE_CANTOPEN
	}

	// Also check for common corruption error messages
	errStr := err.Error()
	return strings.Contains(errStr, "database disk image is malformed") ||
		strings.Contains(errStr, "file is not a database") ||
		strings.Contains(errStr, "database corruption")
}

// IsNotFoundError returns true if the error is a "not found" error.
func IsNotFoundError(err error) bool {
	return errors.Is(err, sql.ErrNoRows)
}

// RecoverFromCorruption attempts to recover from database corruption by backing up
// the corrupted file and creating a new database.
func RecoverFromCorruption(dataDir string) error {
	dbPath := filepath.Join(dataDir, "hive.db")

	// Create backup filename with timestamp
	timestamp := time.Now().Format("20060102-150405")
	backupPath := filepath.Join(dataDir, fmt.Sprintf("hive.db.corrupt.%s", timestamp))

	// Backup the corrupted database
	if err := os.Rename(dbPath, backupPath); err != nil {
		// If file doesn't exist, that's ok
		if !os.IsNotExist(err) {
			return fmt.Errorf("failed to backup corrupted database: %w", err)
		}
	}

	// Also backup WAL and SHM files if they exist
	// These MUST be moved/deleted for recovery to work, otherwise SQLite
	// will find orphaned WAL/SHM files that don't match the new database
	walPath := dbPath + "-wal"
	if _, err := os.Stat(walPath); err == nil {
		walBackup := backupPath + "-wal"
		if err := os.Rename(walPath, walBackup); err != nil {
			// If rename fails, try to delete the file instead
			if delErr := os.Remove(walPath); delErr != nil {
				return fmt.Errorf("failed to backup or remove WAL file: %w", err)
			}
		}
	}

	shmPath := dbPath + "-shm"
	if _, err := os.Stat(shmPath); err == nil {
		shmBackup := backupPath + "-shm"
		if err := os.Rename(shmPath, shmBackup); err != nil {
			// If rename fails, try to delete the file instead
			if delErr := os.Remove(shmPath); delErr != nil {
				return fmt.Errorf("failed to backup or remove SHM file: %w", err)
			}
		}
	}

	return nil
}
