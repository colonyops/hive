//go:build server

package tmuxcc

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

// The headless server build stubs the subsystem out: the e2e lane asserts the
// unavailable UI this produces.
func TestServerBuildIsUnavailable(t *testing.T) {
	t.Parallel()

	require.False(t, buildSupportsTerminal)
	require.False(t, platformSupported())

	m := NewManager(t.Context(), ManagerOptions{
		versionProbe: func(context.Context, string) (string, error) { return "tmux 3.7b", nil },
	})
	t.Cleanup(func() { _ = m.Stop(context.WithoutCancel(t.Context())) })

	require.ErrorIs(t, m.Available(t.Context()), ErrUnavailable)

	_, err := m.Attach(t.Context(), "hive-demo", 80, 24)
	require.ErrorIs(t, err, ErrUnavailable)

	_, err = Attach(t.Context(), t.Context(), Options{Slug: "hive-demo", Cols: 80, Rows: 24})
	require.ErrorIs(t, err, ErrUnavailable)
}
