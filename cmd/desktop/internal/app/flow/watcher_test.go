package flow

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsFlowFile(t *testing.T) {
	cases := []struct {
		name string
		want bool
	}{
		{"triage.yaml", true},
		{"triage.yml", true},
		{"triage.ui.yaml", true}, // layout edits still trigger a reload
		{"triage.ui.yml", true},
		{"triage.sidebar.yaml", false}, // sidebar layout is frontend-owned UI state
		{"triage.sidebar.yml", false},
		{"triage.sidebar.yaml.tmp", false}, // atomic-write temp file
		{"triage.yaml.tmp", false},
		{"notes.txt", false},
		{"triage", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, isFlowFile(filepath.Join("/flows", tc.name)))
		})
	}
}

func TestFlowsWatcherIgnoresSidebarLayout(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	changed := make(chan struct{}, 8)
	watcher, err := NewFlowsWatcher(dir, func() { changed <- struct{}{} }, zerolog.Nop())
	require.NoError(t, err)
	watcher.Start()
	t.Cleanup(watcher.Close)

	require.NoError(t, os.WriteFile(filepath.Join(dir, "work.sidebar.yaml"), []byte("x: 1\n"), 0o600))
	select {
	case <-changed:
		t.Fatal("watcher fired for a sidebar layout file")
	case <-time.After(600 * time.Millisecond):
	}

	require.NoError(t, os.WriteFile(filepath.Join(dir, "triage.yaml"), []byte("x: 1\n"), 0o600))
	select {
	case <-changed:
	case <-time.After(5 * time.Second):
		t.Fatal("watcher did not fire for a flow file")
	}
}
