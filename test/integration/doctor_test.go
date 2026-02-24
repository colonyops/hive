//go:build integration

package integration

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDoctorJSON(t *testing.T) {
	h := NewHarness(t)

	// Doctor may exit non-zero when checks fail, so use RunStdout + manual parse
	// rather than RunJSON which would treat a non-zero exit as an error.
	out, err := h.RunStdout("doctor", "--format", "json")
	if err != nil {
		t.Logf("doctor exited with error (may be expected): %v", err)
	}

	result, err := parseJSON(out)
	require.NoError(t, err, "parse doctor JSON: %s", out)
	assert.Contains(t, result, "healthy")
	assert.Contains(t, result, "summary")
	assert.Contains(t, result, "checks")
}

func TestDoctorText(t *testing.T) {
	h := NewHarness(t)

	out, err := h.Run("doctor")
	// Doctor may exit non-zero when checks fail
	if err != nil {
		t.Logf("doctor exited with error (may be expected): %v", err)
	}

	assert.Contains(t, out, "Doctor")
}
