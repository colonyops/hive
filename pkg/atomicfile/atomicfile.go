// Package atomicfile replaces files so a reader never sees a partial write.
package atomicfile

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// Write replaces path with data and perm. It writes a temporary file in the
// same directory, syncs it, and renames it over path, so a crash leaves
// either the old file or the new one. The parent directory must exist.
//
// The temporary file is hidden and ends in .tmp, so a directory watcher that
// skips dotfiles or matches on extension never loads it.
func Write(path string, data []byte, perm fs.FileMode) (err error) {
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".*.tmp")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmp.Name()
	defer func() {
		if err != nil {
			_ = tmp.Close()
			_ = os.Remove(tmpPath)
		}
	}()

	if err := tmp.Chmod(perm); err != nil {
		return fmt.Errorf("chmod temp file: %w", err)
	}
	if _, err := tmp.Write(data); err != nil {
		return fmt.Errorf("write temp file: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		return fmt.Errorf("sync temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp file: %w", err)
	}
	return os.Rename(tmpPath, path)
}
