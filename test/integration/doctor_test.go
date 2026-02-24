//go:build integration

package integration

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDoctorJSON(t *testing.T) {
	h := NewHarness(t)

	out, err := h.RunStdout("doctor", "--format", "json")
	// Doctor may exit non-zero when checks fail, but must still produce valid JSON
	if err != nil {
		t.Logf("doctor exited with error (may be expected): %v", err)
	}

	var result map[string]any
	require.NoError(t, json.Unmarshal([]byte(out), &result), "parse doctor JSON: %s", out)
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
