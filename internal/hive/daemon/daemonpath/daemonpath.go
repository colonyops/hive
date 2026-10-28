// Package daemonpath names the files of the hive daemon for a data directory,
// so a client finds them without importing the daemon.
package daemonpath

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
// socket. Symlinks are resolved first, so every spelling of one data dir
// gets the same socket.
func SocketPath(dataDir string) string {
	dataDir = resolved(absolute(dataDir))
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

// OutLogPath is where a detached daemon's stdout and stderr go.
func OutLogPath(dataDir string) string {
	return filepath.Join(dataDir, "logs", "hived.out")
}

func fallbackSocketDir() string {
	return filepath.Join(fallbackSocketRoot, fmt.Sprintf("hive-%d", os.Getuid()))
}

// resolved evaluates the symlinks in the longest existing prefix of path, so
// a client that dials before the daemon creates the data dir computes the
// same socket path as the daemon does after.
func resolved(path string) string {
	var rest []string
	for dir := path; ; dir = filepath.Dir(dir) {
		if real, err := filepath.EvalSymlinks(dir); err == nil {
			return filepath.Join(append([]string{real}, rest...)...)
		}
		if filepath.Dir(dir) == dir {
			return path
		}
		rest = append([]string{filepath.Base(dir)}, rest...)
	}
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

// PrepareSocketDir makes the directory a socket lives in. The shared
// fallback directory must be ours and private: another user who created
// /tmp/hive-<uid> first could otherwise read the socket's traffic.
func PrepareSocketDir(socketPath string) error {
	dir := filepath.Dir(socketPath)
	if dir != fallbackSocketDir() {
		return os.MkdirAll(dir, 0o755)
	}
	if err := os.Mkdir(dir, 0o700); err != nil && !errors.Is(err, os.ErrExist) {
		return fmt.Errorf("create socket directory: %w", err)
	}
	info, err := ownFallbackDir(dir)
	if err != nil {
		return err
	}
	if info.Mode().Perm() != 0o700 {
		if err := os.Chmod(dir, 0o700); err != nil {
			return fmt.Errorf("restrict socket directory: %w", err)
		}
	}
	return nil
}

// CheckSocketDir is the client half of PrepareSocketDir: a client must not
// send its requests to a socket another user planted in the shared fallback
// directory. A missing directory passes, since the dial then finds no daemon.
func CheckSocketDir(socketPath string) error {
	dir := filepath.Dir(socketPath)
	if dir != fallbackSocketDir() {
		return nil
	}
	info, err := ownFallbackDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if info.Mode().Perm()&0o022 != 0 {
		return fmt.Errorf("socket directory %s is writable by other users", dir)
	}
	return nil
}

func ownFallbackDir(dir string) (os.FileInfo, error) {
	info, err := os.Lstat(dir)
	if err != nil {
		return nil, fmt.Errorf("inspect socket directory: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("socket directory %s is not a directory", dir)
	}
	owner, ok := fileOwner(info)
	if !ok || owner != os.Getuid() {
		return nil, fmt.Errorf("socket directory %s belongs to another user", dir)
	}
	return info, nil
}
