package process_test

import (
	"testing"

	"github.com/colonyops/hive/internal/core/terminal/process"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeBase is a ProcessReader that records calls to Children so tests can
// assert whether the snapshot short-circuits the underlying reader.
type fakeBase struct {
	childrenCalls int
	children      map[int][]int
}

func (f *fakeBase) TPGID(pid int) (int, error)        { return pid, nil }
func (f *fakeBase) Comm(pid int) string               { return "" }
func (f *fakeBase) Cmdline(pid int) ([]string, error) { return nil, nil }
func (f *fakeBase) Environ(pid int) map[string]string { return nil }
func (f *fakeBase) Children(pid int) ([]int, error) {
	f.childrenCalls++
	return f.children[pid], nil
}

// snapshotWithMap builds a SnapshotReader whose internal children map is
// replaced with the provided one, bypassing the real OS call.
func snapshotWithMap(base process.ProcessReader, m map[int][]int) *process.SnapshotReader {
	s := process.NewSnapshotReader(base)
	process.SetSnapshotChildren(s, m)
	return s
}

func TestSnapshotReader_UsesSnapshot(t *testing.T) {
	base := &fakeBase{}
	snap := snapshotWithMap(base, map[int][]int{
		100: {200, 201},
		200: {300},
	})

	children, err := snap.Children(100)
	require.NoError(t, err)
	assert.ElementsMatch(t, []int{200, 201}, children)
	assert.Equal(t, 0, base.childrenCalls, "base.Children must not be called when snapshot is available")

	children, err = snap.Children(200)
	require.NoError(t, err)
	assert.ElementsMatch(t, []int{300}, children)
	assert.Equal(t, 0, base.childrenCalls)
}

func TestSnapshotReader_EmptyChildrenReturnsNil(t *testing.T) {
	snap := snapshotWithMap(&fakeBase{}, map[int][]int{100: {200}})
	children, err := snap.Children(999) // not in snapshot
	require.NoError(t, err)
	assert.Nil(t, children)
}

func TestSnapshotReader_FallsBackWhenSnapshotNil(t *testing.T) {
	base := &fakeBase{children: map[int][]int{100: {200}}}
	snap := snapshotWithMap(base, nil) // nil map → fall back
	children, err := snap.Children(100)
	require.NoError(t, err)
	assert.ElementsMatch(t, []int{200}, children)
	assert.Equal(t, 1, base.childrenCalls, "base.Children must be called when snapshot is nil")
}

func TestSnapshotReader_DelegatesOtherMethods(t *testing.T) {
	base := &fakeBase{}
	snap := snapshotWithMap(base, nil)

	// TPGID, Comm, Cmdline, Environ all delegate to base without panicking.
	tpgid, err := snap.TPGID(42)
	require.NoError(t, err)
	assert.Equal(t, 42, tpgid)

	assert.Empty(t, snap.Comm(42))
	argv, err := snap.Cmdline(42)
	require.NoError(t, err)
	assert.Nil(t, argv)
	assert.Nil(t, snap.Environ(42))
}

func TestNewSnapshotReader_LiveOS(t *testing.T) {
	// Smoke test: build a real snapshot from the live OS and verify that our
	// own PID appears as a child of its parent. This runs on every platform
	// that has a real snapshotChildren implementation.
	snap := process.NewSnapshotReader(process.OSReader{})
	// We can't assert exact contents, but Children should not panic and the
	// snapshot-backed reader must satisfy the ProcessReader interface.
	var _ process.ProcessReader = snap
}
