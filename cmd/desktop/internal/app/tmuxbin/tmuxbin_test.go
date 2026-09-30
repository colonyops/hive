package tmuxbin

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// fakeTmux writes an executable named tmux into dir and returns its path.
func fakeTmux(t *testing.T, dir string) string {
	t.Helper()
	path := filepath.Join(dir, "tmux")
	require.NoError(t, os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0o755))
	return path
}

func TestLocatePrefersPath(t *testing.T) {
	onPath := t.TempDir()
	want := fakeTmux(t, onPath)
	t.Setenv("PATH", onPath)

	fallback := t.TempDir()
	fakeTmux(t, fallback)

	got, err := locate("", []string{fallback})
	require.NoError(t, err)
	require.Equal(t, want, got)
}

func TestLocateFallsBackToCommonPrefixes(t *testing.T) {
	t.Setenv("PATH", t.TempDir())

	empty := t.TempDir()
	installed := t.TempDir()
	want := fakeTmux(t, installed)

	got, err := locate("", []string{empty, installed})
	require.NoError(t, err)
	require.Equal(t, want, got)
}

func TestLocateSkipsUnusableCandidates(t *testing.T) {
	t.Setenv("PATH", t.TempDir())

	notExecutable := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(notExecutable, "tmux"), []byte("text"), 0o644))
	isDir := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(isDir, "tmux"), 0o755))
	installed := t.TempDir()
	want := fakeTmux(t, installed)

	got, err := locate("", []string{notExecutable, isDir, installed})
	require.NoError(t, err)
	require.Equal(t, want, got)
}

func TestLocateReportsWhereItLooked(t *testing.T) {
	t.Setenv("PATH", t.TempDir())

	searched := t.TempDir()
	_, err := locate("", []string{searched})
	require.ErrorIs(t, err, ErrNotFound)
	require.Contains(t, err.Error(), searched)
	require.Contains(t, err.Error(), "paths.tmux")
}

// An override that does not work must fail rather than fall through: a user who
// configured a path needs to hear that it is wrong, not silently get another
// tmux — or worse, the one whose absence made them set it.
func TestLocateOverrideWins(t *testing.T) {
	elsewhere := t.TempDir()
	fakeTmux(t, elsewhere)
	t.Setenv("PATH", elsewhere)

	configured := t.TempDir()
	want := fakeTmux(t, configured)
	got, err := locate(want, []string{elsewhere})
	require.NoError(t, err)
	require.Equal(t, want, got)

	_, err = locate(filepath.Join(t.TempDir(), "tmux"), []string{elsewhere})
	require.Error(t, err)
	require.NotErrorIs(t, err, ErrNotFound)
	require.Contains(t, err.Error(), "paths.tmux")
}

func TestResolverCachesSuccessOnly(t *testing.T) {
	dir := t.TempDir()
	want := filepath.Join(dir, "tmux")

	r := NewResolver(want)
	_, err := r.Path()
	require.Error(t, err, "nothing installed yet")

	fakeTmux(t, dir)
	got, err := r.Path()
	require.NoError(t, err, "installing tmux takes effect without a relaunch")
	require.Equal(t, want, got)

	require.NoError(t, os.Remove(want))
	got, err = r.Path()
	require.NoError(t, err, "a located tmux is remembered")
	require.Equal(t, want, got)
}
