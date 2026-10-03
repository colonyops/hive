package process

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCandidates_ForegroundProcess(t *testing.T) {
	reader := fakeReader{procs: map[int]fakeProc{
		100: {tpgid: 200},
		200: {comm: "claude", argv: []string{"/usr/local/bin/claude"}},
	}}
	got, err := Candidates(100, reader)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, 200, got.Foreground.PID)
	assert.Equal(t, "claude", got.Foreground.Comm)
	assert.Equal(t, []string{"/usr/local/bin/claude"}, got.Foreground.Argv)
}

func TestCandidates_IncludesChildrenToDepth2(t *testing.T) {
	reader := fakeReader{
		procs: map[int]fakeProc{
			100: {tpgid: 200},
			200: {comm: "node", argv: []string{"node", "wrapper.js"}},
			201: {comm: "bash"},
			202: {comm: "claude", argv: []string{"claude"}},
		},
		children: map[int][]int{
			200: {201},
			201: {202},
		},
	}
	got, err := Candidates(100, reader)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "node", got.Foreground.Comm)
	require.Len(t, got.Children, 2)
	assert.Equal(t, "bash", got.Children[0].Comm)
	assert.Equal(t, "claude", got.Children[1].Comm)
}

func TestCandidates_DoesNotExceedDepth2(t *testing.T) {
	reader := fakeReader{
		procs: map[int]fakeProc{
			100: {tpgid: 200},
			200: {comm: "sh"},
			201: {comm: "bash"},
			202: {comm: "zsh"},
			203: {comm: "claude"}, // depth 3 — must not appear
		},
		children: map[int][]int{
			200: {201},
			201: {202},
			202: {203},
		},
	}
	got, err := Candidates(100, reader)
	require.NoError(t, err)
	require.Len(t, got.Children, 2, "depth-3 processes must not be included")
	comms := []string{got.Children[0].Comm, got.Children[1].Comm}
	assert.NotContains(t, comms, "claude")
}

func TestCandidates_TpgidErrorFallsBackToPanePID(t *testing.T) {
	reader := fakeReader{
		tpgidErr: errors.New("permission denied"),
		procs:    map[int]fakeProc{100: {comm: "claude", argv: []string{"claude"}}},
	}
	got, err := Candidates(100, reader)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, 100, got.Foreground.PID)
	assert.Equal(t, "claude", got.Foreground.Comm)
}

func TestCandidates_InvalidPIDReturnsNil(t *testing.T) {
	got, err := Candidates(0, fakeReader{})
	require.NoError(t, err)
	assert.Nil(t, got)
}

func TestCandidates_SkipsDuplicatePIDs(t *testing.T) {
	reader := fakeReader{
		procs: map[int]fakeProc{
			100: {tpgid: 200},
			200: {comm: "sh"},
			201: {comm: "bash"},
		},
		// cycle: 200 → 201 → 200 (should not loop)
		children: map[int][]int{200: {201}, 201: {200}},
	}
	got, err := Candidates(100, reader)
	require.NoError(t, err)
	require.Len(t, got.Children, 1)
	assert.Equal(t, "bash", got.Children[0].Comm)
}

type fakeReader struct {
	procs    map[int]fakeProc
	children map[int][]int
	tpgidErr error
}

type fakeProc struct {
	tpgid   int
	comm    string
	argv    []string
	env     map[string]string
	argvErr error
}

func (f fakeReader) TPGID(pid int) (int, error) {
	if f.tpgidErr != nil {
		return 0, f.tpgidErr
	}
	return f.procs[pid].tpgid, nil
}
func (f fakeReader) Comm(pid int) string { return f.procs[pid].comm }
func (f fakeReader) Cmdline(pid int) ([]string, error) {
	p := f.procs[pid]
	if p.argvErr != nil {
		return nil, p.argvErr
	}
	return p.argv, nil
}
func (f fakeReader) Environ(pid int) map[string]string { return f.procs[pid].env }
func (f fakeReader) Children(pid int) ([]int, error)   { return f.children[pid], nil }
