package process

import (
	"sync"
	"time"
)

// SnapshotReader wraps a ProcessReader and answers Children from a single
// ppid → child PID map read on first use. All Children calls during a
// refresh cycle share that snapshot instead of re-querying the OS per pane,
// and a cycle that never asks for children never reads the process table.
//
// Create one SnapshotReader at the top of each refresh cycle and discard it
// afterwards. The snapshot may be slightly stale (a process could start or
// exit mid-cycle), but for agent detection this is an acceptable trade-off.
type SnapshotReader struct {
	base     ProcessReader
	load     func() map[int][]int // returns nil when the snapshot is unavailable
	once     sync.Once
	children map[int][]int
}

// NewSnapshotReader builds a SnapshotReader backed by base that reads the
// process table on its first Children call. If the read fails (e.g., a
// permission error), Children falls back to the base reader so detection
// still works at the cost of per-pane syscalls.
func NewSnapshotReader(base ProcessReader) *SnapshotReader {
	return &SnapshotReader{base: base, load: snapshotChildren}
}

// Children returns child PIDs from the snapshot when available, falling back
// to the base reader if the snapshot could not be built.
func (s *SnapshotReader) Children(pid int) ([]int, error) {
	s.once.Do(func() { s.children = s.load() })
	if s.children != nil {
		return s.children[pid], nil
	}
	return s.base.Children(pid)
}

// SetSnapshotChildren replaces the snapshot with m. Intended for tests only —
// allows injecting a known map without triggering a real OS snapshot.
func SetSnapshotChildren(s *SnapshotReader, m map[int][]int) {
	s.load = func() map[int][]int { return m }
}

func (s *SnapshotReader) TPGID(pid int) (int, error)        { return s.base.TPGID(pid) }
func (s *SnapshotReader) Comm(pid int) string               { return s.base.Comm(pid) }
func (s *SnapshotReader) Cmdline(pid int) ([]string, error) { return s.base.Cmdline(pid) }
func (s *SnapshotReader) Environ(pid int) map[string]string { return s.base.Environ(pid) }

// SnapshotCache shares one process-table snapshot across refresh cycles that
// fall within maxAge of each other. On darwin a snapshot copies the whole
// kernel process table, which is too costly to repeat on every status poll.
// It is safe for concurrent use.
type SnapshotCache struct {
	maxAge time.Duration
	now    func() time.Time
	take   func() map[int][]int

	mu       sync.Mutex
	children map[int][]int
	takenAt  time.Time
}

// NewSnapshotCache returns a cache whose snapshots are reused for maxAge.
func NewSnapshotCache(maxAge time.Duration) *SnapshotCache {
	return &SnapshotCache{maxAge: maxAge, now: time.Now, take: snapshotChildren}
}

// Reader returns a SnapshotReader over base that takes its children map from
// the cache on first use.
func (c *SnapshotCache) Reader(base ProcessReader) *SnapshotReader {
	return &SnapshotReader{base: base, load: c.load}
}

func (c *SnapshotCache) load() map[int][]int {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := c.now()
	if c.children != nil && now.Sub(c.takenAt) < c.maxAge {
		return c.children
	}
	c.children = c.take()
	c.takenAt = now
	return c.children
}
