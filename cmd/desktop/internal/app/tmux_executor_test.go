package app

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/desktop/internal/app/tmuxbin"
)

type recordingExecutor struct{ commands []string }

func (e *recordingExecutor) Run(_ context.Context, cmd string, _ ...string) ([]byte, error) {
	e.commands = append(e.commands, cmd)
	return nil, nil
}

func (e *recordingExecutor) RunDir(_ context.Context, _, cmd string, _ ...string) ([]byte, error) {
	e.commands = append(e.commands, cmd)
	return nil, nil
}

func (e *recordingExecutor) RunStream(_ context.Context, _, _ io.Writer, cmd string, _ ...string) error {
	e.commands = append(e.commands, cmd)
	return nil
}

func (e *recordingExecutor) RunDirStream(_ context.Context, _ string, _, _ io.Writer, cmd string, _ ...string) error {
	e.commands = append(e.commands, cmd)
	return nil
}

func TestTmuxExecutorSubstitutesTheLocatedBinary(t *testing.T) {
	dir := t.TempDir()
	located := filepath.Join(dir, "tmux")
	require.NoError(t, os.WriteFile(located, []byte("#!/bin/sh\n"), 0o755))

	inner := &recordingExecutor{}
	exec := newTmuxExecutor(inner, tmuxbin.NewResolver(located))

	_, err := exec.Run(t.Context(), "tmux", "kill-session")
	require.NoError(t, err)
	_, err = exec.RunDir(t.Context(), dir, "tmux", "has-session")
	require.NoError(t, err)
	require.NoError(t, exec.RunStream(t.Context(), io.Discard, io.Discard, "tmux", "-V"))
	require.NoError(t, exec.RunDirStream(t.Context(), dir, io.Discard, io.Discard, "tmux", "-V"))
	_, err = exec.Run(t.Context(), "git", "status")
	require.NoError(t, err)

	require.Equal(t, []string{located, located, located, located, "git"}, inner.commands)
}

// Discovery failing must not change which command runs: the vendored caller's
// own "tmux not found" is a better error than anything a rewrite could produce.
func TestTmuxExecutorFallsThroughWhenDiscoveryFails(t *testing.T) {
	inner := &recordingExecutor{}
	exec := newTmuxExecutor(inner, tmuxbin.NewResolver(filepath.Join(t.TempDir(), "tmux")))

	_, err := exec.Run(t.Context(), "tmux", "kill-session")
	require.NoError(t, err)
	require.Equal(t, []string{"tmux"}, inner.commands)
}
