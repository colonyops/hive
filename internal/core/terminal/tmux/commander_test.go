package tmux

import (
	"context"
	"fmt"
	"slices"
	"testing"

	"github.com/colonyops/hive/internal/core/terminal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeCommander struct {
	available bool
	calls     [][]string
}

func (f *fakeCommander) Available() bool { return f.available }

func (f *fakeCommander) Output(_ context.Context, args ...string) ([]byte, error) {
	f.calls = append(f.calls, slices.Clone(args))
	switch args[0] {
	case "list-panes":
		return []byte("session|||0|||shell|||/work|||100|||%1|||0|||shell|||session\n"), nil
	case "capture-pane":
		return []byte("## Task\n● Finished\n❯ "), nil
	default:
		return nil, fmt.Errorf("unexpected tmux command %q", args[0])
	}
}

func TestWithCommanderControlsAvailabilityListingAndCapture(t *testing.T) {
	commander := &fakeCommander{}
	integration := NewFromPreviewMatchers([]string{"claude"}, WithCommander(commander))

	assert.False(t, integration.Available())
	commander.available = true
	assert.True(t, integration.Available(), "availability failures must not be cached")

	integration.RefreshCache()
	info, err := integration.DiscoverSession(t.Context(), "session", nil)
	require.NoError(t, err)
	require.NotNil(t, info)

	status, err := integration.GetStatus(t.Context(), info)
	require.NoError(t, err)
	assert.Equal(t, terminal.StatusReady, status)
	assert.Equal(t, "## Task\n● Finished\n❯ ", info.PaneContent)
	require.Len(t, commander.calls, 3)
	assert.Equal(t, "list-panes", commander.calls[0][0])
	assert.Equal(t, []string{"capture-pane", "-t", "%1", "-p", "-J"}, commander.calls[1])
	assert.Equal(t, []string{"capture-pane", "-t", "%1", "-p", "-J"}, commander.calls[2])
}
