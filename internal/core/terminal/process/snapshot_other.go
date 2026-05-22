//go:build !linux && !darwin

package process

// snapshotChildren is not implemented on this platform; SnapshotReader falls
// back to per-call Children lookups via the base ProcessReader.
func snapshotChildren() map[int][]int { return nil }
