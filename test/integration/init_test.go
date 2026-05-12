//go:build integration

package integration

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestInitHelp verifies that init is registered and its help text is reachable.
// Mirrors TestDoctorText in doctor_test.go.
func TestInitHelp(t *testing.T) {
	h := NewHarness(t)
	out, err := h.RunStdout("init", "--help")
	// --help exits with code 0; RunStdout should succeed
	require.NoError(t, err)
	assert.Contains(t, out, "interactive setup wizard")
}
