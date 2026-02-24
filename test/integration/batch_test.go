//go:build integration

package integration

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBatchCreate(t *testing.T) {
	h := NewHarness(t)
	repo := createBareRepo(t, "batch-repo")

	input := `{"sessions":[{"name":"batch-one","remote":"` + repo + `"},{"name":"batch-two","remote":"` + repo + `"}]}`

	// Run batch via stdin
	cmd := h.command("batch")
	cmd.Stdin = strings.NewReader(input)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "hive batch: %s", out)

	// Parse JSON output
	var result map[string]any
	require.NoError(t, json.Unmarshal(out, &result), "parse batch output: %s", out)

	assert.Contains(t, result, "batch_id")
	assert.Contains(t, result, "results")

	results := result["results"].([]any)
	require.Len(t, results, 2)

	for _, r := range results {
		entry := r.(map[string]any)
		assert.Equal(t, "created", entry["status"], "session %s should be created", entry["name"])
		assert.NotEmpty(t, entry["session_id"])
	}

	// Verify sessions appear in ls
	lsOut, err := h.Run("ls")
	require.NoError(t, err)
	assert.Contains(t, lsOut, "batch-one")
	assert.Contains(t, lsOut, "batch-two")
}
