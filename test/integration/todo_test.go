//go:build integration

package integration

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTodoCRUD(t *testing.T) {
	h := NewHarness(t)

	// Add a todo
	lines, err := h.RunJSONLines("todo", "add", "--title", "Review docs", "--source", "human")
	require.NoError(t, err)
	require.Len(t, lines, 1)

	todoID, ok := lines[0]["id"].(string)
	require.True(t, ok, "todo missing 'id' string field: %v", lines[0])
	assert.Equal(t, "Review docs", lines[0]["title"])
	assert.Equal(t, "pending", lines[0]["status"])

	// List all todos
	listLines, err := h.RunJSONLines("todo", "list")
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(listLines), 1)

	// Update status to completed
	updateLines, err := h.RunJSONLines("todo", "update", todoID, "--status", "completed")
	require.NoError(t, err)
	require.Len(t, updateLines, 1)
	assert.Equal(t, "completed", updateLines[0]["status"])

	// List with status filter
	filterLines, err := h.RunJSONLines("todo", "list", "--status", "completed")
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(filterLines), 1)

	found := false
	for _, l := range filterLines {
		if l["id"] == todoID {
			found = true
			assert.Equal(t, "completed", l["status"])
		}
	}
	assert.True(t, found, "updated todo should appear in filtered list")
}
