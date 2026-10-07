//go:build !server

package app

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTerminalAnalyticsOnlySuccessfulNewStarts(t *testing.T) {
	tmux := privateTmux(t)
	starter := &spawningStarter{tmux: tmux}
	terminals := newTestTerminalsIn(t, starter, func() (string, error) { return t.TempDir(), nil })
	capture := &usageCapture{}
	terminals.analytics = capture
	for _, slug := range []string{"analytics-hive", ScratchSlug} {
		started, err := terminals.Start(t.Context(), slug)
		require.NoError(t, err)
		require.True(t, started)
		started, err = terminals.Start(t.Context(), slug)
		require.NoError(t, err)
		require.False(t, started)
		_, err = terminals.Attach(t.Context(), slug, 120, 40)
		require.NoError(t, err)
	}
	require.Len(t, capture.events, 2)
	require.Contains(t, capture.events[0].Properties(), `"kind":"hive"`)
	require.Contains(t, capture.events[1].Properties(), `"kind":"scratch"`)
	starter.err = Errorf(KindUnavailable, "spawn failed")
	started, err := terminals.Start(t.Context(), "analytics-failure")
	require.Error(t, err)
	require.False(t, started)
	require.Len(t, capture.events, 2)
	require.NoError(t, tmux("kill-session", "-t", ScratchSlug))
	terminals.home = func() (string, error) { return "", Errorf(KindUnavailable, "home unavailable") }
	started, err = terminals.Start(t.Context(), ScratchSlug)
	require.Error(t, err)
	require.False(t, started)
	require.Len(t, capture.events, 2)
}
