package tmuxcc

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCommanderUsesResolvedBinaryForEveryCommand(t *testing.T) {
	resolved := "/opt/homebrew/bin/tmux"
	commander := NewCommander(func() (string, error) { return resolved, nil }, nil)
	var gotBinary string
	var gotArgs []string
	commander.output = func(_ context.Context, binary string, args ...string) ([]byte, error) {
		gotBinary = binary
		gotArgs = args
		return []byte("pane output"), nil
	}

	assert.True(t, commander.Available())
	out, err := commander.Output(t.Context(), "capture-pane", "-t", "%1")
	require.NoError(t, err)
	assert.Equal(t, "pane output", string(out))
	assert.Equal(t, resolved, gotBinary)
	assert.Equal(t, []string{"capture-pane", "-t", "%1"}, gotArgs)
}

func TestCommanderRetriesResolutionAfterFailure(t *testing.T) {
	available := false
	commander := NewCommander(func() (string, error) {
		if !available {
			return "", errors.New("not installed")
		}
		return "/usr/local/bin/tmux", nil
	}, nil)

	assert.False(t, commander.Available())
	available = true
	assert.True(t, commander.Available())
}
