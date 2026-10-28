package daemon

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

// maxSocketPath stays under the 104-byte sun_path of macOS (Linux allows 108).
const maxSocketPath = 100

// fallbackSocketRoot is fixed, not os.TempDir: $TMPDIR differs between
// launchd, systemd, Desktop, and shells, and every client must find the
// same socket.
const fallbackSocketRoot = "/tmp"

// SocketPath is where the daemon for dataDir listens: <dataDir>/hived.sock,
// or /tmp/hive-<uid>/<hash>.sock when that path is too long for a unix
// socket.
func SocketPath(dataDir string) string {
	dataDir = absolute(dataDir)
	path := filepath.Join(dataDir, "hived.sock")
	if len(path) <= maxSocketPath {
		return path
	}
	sum := sha256.Sum256([]byte(dataDir))
	return filepath.Join(fallbackSocketDir(), fmt.Sprintf("%x.sock", sum[:6]))
}

// LockPath is the single-instance lock for the daemon of dataDir.
func LockPath(dataDir string) string {
	return filepath.Join(absolute(dataDir), "hived.lock")
}

func fallbackSocketDir() string {
	return filepath.Join(fallbackSocketRoot, fmt.Sprintf("hive-%d", os.Getuid()))
}

func absolute(path string) string {
	if abs, err := filepath.Abs(path); err == nil {
		return abs
	}
	return path
}

// fileOwner is a variable so a test can stand in another user.
var fileOwner = func(info os.FileInfo) (int, bool) {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, false
	}
	return int(stat.Uid), true
}

// prepareSocketDir makes the directory a socket lives in. The shared
// fallback directory must be ours and private: another user who created
// /tmp/hive-<uid> first could otherwise read the socket's traffic.
func prepareSocketDir(socketPath string) error {
	dir := filepath.Dir(socketPath)
	if dir != fallbackSocketDir() {
		return os.MkdirAll(dir, 0o755)
	}
	if err := os.Mkdir(dir, 0o700); err != nil && !errors.Is(err, os.ErrExist) {
		return fmt.Errorf("create socket directory: %w", err)
	}
	info, err := os.Lstat(dir)
	if err != nil {
		return fmt.Errorf("inspect socket directory: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("socket directory %s is not a directory", dir)
	}
	owner, ok := fileOwner(info)
	if !ok || owner != os.Getuid() {
		return fmt.Errorf("socket directory %s belongs to another user", dir)
	}
	if info.Mode().Perm() != 0o700 {
		if err := os.Chmod(dir, 0o700); err != nil {
			return fmt.Errorf("restrict socket directory: %w", err)
		}
	}
	return nil
}
