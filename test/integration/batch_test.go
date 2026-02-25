//go:build integration

package integration

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestBatchWorktreeStrategy verifies that batch session creation respects
// the config-level clone_strategy: worktree setting, producing worktree
// layouts and persisting clone_strategy in the database for each session.
func TestBatchWorktreeStrategy(t *testing.T) {
	h := NewHarness(t).WithConfig(`
version: "0.2.4"
git_path: git
clone_strategy: worktree
agents:
  default: testbash
  testbash:
    command: bash
rules:
  - spawn:
      - "tmux new-session -d -s {{ .Name | shq }} -c {{ .Path | shq }}"
    batch_spawn:
      - "tmux new-session -d -s {{ .Name | shq }} -c {{ .Path | shq }}"
`)
	repo := createBareRepo(t, "batch-wt")

	type batchSession struct {
		Name   string `json:"name"`
		Remote string `json:"remote"`
	}
	inputBytes, err := json.Marshal(map[string]any{
		"sessions": []batchSession{
			{Name: "bwt-alpha", Remote: repo},
			{Name: "bwt-beta", Remote: repo},
		},
	})
	require.NoError(t, err)

	result, err := h.RunJSONWithStdin(string(inputBytes), "batch")
	require.NoError(t, err, "batch with worktree strategy should succeed")

	results, ok := result["results"].([]any)
	require.True(t, ok, "results should be an array")
	require.Len(t, results, 2)

	for _, r := range results {
		entry, ok := r.(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "created", entry["status"], "session %s should be created", entry["name"])

		path, ok := entry["path"].(string)
		require.True(t, ok, "batch result must include path")
		assert.NotEmpty(t, path)

		assertWorktreeLayout(t, path)

		name, _ := entry["name"].(string)
		row := readSessionRowByName(t, h, name)
		assert.Equal(t, "worktree", row.CloneStrategy)
		assert.Equal(t, "active", row.State)
	}
}

func TestBatchCreate(t *testing.T) {
	h := NewHarness(t)
	repo := createBareRepo(t, "batch-repo")

	type batchSession struct {
		Name   string `json:"name"`
		Remote string `json:"remote"`
	}
	type batchInput struct {
		Sessions []batchSession `json:"sessions"`
	}

	inputData := batchInput{Sessions: []batchSession{
		{Name: "batch-one", Remote: repo},
		{Name: "batch-two", Remote: repo},
	}}
	inputBytes, err := json.Marshal(inputData)
	require.NoError(t, err)

	result, err := h.RunJSONWithStdin(string(inputBytes), "batch")
	require.NoError(t, err)

	assert.Contains(t, result, "batch_id")
	assert.Contains(t, result, "results")

	resultsRaw, ok := result["results"].([]any)
	require.True(t, ok, "results should be an array, got: %T", result["results"])
	require.Len(t, resultsRaw, 2)

	for _, r := range resultsRaw {
		entry, ok := r.(map[string]any)
		require.True(t, ok, "batch result entry should be an object, got: %T", r)
		assert.Equal(t, "created", entry["status"], "session %s should be created", entry["name"])
		assert.NotEmpty(t, entry["session_id"])
	}

	// Verify sessions appear in ls
	lsOut, err := h.Run("ls")
	require.NoError(t, err)
	assert.Contains(t, lsOut, "batch-one")
	assert.Contains(t, lsOut, "batch-two")
}
