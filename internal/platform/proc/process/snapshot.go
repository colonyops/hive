package process

// SnapshotReader wraps a ProcessReader and pre-loads child process mappings
// from a single OS call at construction time. All Children calls during a
// refresh cycle share that snapshot instead of re-querying the OS per pane.
//
// Create one SnapshotReader at the top of each refresh cycle and discard it
// afterwards. The snapshot is intentionally short-lived: it may be slightly
// stale (a process could start or exit mid-cycle), but for agent detection
// this is an acceptable trade-off.
type SnapshotReader struct {
	base     ProcessReader
	children map[int][]int // ppid → child PIDs, nil if snapshot unavailable
}

// NewSnapshotReader builds a SnapshotReader backed by base. It reads the full
// process table once via snapshotChildren and stores a ppid→childPID map.
// If the snapshot fails (e.g., permission error), Children falls back to the
// base reader so detection still works at the cost of per-pane syscalls.
func NewSnapshotReader(base ProcessReader) *SnapshotReader {
	return &SnapshotReader{
		base:     base,
		children: snapshotChildren(), // platform-specific; returns nil on failure
	}
}

// Children returns child PIDs from the in-memory snapshot when available,
// falling back to the base reader if the snapshot was not built.
func (s *SnapshotReader) Children(pid int) ([]int, error) {
	if s.children != nil {
		return s.children[pid], nil
	}
	return s.base.Children(pid)
}

// SetSnapshotChildren replaces the internal children map. Intended for tests
// only — allows injecting a known map without triggering a real OS snapshot.
func SetSnapshotChildren(s *SnapshotReader, m map[int][]int) { s.children = m }

func (s *SnapshotReader) TPGID(pid int) (int, error)        { return s.base.TPGID(pid) }
func (s *SnapshotReader) Comm(pid int) string               { return s.base.Comm(pid) }
func (s *SnapshotReader) Cmdline(pid int) ([]string, error) { return s.base.Cmdline(pid) }
func (s *SnapshotReader) Environ(pid int) map[string]string { return s.base.Environ(pid) }
