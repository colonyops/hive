//go:build integration

package integration

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSmokeHelp(t *testing.T) {
	h := NewHarness(t)
	out, err := h.Run("help")
	require.NoError(t, err, "hive help should succeed: %s", out)
	assert.Contains(t, out, "hive", "help output should mention hive")
}
