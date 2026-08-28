package commands

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func notIsolated() bool { return false }
func isolated() bool    { return true }

func TestSupportedIsolationMarkersRequiresDockerAndMiseMarker(t *testing.T) {
	assert.True(t, supportedIsolationMarkers(true, "1"))
	assert.False(t, supportedIsolationMarkers(false, "1"))
	assert.False(t, supportedIsolationMarkers(true, ""))
	assert.False(t, supportedIsolationMarkers(true, "0"))
}

func TestEnsureContainerSafe_AllowHostSkipsChecks(t *testing.T) {
	resolveCalled := false
	isolationCalled := false
	resolve := func(_ context.Context, _ string) (string, error) {
		resolveCalled = true
		return "/tmp/tmux-501/default", nil
	}

	err := ensureContainerSafe(context.Background(), "mypane:0.0", true, resolve, func() bool {
		isolationCalled = true
		return false
	})
	require.NoError(t, err)
	assert.False(t, resolveCalled)
	assert.False(t, isolationCalled)
}

func TestEnsureContainerSafe_AllowsKnownContainerWithoutSocketHeuristic(t *testing.T) {
	resolve := func(_ context.Context, _ string) (string, error) {
		t.Fatal("isolated execution must not need a socket-name heuristic")
		return "", nil
	}

	require.NoError(t, ensureContainerSafe(context.Background(), "mypane:0.0", false, resolve, isolated))
}

func TestEnsureContainerSafe_RefusesAllHostSockets(t *testing.T) {
	for _, path := range []string{
		"/tmp/tmux-501/default",
		"/tmp/tmux-501/named-host-socket",
		"/tmp/hive/tmux-sockets/dev.sock",
	} {
		t.Run(path, func(t *testing.T) {
			resolve := func(_ context.Context, _ string) (string, error) { return path, nil }

			err := ensureContainerSafe(context.Background(), "mypane:0.0", false, resolve, notIsolated)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "socket names do not prove isolation")
			assert.Contains(t, err.Error(), "mise container")
			assert.Contains(t, err.Error(), "--allow-host")
		})
	}
}

func TestEnsureContainerSafe_FailsClosedOnResolveError(t *testing.T) {
	sentinel := errors.New("no tmux server running")
	resolve := func(_ context.Context, _ string) (string, error) {
		return "", sentinel
	}

	err := ensureContainerSafe(context.Background(), "mypane:0.0", false, resolve, notIsolated)
	require.Error(t, err)
	require.ErrorIs(t, err, sentinel)
	assert.Contains(t, err.Error(), "cannot prove")
}
