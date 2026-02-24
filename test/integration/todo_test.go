//go:build integration

package integration

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTodoCRUD(t *testing.T) {
	h := NewHarness(t)

	// Add a todo
	addOut, err := h.RunStdout("todo", "add", "--title", "Review docs", "--source", "human")
	require.NoError(t, err, "todo add: %s", addOut)

	lines, err := parseJSONLines(strings.TrimSpace(addOut))
	require.NoError(t, err, "parse todo add output: %s", addOut)
	require.Len(t, lines, 1)

	todoID, ok := lines[0]["id"].(string)
	require.True(t, ok, "todo missing 'id' string field: %v", lines[0])
	assert.Equal(t, "Review docs", lines[0]["title"])
	assert.Equal(t, "pending", lines[0]["status"])

	// List all todos
	listOut, err := h.RunStdout("todo", "list")
	require.NoError(t, err, "todo list: %s", listOut)

	listLines, err := parseJSONLines(strings.TrimSpace(listOut))
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(listLines), 1)

	// Update status to completed
	updateOut, err := h.RunStdout("todo", "update", todoID, "--status", "completed")
	require.NoError(t, err, "todo update: %s", updateOut)

	updateLines, err := parseJSONLines(strings.TrimSpace(updateOut))
	require.NoError(t, err)
	require.Len(t, updateLines, 1)
	assert.Equal(t, "completed", updateLines[0]["status"])

	// List with status filter
	filterOut, err := h.RunStdout("todo", "list", "--status", "completed")
	require.NoError(t, err, "todo list filtered: %s", filterOut)

	filterLines, err := parseJSONLines(strings.TrimSpace(filterOut))
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
