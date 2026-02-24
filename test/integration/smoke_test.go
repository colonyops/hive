//go:build integration

package integration

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSmokeVersion(t *testing.T) {
	h := NewHarness(t)
	out, err := h.Run("version")
	require.NoError(t, err, "hive version should succeed")
	assert.NotEmpty(t, out, "version output should not be empty")
}
