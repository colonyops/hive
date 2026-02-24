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
	// Doctor may exit non-zero if checks fail; we still want to parse output
	_ = err

	var result map[string]any
	require.NoError(t, json.Unmarshal([]byte(out), &result), "parse doctor JSON: %s", out)
	assert.Contains(t, result, "healthy")
	assert.Contains(t, result, "summary")
	assert.Contains(t, result, "checks")
}

func TestDoctorText(t *testing.T) {
	h := NewHarness(t)

	out, err := h.Run("doctor")
	// Doctor may exit non-zero; just verify it produces output
	_ = err

	assert.Contains(t, out, "Doctor")
}
