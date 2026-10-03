package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/colonyops/hive/internal/platform/tmux/bin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTmuxRunnerUsesResolvedBinaryForEveryCommand(t *testing.T) {
	t.Setenv("TMUX", "")
	resolved := filepath.Join(t.TempDir(), "tmux")
	require.NoError(t, os.WriteFile(resolved, []byte("#!/bin/sh\nprintf '%s' \"$*\"\n"), 0o755))
	runner := newTmuxRunner(func(context.Context) (string, error) { return resolved, nil }, nil)

	assert.True(t, runner.Available())
	out, _, err := runner.Capture(t.Context(), "capture-pane", "-t", "%1")
	require.NoError(t, err)
	assert.Equal(t, "capture-pane -t %1", string(out))
}

func TestTmuxRunnerRetriesResolutionAfterFailure(t *testing.T) {
	available := false
	runner := newTmuxRunner(func(context.Context) (string, error) {
		if !available {
			return "", errors.New("not installed")
		}
		return "/usr/local/bin/tmux", nil
	}, nil)

	assert.False(t, runner.Available())
	available = true
	assert.True(t, runner.Available())
}

func TestTmuxRunnerUsesDetachedEnvironmentAndInheritedSocket(t *testing.T) {
	t.Setenv("TMUX", "/tmp/custom.sock,123,0")
	resolved := filepath.Join(t.TempDir(), "tmux")
	require.NoError(t, os.WriteFile(resolved, []byte("#!/bin/sh\nprintf 'args=%s;tmux=%s;pane=%s' \"$*\" \"$TMUX\" \"$TMUX_PANE\"\n"), 0o755))
	runner := newTmuxRunner(
		func(context.Context) (string, error) { return resolved, nil },
		func(context.Context) []string {
			return []string{"PATH=" + os.Getenv("PATH"), "TMUX=client", "TMUX_PANE=%9"}
		},
	)

	out, _, err := runner.Capture(t.Context(), "list-panes")
	require.NoError(t, err)
	assert.Equal(t, "args=-S /tmp/custom.sock list-panes;tmux=;pane=", string(out))
}

func TestResolveTmuxBinaryFallsBackToTheLoginShellPath(t *testing.T) {
	fallbackCalls := 0
	path, err := resolveTmuxBinary(
		t.Context(),
		func() (string, error) { return "", tmuxbin.ErrNotFound },
		func(_ context.Context, name string) (string, error) {
			fallbackCalls++
			assert.Equal(t, "tmux", name)
			return "/custom/bin/tmux", nil
		},
	)

	require.NoError(t, err)
	assert.Equal(t, "/custom/bin/tmux", path)
	assert.Equal(t, 1, fallbackCalls)
}

func TestResolveTmuxBinaryDoesNotBypassAConfiguredPathError(t *testing.T) {
	configuredErr := errors.New("paths.tmux: permission denied")
	fallbackCalls := 0
	_, err := resolveTmuxBinary(
		t.Context(),
		func() (string, error) { return "", configuredErr },
		func(context.Context, string) (string, error) {
			fallbackCalls++
			return "/custom/bin/tmux", nil
		},
	)

	require.ErrorIs(t, err, configuredErr)
	assert.Zero(t, fallbackCalls)
}
