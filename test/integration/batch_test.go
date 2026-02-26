//go:build integration

package integration

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)


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

func TestBatchWorktreeStrategy(t *testing.T) {
	h := NewHarness(t).WithConfig(worktreeConfig("worktree"))
	repo := createBareRepo(t, "batch-wt-repo")

	type batchSession struct {
		Name   string `json:"name"`
		Remote string `json:"remote"`
	}
	type batchInput struct {
		Sessions []batchSession `json:"sessions"`
	}

	inputData := batchInput{Sessions: []batchSession{
		{Name: "wt-batch-one", Remote: repo},
		{Name: "wt-batch-two", Remote: repo},
	}}
	inputBytes, err := json.Marshal(inputData)
	require.NoError(t, err)

	result, err := h.RunJSONWithStdin(string(inputBytes), "batch")
	require.NoError(t, err)

	resultsRaw, ok := result["results"].([]any)
	require.True(t, ok, "results should be an array, got: %T", result["results"])
	require.Len(t, resultsRaw, 2)

	for _, r := range resultsRaw {
		entry, ok := r.(map[string]any)
		require.True(t, ok, "batch result entry should be an object, got: %T", r)
		require.Equal(t, "created", entry["status"], "session %s: %s", entry["name"], entry["error"])

		path, ok := entry["path"].(string)
		require.True(t, ok && path != "", "path must be non-empty string, got: %v", entry["path"])
		assertWorktreeLayout(t, path)
	}
}
