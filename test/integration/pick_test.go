//go:build integration

package integration

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPickHelp(t *testing.T) {
	h := NewHarness(t)
	out, err := h.Run("x", "pick", "--help")
	require.NoError(t, err, "hive x pick --help should exit 0: %s", out)

	// Verify all flags are documented
	for _, flag := range []string{"--status", "--repo", "--print", "--format", "--hide-current", "--no-recents"} {
		assert.Contains(t, out, flag, "help should document %s flag", flag)
	}
}

func TestPickInvalidStatusFlag(t *testing.T) {
	h := NewHarness(t)
	out, err := h.Run("x", "pick", "--status", "bogus", "--print")
	require.Error(t, err, "invalid --status should fail")
	assert.Contains(t, out, "invalid --status")
	assert.Contains(t, out, "bogus")
}

func TestPickInvalidFormatFlag(t *testing.T) {
	h := NewHarness(t)
	out, err := h.Run("x", "pick", "--format", "bogus", "--print")
	require.Error(t, err, "invalid --format should fail")
	assert.Contains(t, out, "invalid --format")
	assert.Contains(t, out, "bogus")
}
