//go:build integration

package integration

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHCCreateSingle(t *testing.T) {
	h := NewHarness(t)

	lines, err := h.RunJSONLines("hc", "create", "My Epic", "--type", "epic")
	require.NoError(t, err)
	require.Len(t, lines, 1)

	id, ok := lines[0]["id"].(string)
	require.True(t, ok, "item missing 'id' string field: %v", lines[0])
	assert.True(t, strings.HasPrefix(id, "hc-"), "id should have 'hc-' prefix, got: %s", id)
	assert.Equal(t, "epic", lines[0]["type"])
}

func TestHCCreateBulk(t *testing.T) {
	h := NewHarness(t)

	input := `{"title":"Root Epic","type":"epic","children":[{"title":"Task A","type":"task"},{"title":"Task B","type":"task"}]}`
	out, err := h.RunWithStdin(input, "hc", "create")
	require.NoError(t, err)
	require.NotEmpty(t, out)

	items, err := h.RunJSONLines("hc", "list", "--json")
	require.NoError(t, err)
	require.Len(t, items, 3)

	var rootID string
	var epicCount, taskCount int
	for _, item := range items {
		switch item["type"] {
		case "epic":
			epicCount++
			rootID, _ = item["id"].(string)
		case "task":
			taskCount++
		}
	}
	assert.Equal(t, 1, epicCount, "should have 1 epic")
	assert.Equal(t, 2, taskCount, "should have 2 tasks")
	require.NotEmpty(t, rootID, "root epic id should not be empty")

	for _, item := range items {
		if item["type"] == "task" {
			epicID, _ := item["epic_id"].(string)
			assert.Equal(t, rootID, epicID, "task epic_id should match root epic id")
		}
	}
}

func TestHCCreateParentChild(t *testing.T) {
	h := NewHarness(t)

	epicLines, err := h.RunJSONLines("hc", "create", "Parent Epic", "--type", "epic")
	require.NoError(t, err)
	require.Len(t, epicLines, 1)

	epicID, ok := epicLines[0]["id"].(string)
	require.True(t, ok)

	childLines, err := h.RunJSONLines("hc", "create", "Child Task", "--type", "task", "--parent", epicID)
	require.NoError(t, err)
	require.Len(t, childLines, 1)

	childEpicID, _ := childLines[0]["epic_id"].(string)
	assert.Equal(t, epicID, childEpicID, "child epic_id should match parent epic id")

	depth, _ := childLines[0]["depth"].(float64)
	assert.Equal(t, float64(1), depth, "child depth should be 1")
}

func TestHCList(t *testing.T) {
	h := NewHarness(t)

	_, err := h.RunJSONLines("hc", "create", "Epic One", "--type", "epic")
	require.NoError(t, err)
	_, err = h.RunJSONLines("hc", "create", "Epic Two", "--type", "epic")
	require.NoError(t, err)

	allItems, err := h.RunJSONLines("hc", "list", "--json")
	require.NoError(t, err)
	assert.Len(t, allItems, 2)

	openItems, err := h.RunJSONLines("hc", "list", "--json", "--status", "open")
	require.NoError(t, err)
	assert.Len(t, openItems, 2)

	doneItems, err := h.RunJSONLines("hc", "list", "--json", "--status", "done")
	require.NoError(t, err)
	assert.Empty(t, doneItems)
}

func TestHCUpdate(t *testing.T) {
	h := NewHarness(t)

	lines, err := h.RunJSONLines("hc", "create", "My Epic", "--type", "epic")
	require.NoError(t, err)
	require.Len(t, lines, 1)

	id, ok := lines[0]["id"].(string)
	require.True(t, ok)

	updateLines, err := h.RunJSONLines("hc", "update", id, "--status", "in_progress")
	require.NoError(t, err)
	require.Len(t, updateLines, 1)
	assert.Equal(t, "in_progress", updateLines[0]["status"])
}

func TestHCCheckpoint(t *testing.T) {
	h := NewHarness(t)

	lines, err := h.RunJSONLines("hc", "create", "My Epic", "--type", "epic")
	require.NoError(t, err)
	require.Len(t, lines, 1)

	id, ok := lines[0]["id"].(string)
	require.True(t, ok)

	checkpointLines, err := h.RunJSONLines("hc", "checkpoint", id, "Work started")
	require.NoError(t, err)
	require.Len(t, checkpointLines, 1)
	assert.Equal(t, "checkpoint", checkpointLines[0]["type"])
	assert.Equal(t, "Work started", checkpointLines[0]["message"])

	showLines, err := h.RunJSONLines("hc", "show", "--json", id)
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(showLines), 2, "show should return item + at least one activity")

	var foundCheckpoint bool
	for _, line := range showLines {
		if line["type"] == "checkpoint" && line["message"] == "Work started" {
			foundCheckpoint = true
			break
		}
	}
	assert.True(t, foundCheckpoint, "should find checkpoint activity in show output")
}

func TestHCNext(t *testing.T) {
	h := NewHarness(t)

	out, err := h.Run("hc", "next")
	require.Error(t, err, "next should error when no session detected; got output: %s", out)
}

func TestHCContext(t *testing.T) {
	h := NewHarness(t)

	epicLines, err := h.RunJSONLines("hc", "create", "Context Epic", "--type", "epic")
	require.NoError(t, err)
	require.Len(t, epicLines, 1)

	epicID, ok := epicLines[0]["id"].(string)
	require.True(t, ok)

	contextLines, err := h.RunJSONLines("hc", "context", "--json", epicID)
	require.NoError(t, err)
	require.Len(t, contextLines, 1)

	line := contextLines[0]
	assert.Contains(t, line, "Epic", "context should have 'Epic' field")
	assert.Contains(t, line, "Counts", "context should have 'Counts' field")
	assert.Contains(t, line, "MyTasks", "context should have 'MyTasks' field")
	assert.Contains(t, line, "AllOpenTasks", "context should have 'AllOpenTasks' field")
}

func TestHCPruneDryRun(t *testing.T) {
	h := NewHarness(t)

	createLines, err := h.RunJSONLines("hc", "create", "Done Epic", "--type", "epic")
	require.NoError(t, err)
	require.Len(t, createLines, 1)

	id, ok := createLines[0]["id"].(string)
	require.True(t, ok)

	_, err = h.RunJSONLines("hc", "update", id, "--status", "done")
	require.NoError(t, err)

	pruneLines, err := h.RunJSONLines("hc", "prune", "--dry-run", "--older-than", "0s")
	require.NoError(t, err)
	require.Len(t, pruneLines, 1)

	count, ok := pruneLines[0]["count"].(float64)
	require.True(t, ok, "prune result should have numeric 'count' field: %v", pruneLines[0])
	assert.GreaterOrEqual(t, count, float64(1))

	listLines, err := h.RunJSONLines("hc", "list", "--json")
	require.NoError(t, err)
	require.Len(t, listLines, 1, "dry-run should not delete items")
}
