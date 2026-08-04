package commands

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsHostDefaultSocket(t *testing.T) {
	tests := []struct {
		name string
		path string
		want bool
	}{
		{"host default socket", "/tmp/tmux-501/default", true},
		{"host default socket, different uid", "/tmp/tmux-0/default", true},
		{"named socket under the default dir", "/tmp/tmux-501/hive-container", false},
		{"container socket elsewhere entirely", "/tmp/hive/tmux-sockets/dev.sock", false},
		{"empty path", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, isHostDefaultSocket(tt.path))
		})
	}
}

func TestEnsureContainerSafe_AllowHostSkipsResolution(t *testing.T) {
	resolveCalled := false
	resolve := func(_ context.Context, _ string) (string, error) {
		resolveCalled = true
		return "/tmp/tmux-501/default", nil
	}

	err := ensureContainerSafe(context.Background(), "mypane:0.0", true, resolve)
	require.NoError(t, err)
	assert.False(t, resolveCalled, "--allow-host must skip the socket resolution entirely")
}

func TestEnsureContainerSafe_RefusesHostDefaultSocket(t *testing.T) {
	resolve := func(_ context.Context, _ string) (string, error) {
		return "/tmp/tmux-501/default", nil
	}

	err := ensureContainerSafe(context.Background(), "mypane:0.0", false, resolve)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "mise container")
	assert.Contains(t, err.Error(), "--allow-host")
}

func TestEnsureContainerSafe_AllowsNonDefaultSocket(t *testing.T) {
	resolve := func(_ context.Context, _ string) (string, error) {
		return "/tmp/hive/tmux-sockets/dev.sock", nil
	}

	err := ensureContainerSafe(context.Background(), "mypane:0.0", false, resolve)
	require.NoError(t, err)
}

func TestEnsureContainerSafe_PropagatesResolveError(t *testing.T) {
	sentinel := errors.New("no tmux server running")
	resolve := func(_ context.Context, _ string) (string, error) {
		return "", sentinel
	}

	err := ensureContainerSafe(context.Background(), "mypane:0.0", false, resolve)
	require.Error(t, err)
	assert.ErrorIs(t, err, sentinel)
}
