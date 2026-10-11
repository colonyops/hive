package logutils

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"sync"
	"time"
)

const (
	DefaultMaxFileSize = 10 << 20
	DefaultFileBackups = 3

	rotateCheckBytes    = 256 << 10
	rotateCheckInterval = 30 * time.Second
)

// rotatingFile appends to path and rotates it to path.1 … path.N once it
// reaches maxSize. Every hive process appends to the same file, so the size
// and identity of path are re-read from disk rather than counted: rotation
// runs under an flock, and a writer whose path was rotated by another process
// reopens it.
type rotatingFile struct {
	path       string
	maxSize    int64
	backups    int
	checkBytes int64

	mu          sync.Mutex
	file        *os.File
	sinceCheck  int64
	lastChecked time.Time
}

func openRotating(path string, maxSize int64, backups int) (*rotatingFile, error) {
	r := &rotatingFile{
		path:       path,
		maxSize:    maxSize,
		backups:    backups,
		checkBytes: min(rotateCheckBytes, max(maxSize/8, 1)),
	}
	// A failed rotation leaves the file growing; it must not cost the log.
	_ = rotate(path, maxSize, backups)
	f, err := openAppend(path)
	if err != nil {
		return nil, err
	}
	r.file = f
	r.lastChecked = time.Now()
	return r, nil
}

func (r *rotatingFile) Write(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.file == nil {
		return 0, os.ErrClosed
	}
	n, err := r.file.Write(p)
	r.sinceCheck += int64(n)
	if r.sinceCheck >= r.checkBytes || time.Since(r.lastChecked) >= rotateCheckInterval {
		r.check()
	}
	return n, err
}

func (r *rotatingFile) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.file == nil {
		return nil
	}
	err := r.file.Close()
	r.file = nil
	return err
}

func (r *rotatingFile) check() {
	r.sinceCheck = 0
	r.lastChecked = time.Now()

	held, err := r.file.Stat()
	if err != nil {
		return
	}
	current, err := os.Stat(r.path)
	if err == nil && os.SameFile(held, current) {
		if current.Size() < r.maxSize {
			return
		}
		_ = rotate(r.path, r.maxSize, r.backups)
	}
	f, err := openAppend(r.path)
	if err != nil {
		return
	}
	_ = r.file.Close()
	r.file = f
}

// rotate moves path to path.1, shifting older backups up and dropping the
// one past backups. It re-checks the size under the lock because another
// process may have rotated first.
func rotate(path string, maxSize int64, backups int) error {
	unlock, err := lockFile(path + ".lock")
	if err != nil {
		return fmt.Errorf("lock log file: %w", err)
	}
	defer unlock()

	info, err := os.Stat(path)
	if err != nil || info.Size() < maxSize {
		return nil
	}
	if backups < 1 {
		return os.Remove(path)
	}
	for i := backups; i > 1; i-- {
		if err := os.Rename(backupPath(path, i-1), backupPath(path, i)); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("rotate log file: %w", err)
		}
	}
	if err := os.Rename(path, backupPath(path, 1)); err != nil {
		return fmt.Errorf("rotate log file: %w", err)
	}
	return nil
}

func backupPath(path string, n int) string {
	return fmt.Sprintf("%s.%d", path, n)
}
