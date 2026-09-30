package main

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"github.com/rs/zerolog"
)

type instanceLock struct {
	path      string
	worktree  string
	pid       int
	alive     func(int) bool
	owned     bool
	stalePath string
	logger    zerolog.Logger
}

func (d *devtools) withLock(operation func() error) error {
	lock := &instanceLock{path: d.lockPath, worktree: d.worktree, pid: d.pid, alive: d.alive, logger: d.logger}
	if err := lock.acquire(); err != nil {
		return err
	}
	defer lock.release()
	return operation()
}

func (l *instanceLock) acquire() error {
	path := filepath.Clean(l.path)
	root := filepath.Clean(l.worktree)
	if path != filepath.Join(root, ".hive-desktop.lock") && path != filepath.Join(root, ".hive-desktop-onboarding.lock") {
		return errors.New("development lock escaped the worktree")
	}
	if info, err := os.Lstat(l.path); err == nil && info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("refusing symlink lock at %s", l.path)
	} else if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	if err := os.Mkdir(l.path, 0o700); err == nil {
		return l.claim()
	} else if !errors.Is(err, fs.ErrExist) {
		return err
	}

	info, err := os.Lstat(l.path)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("development lock %s is not a directory", l.path)
	}
	ownerBytes, ownerErr := os.ReadFile(filepath.Join(l.path, "owner"))
	pidBytes, pidErr := os.ReadFile(filepath.Join(l.path, "pid"))
	if ownerErr != nil || pidErr != nil {
		return fmt.Errorf("development lock is incomplete; inspect %s", l.path)
	}
	owner := strings.TrimSpace(string(ownerBytes))
	pid, err := strconv.Atoi(strings.TrimSpace(string(pidBytes)))
	if err != nil || pid <= 0 {
		return fmt.Errorf("development lock has an invalid pid; inspect %s", l.path)
	}
	if owner != l.worktree {
		return fmt.Errorf("development lock belongs to %q", owner)
	}
	if l.alive(pid) {
		return fmt.Errorf("desktop development instance is active (pid %d)", pid)
	}

	l.logger.Warn().Int("stale_pid", pid).Str("lock", l.path).Msg("recovering stale desktop development lock")
	l.stalePath = fmt.Sprintf("%s.stale.%d", l.path, l.pid)
	if err := os.Rename(l.path, l.stalePath); err != nil {
		return fmt.Errorf("recover stale development lock: %w", err)
	}
	defer func() { _ = os.RemoveAll(l.stalePath) }()
	if err := os.Mkdir(l.path, 0o700); err != nil {
		return fmt.Errorf("another process acquired the development lock: %w", err)
	}
	return l.claim()
}

func (l *instanceLock) claim() error {
	if err := os.WriteFile(filepath.Join(l.path, "pid"), []byte(strconv.Itoa(l.pid)+"\n"), 0o600); err != nil {
		_ = os.RemoveAll(l.path)
		return err
	}
	if err := os.WriteFile(filepath.Join(l.path, "owner"), []byte(l.worktree+"\n"), 0o600); err != nil {
		_ = os.RemoveAll(l.path)
		return err
	}
	l.owned = true
	return nil
}

func (l *instanceLock) release() {
	if !l.owned {
		return
	}
	owner, _ := os.ReadFile(filepath.Join(l.path, "owner"))
	pid, _ := os.ReadFile(filepath.Join(l.path, "pid"))
	if strings.TrimSpace(string(owner)) == l.worktree && strings.TrimSpace(string(pid)) == strconv.Itoa(l.pid) {
		_ = os.RemoveAll(l.path)
	}
	l.owned = false
}

func processAlive(pid int) bool {
	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}
