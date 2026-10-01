package releasenotes

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStateRoundTrip(t *testing.T) {
	state := NewState(t.TempDir())

	assert.Empty(t, state.Acknowledged(), "nothing is acknowledged before the first write")
	require.NoError(t, state.Acknowledge("1.2.0-dev.3"))
	assert.Equal(t, "1.2.0-dev.3", state.Acknowledged())

	require.NoError(t, state.Acknowledge("1.2.0"))
	assert.Equal(t, "1.2.0", state.Acknowledged())
}

func TestStateCreatesItsDirectory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested", "state")
	state := NewState(dir)

	require.NoError(t, state.Acknowledge("1.2.0"))
	assert.FileExists(t, filepath.Join(dir, StateFileName))
}

// A marker that cannot be read must not be able to block a launch; the worst
// it can cost is one extra What's New surface.
func TestStateTreatsACorruptMarkerAsUnacknowledged(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, StateFileName), []byte("{not json"), 0o600))

	assert.Empty(t, NewState(dir).Acknowledged())
}
