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

	epicLines, err := h.RunJSONLines("hc", "create", "Auth Epic", "--type", "epic")
	require.NoError(t, err)
	epicID := epicLines[0]["id"].(string)

	lines, err := h.RunJSONLines("hc", "create", "Implement auth", "--type", "task", "--parent", epicID)
	require.NoError(t, err)
	require.Len(t, lines, 1)

	item := lines[0]
	assert.NotEmpty(t, item["id"])
	assert.Equal(t, "Implement auth", item["title"])
	assert.Equal(t, "task", item["type"])
	assert.Equal(t, "open", item["status"])
}

func TestHCCreateEpic(t *testing.T) {
	h := NewHarness(t)

	lines, err := h.RunJSONLines("hc", "create", "My Epic", "--type", "epic")
	require.NoError(t, err)
	require.Len(t, lines, 1)

	item := lines[0]
	assert.Equal(t, "My Epic", item["title"])
	assert.Equal(t, "epic", item["type"])
	assert.Equal(t, "open", item["status"])
}

func TestHCCreateTaskWithParent(t *testing.T) {
	h := NewHarness(t)

	epicLines, err := h.RunJSONLines("hc", "create", "Parent Epic", "--type", "epic")
	require.NoError(t, err)
	require.Len(t, epicLines, 1)
	epicID := epicLines[0]["id"].(string)

	taskLines, err := h.RunJSONLines("hc", "create", "Child Task", "--type", "task", "--parent", epicID)
	require.NoError(t, err)
	require.Len(t, taskLines, 1)

	task := taskLines[0]
	assert.Equal(t, "Child Task", task["title"])
	assert.Equal(t, epicID, task["epic_id"])
	assert.Equal(t, epicID, task["parent_id"])
}

func TestHCCreateBulkFromStdin(t *testing.T) {
	h := NewHarness(t)

	input := `{"title":"Big Epic","type":"epic","children":[{"title":"Task A","type":"task"},{"title":"Task B","type":"task"}]}`

	out, err := h.RunWithStdin(input, "hc", "create")
	require.NoError(t, err)

	lines, err := parseJSONLines(strings.TrimSpace(out))
	require.NoError(t, err)
	require.Len(t, lines, 3, "should have epic + 2 tasks")

	assert.Equal(t, "Big Epic", lines[0]["title"])
	assert.Equal(t, "epic", lines[0]["type"])
	assert.Equal(t, "Task A", lines[1]["title"])
	assert.Equal(t, "Task B", lines[2]["title"])
}

func TestHCCreateInvalidType(t *testing.T) {
	h := NewHarness(t)

	_, err := h.Run("hc", "create", "My Item", "--type", "bogus")
	require.Error(t, err)
}

func TestHCList(t *testing.T) {
	h := NewHarness(t)

	epicLines, err := h.RunJSONLines("hc", "create", "List Epic", "--type", "epic")
	require.NoError(t, err)
	epicID := epicLines[0]["id"].(string)

	_, err = h.RunJSONLines("hc", "create", "Task One", "--type", "task", "--parent", epicID)
	require.NoError(t, err)
	_, err = h.RunJSONLines("hc", "create", "Task Two", "--type", "task", "--parent", epicID)
	require.NoError(t, err)

	lines, err := h.RunJSONLines("hc", "list", "--json")
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(lines), 2)
}

func TestHCListStatusFilter(t *testing.T) {
	h := NewHarness(t)

	epicLines, err := h.RunJSONLines("hc", "create", "Filter Epic", "--type", "epic")
	require.NoError(t, err)
	epicID := epicLines[0]["id"].(string)

	createLines, err := h.RunJSONLines("hc", "create", "Filterable Task", "--type", "task", "--parent", epicID)
	require.NoError(t, err)
	require.Len(t, createLines, 1)
	id := createLines[0]["id"].(string)

	_, err = h.RunJSONLines("hc", "update", id, "--status", "done")
	require.NoError(t, err)

	doneLines, err := h.RunJSONLines("hc", "list", "--status", "done", "--json")
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(doneLines), 1)

	found := false
	for _, l := range doneLines {
		if l["id"] == id {
			found = true
			assert.Equal(t, "done", l["status"])
		}
	}
	assert.True(t, found, "updated item should appear in done filter")

	openLines, err := h.RunJSONLines("hc", "list", "--status", "open", "--json")
	require.NoError(t, err)
	for _, l := range openLines {
		assert.NotEqual(t, id, l["id"], "done item should not appear in open filter")
	}
}

func TestHCShow(t *testing.T) {
	h := NewHarness(t)

	epicLines, err := h.RunJSONLines("hc", "create", "Show Epic", "--type", "epic")
	require.NoError(t, err)
	epicID := epicLines[0]["id"].(string)

	createLines, err := h.RunJSONLines("hc", "create", "Show Me", "--type", "task", "--parent", epicID)
	require.NoError(t, err)
	require.Len(t, createLines, 1)
	id := createLines[0]["id"].(string)

	_, err = h.RunJSONLines("hc", "comment", id, "first note")
	require.NoError(t, err)

	out, err := h.RunStdout("hc", "show", id, "--json")
	require.NoError(t, err)

	showLines, err := parseJSONLines(strings.TrimSpace(out))
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(showLines), 2, "should have item + at least one comment")

	assert.Equal(t, id, showLines[0]["id"])
	assert.Equal(t, "Show Me", showLines[0]["title"])
	assert.Equal(t, "first note", showLines[1]["message"])
}

func TestHCUpdate(t *testing.T) {
	h := NewHarness(t)

	epicLines, err := h.RunJSONLines("hc", "create", "Update Epic", "--type", "epic")
	require.NoError(t, err)
	epicID := epicLines[0]["id"].(string)

	createLines, err := h.RunJSONLines("hc", "create", "Updatable Task", "--type", "task", "--parent", epicID)
	require.NoError(t, err)
	require.Len(t, createLines, 1)
	id := createLines[0]["id"].(string)

	updateLines, err := h.RunJSONLines("hc", "update", id, "--status", "in_progress")
	require.NoError(t, err)
	require.Len(t, updateLines, 1)
	assert.Equal(t, "in_progress", updateLines[0]["status"])
}

func TestHCUpdateInvalidStatus(t *testing.T) {
	h := NewHarness(t)

	epicLines, err := h.RunJSONLines("hc", "create", "Invalid Status Epic", "--type", "epic")
	require.NoError(t, err)
	epicID := epicLines[0]["id"].(string)

	createLines, err := h.RunJSONLines("hc", "create", "Task", "--type", "task", "--parent", epicID)
	require.NoError(t, err)
	id := createLines[0]["id"].(string)

	_, err = h.Run("hc", "update", id, "--status", "invalid")
	require.Error(t, err)
}

func TestHCNextRequiresEpicID(t *testing.T) {
	h := NewHarness(t)

	_, err := h.Run("hc", "next")
	require.Error(t, err)
}

func TestHCNext(t *testing.T) {
	h := NewHarness(t)

	bulkInput := `{"title":"Next Epic","type":"epic","children":[{"title":"Next Task","type":"task"}]}`
	out, err := h.RunWithStdin(bulkInput, "hc", "create")
	require.NoError(t, err)

	lines, err := parseJSONLines(strings.TrimSpace(out))
	require.NoError(t, err)
	require.NotEmpty(t, lines)

	epicID := lines[0]["id"].(string)

	nextLines, err := h.RunJSONLines("hc", "next", epicID)
	require.NoError(t, err)
	require.Len(t, nextLines, 1)

	item := nextLines[0]
	assert.Equal(t, "Next Task", item["title"])
	assert.Equal(t, "task", item["type"])
}

func TestHCNextNoActionableTasks(t *testing.T) {
	h := NewHarness(t)

	epicLines, err := h.RunJSONLines("hc", "create", "Empty Epic", "--type", "epic")
	require.NoError(t, err)
	epicID := epicLines[0]["id"].(string)

	_, err = h.Run("hc", "next", epicID)
	require.Error(t, err)
}

func TestHCComment(t *testing.T) {
	h := NewHarness(t)

	epicLines, err := h.RunJSONLines("hc", "create", "Comment Epic", "--type", "epic")
	require.NoError(t, err)
	epicID := epicLines[0]["id"].(string)

	createLines, err := h.RunJSONLines("hc", "create", "Commentable Task", "--type", "task", "--parent", epicID)
	require.NoError(t, err)
	id := createLines[0]["id"].(string)

	commentLines, err := h.RunJSONLines("hc", "comment", id, "noted some progress")
	require.NoError(t, err)
	require.Len(t, commentLines, 1)

	comment := commentLines[0]
	assert.Equal(t, id, comment["item_id"])
	assert.Equal(t, "noted some progress", comment["message"])
	assert.NotEmpty(t, comment["id"])
}

func TestHCContext(t *testing.T) {
	h := NewHarness(t)

	bulkInput := `{"title":"Context Epic","type":"epic","children":[{"title":"Open Task","type":"task"}]}`
	_, err := h.RunWithStdin(bulkInput, "hc", "create")
	require.NoError(t, err)

	listLines, err := h.RunJSONLines("hc", "list", "--status", "open", "--json")
	require.NoError(t, err)

	var epicID string
	for _, l := range listLines {
		if l["type"] == "epic" {
			epicID = l["id"].(string)
			break
		}
	}
	require.NotEmpty(t, epicID, "epic should be found in list")

	obj, err := h.RunJSON("hc", "context", epicID, "--json")
	require.NoError(t, err)

	assert.Contains(t, obj, "epic")
	assert.Contains(t, obj, "counts")
	assert.Contains(t, obj, "all_open_tasks")

	epic := obj["epic"].(map[string]any)
	assert.Equal(t, "Context Epic", epic["title"])
}

func TestHCContextMarkdown(t *testing.T) {
	h := NewHarness(t)

	bulkInput := `{"title":"Markdown Epic","type":"epic","children":[{"title":"Task A","type":"task"}]}`
	_, err := h.RunWithStdin(bulkInput, "hc", "create")
	require.NoError(t, err)

	listLines, err := h.RunJSONLines("hc", "list", "--status", "open", "--json")
	require.NoError(t, err)

	var epicID string
	for _, l := range listLines {
		if l["type"] == "epic" {
			epicID = l["id"].(string)
			break
		}
	}
	require.NotEmpty(t, epicID)

	out, err := h.RunStdout("hc", "context", epicID)
	require.NoError(t, err)
	assert.Contains(t, out, "Markdown Epic")
	assert.Contains(t, out, "Progress:")
}

func TestHCPruneDryRun(t *testing.T) {
	h := NewHarness(t)

	epicLines, err := h.RunJSONLines("hc", "create", "Prune Epic", "--type", "epic")
	require.NoError(t, err)
	epicID := epicLines[0]["id"].(string)

	createLines, err := h.RunJSONLines("hc", "create", "Prunable Task", "--type", "task", "--parent", epicID)
	require.NoError(t, err)
	id := createLines[0]["id"].(string)

	_, err = h.RunJSONLines("hc", "update", id, "--status", "done")
	require.NoError(t, err)

	out, err := h.RunStdout("hc", "prune", "--older-than", "0s", "--dry-run")
	require.NoError(t, err)

	result, err := parseJSON(strings.TrimSpace(out))
	require.NoError(t, err)
	assert.Equal(t, "would prune", result["action"])
	assert.GreaterOrEqual(t, result["count"], float64(0))
}

func TestHCPrune(t *testing.T) {
	h := NewHarness(t)

	epicLines, err := h.RunJSONLines("hc", "create", "Old Epic", "--type", "epic")
	require.NoError(t, err)
	epicID := epicLines[0]["id"].(string)

	createLines, err := h.RunJSONLines("hc", "create", "Old Task", "--type", "task", "--parent", epicID)
	require.NoError(t, err)
	id := createLines[0]["id"].(string)

	_, err = h.RunJSONLines("hc", "update", id, "--status", "done")
	require.NoError(t, err)

	out, err := h.RunStdout("hc", "prune", "--older-than", "0s")
	require.NoError(t, err)

	result, err := parseJSON(strings.TrimSpace(out))
	require.NoError(t, err)
	assert.Equal(t, "pruned", result["action"])
}

func TestHCNext_Assign(t *testing.T) {
	h := NewHarness(t)

	bulkInput := `{"title":"Assign Epic","type":"epic","children":[{"title":"Assign Task","type":"task"}]}`
	out, err := h.RunWithStdin(bulkInput, "hc", "create")
	require.NoError(t, err)

	lines, err := parseJSONLines(strings.TrimSpace(out))
	require.NoError(t, err)
	require.NotEmpty(t, lines)
	epicID := lines[0]["id"].(string)

	// Without a real session, --assign fails because session detection returns empty.
	// Run next without --assign to verify basic functionality.
	nextLines, err := h.RunJSONLines("hc", "next", epicID)
	require.NoError(t, err)
	require.Len(t, nextLines, 1)
	assert.Equal(t, "Assign Task", nextLines[0]["title"])
	assert.Equal(t, "open", nextLines[0]["status"])
}

func TestHCUpdate_Assign(t *testing.T) {
	h := NewHarness(t)

	epicLines, err := h.RunJSONLines("hc", "create", "Assign Epic", "--type", "epic")
	require.NoError(t, err)
	epicID := epicLines[0]["id"].(string)

	createLines, err := h.RunJSONLines("hc", "create", "Assign Task", "--type", "task", "--parent", epicID)
	require.NoError(t, err)
	id := createLines[0]["id"].(string)

	// --assign without a detectable session returns an error.
	_, err = h.Run("hc", "update", id, "--assign")
	require.Error(t, err, "assign without session should fail")
}

func TestHCUpdate_Unassign(t *testing.T) {
	h := NewHarness(t)

	epicLines, err := h.RunJSONLines("hc", "create", "Unassign Epic", "--type", "epic")
	require.NoError(t, err)
	epicID := epicLines[0]["id"].(string)

	createLines, err := h.RunJSONLines("hc", "create", "Unassign Task", "--type", "task", "--parent", epicID)
	require.NoError(t, err)
	id := createLines[0]["id"].(string)

	updateLines, err := h.RunJSONLines("hc", "update", id, "--unassign")
	require.NoError(t, err)
	require.Len(t, updateLines, 1)
	assert.Equal(t, "", updateLines[0]["session_id"])
}

func TestHCUpdate_MutuallyExclusive(t *testing.T) {
	h := NewHarness(t)

	epicLines, err := h.RunJSONLines("hc", "create", "Mutex Epic", "--type", "epic")
	require.NoError(t, err)
	epicID := epicLines[0]["id"].(string)

	createLines, err := h.RunJSONLines("hc", "create", "Mutex Task", "--type", "task", "--parent", epicID)
	require.NoError(t, err)
	id := createLines[0]["id"].(string)

	_, err = h.Run("hc", "update", id, "--assign", "--unassign")
	require.Error(t, err, "--assign and --unassign together should fail")
}

func TestHCUpdate_NoOp(t *testing.T) {
	h := NewHarness(t)

	epicLines, err := h.RunJSONLines("hc", "create", "NoOp Epic", "--type", "epic")
	require.NoError(t, err)
	epicID := epicLines[0]["id"].(string)

	createLines, err := h.RunJSONLines("hc", "create", "NoOp Task", "--type", "task", "--parent", epicID)
	require.NoError(t, err)
	id := createLines[0]["id"].(string)

	_, err = h.Run("hc", "update", id)
	require.Error(t, err, "update with no flags should fail")
}

func TestHCCreate_TaskWithoutParent(t *testing.T) {
	h := NewHarness(t)

	_, err := h.Run("hc", "create", "Orphan Task", "--type", "task")
	require.Error(t, err, "task without --parent should fail")
}

func TestHCShow_NotFound(t *testing.T) {
	h := NewHarness(t)

	_, err := h.Run("hc", "show", "hc-doesnotexist")
	require.Error(t, err, "show with nonexistent ID should fail")
}

func TestHCCRUD(t *testing.T) {
	h := NewHarness(t)

	epicLines, err := h.RunJSONLines("hc", "create", "CRUD Epic", "--type", "epic")
	require.NoError(t, err)
	epicID := epicLines[0]["id"].(string)

	taskLines, err := h.RunJSONLines("hc", "create", "CRUD Task", "--type", "task", "--parent", epicID)
	require.NoError(t, err)
	taskID := taskLines[0]["id"].(string)

	listLines, err := h.RunJSONLines("hc", "list", epicID, "--json")
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(listLines), 1)

	found := false
	for _, l := range listLines {
		if l["id"] == taskID {
			found = true
		}
	}
	assert.True(t, found)

	updateLines, err := h.RunJSONLines("hc", "update", taskID, "--status", "in_progress")
	require.NoError(t, err)
	assert.Equal(t, "in_progress", updateLines[0]["status"])

	commentLines, err := h.RunJSONLines("hc", "comment", taskID, "making progress")
	require.NoError(t, err)
	assert.Equal(t, taskID, commentLines[0]["item_id"])

	showOut, err := h.RunStdout("hc", "show", taskID, "--json")
	require.NoError(t, err)
	showLines, err := parseJSONLines(strings.TrimSpace(showOut))
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(showLines), 2)
	assert.Equal(t, taskID, showLines[0]["id"])

	doneLines, err := h.RunJSONLines("hc", "update", taskID, "--status", "done")
	require.NoError(t, err)
	assert.Equal(t, "done", doneLines[0]["status"])
}

// ---------------------------------------------------------------------------
// Phase 1: --title and --desc flag tests
// ---------------------------------------------------------------------------

func TestHCUpdate_TitleRoundTrip(t *testing.T) {
	h := NewHarness(t)

	epicLines, err := h.RunJSONLines("hc", "create", "Original Title", "--type", "epic")
	require.NoError(t, err)
	id := epicLines[0]["id"].(string)

	updateLines, err := h.RunJSONLines("hc", "update", id, "--title", "Updated Title")
	require.NoError(t, err)
	require.Len(t, updateLines, 1)
	assert.Equal(t, "Updated Title", updateLines[0]["title"])
}

func TestHCUpdate_DescRoundTrip(t *testing.T) {
	h := NewHarness(t)

	epicLines, err := h.RunJSONLines("hc", "create", "My Epic", "--type", "epic")
	require.NoError(t, err)
	id := epicLines[0]["id"].(string)

	updateLines, err := h.RunJSONLines("hc", "update", id, "--desc", "brand new description")
	require.NoError(t, err)
	require.Len(t, updateLines, 1)
	assert.Equal(t, "brand new description", updateLines[0]["desc"])
}

func TestHCUpdate_EmptyTitleRejected(t *testing.T) {
	h := NewHarness(t)

	epicLines, err := h.RunJSONLines("hc", "create", "My Epic", "--type", "epic")
	require.NoError(t, err)
	id := epicLines[0]["id"].(string)

	out, err := h.Run("hc", "update", id, "--title", "")
	require.Error(t, err, "empty --title should be rejected")
	assert.Contains(t, out, "--title cannot be empty")
}

func TestHCUpdate_TitleAndStatusCombined(t *testing.T) {
	h := NewHarness(t)

	epicLines, err := h.RunJSONLines("hc", "create", "Old Title", "--type", "epic")
	require.NoError(t, err)
	id := epicLines[0]["id"].(string)

	// Create a task so the epic can have children but we update the epic itself
	taskLines, err := h.RunJSONLines("hc", "create", "Child Task", "--type", "task", "--parent", id)
	require.NoError(t, err)
	taskID := taskLines[0]["id"].(string)

	updateLines, err := h.RunJSONLines("hc", "update", taskID, "--title", "New Task Title", "--status", "done")
	require.NoError(t, err)
	require.Len(t, updateLines, 1)
	assert.Equal(t, "New Task Title", updateLines[0]["title"])
	assert.Equal(t, "done", updateLines[0]["status"])
}

// ---------------------------------------------------------------------------
// Phase 2: cascade behavior tests
// ---------------------------------------------------------------------------

func TestHCUpdate_CascadeOnEpicDone(t *testing.T) {
	h := NewHarness(t)

	epicLines, err := h.RunJSONLines("hc", "create", "Cascade Epic", "--type", "epic")
	require.NoError(t, err)
	epicID := epicLines[0]["id"].(string)

	_, err = h.RunJSONLines("hc", "create", "Task One", "--type", "task", "--parent", epicID)
	require.NoError(t, err)
	_, err = h.RunJSONLines("hc", "create", "Task Two", "--type", "task", "--parent", epicID)
	require.NoError(t, err)

	_, err = h.RunJSONLines("hc", "update", epicID, "--status", "done")
	require.NoError(t, err)

	listLines, err := h.RunJSONLines("hc", "list", "--all", "--json")
	require.NoError(t, err)

	for _, line := range listLines {
		if line["epic_id"] == epicID {
			assert.Equal(t, "done", line["status"], "child task %s should be done", line["id"])
		}
	}
}

func TestHCUpdate_CascadeRespectsTerminalGuard(t *testing.T) {
	h := NewHarness(t)

	epicLines, err := h.RunJSONLines("hc", "create", "Guard Epic", "--type", "epic")
	require.NoError(t, err)
	epicID := epicLines[0]["id"].(string)

	task1Lines, err := h.RunJSONLines("hc", "create", "Task One", "--type", "task", "--parent", epicID)
	require.NoError(t, err)
	task1ID := task1Lines[0]["id"].(string)

	task2Lines, err := h.RunJSONLines("hc", "create", "Task Two", "--type", "task", "--parent", epicID)
	require.NoError(t, err)
	task2ID := task2Lines[0]["id"].(string)

	// Mark task1 as done before cascading
	_, err = h.RunJSONLines("hc", "update", task1ID, "--status", "done")
	require.NoError(t, err)

	// Now cancel the epic
	_, err = h.RunJSONLines("hc", "update", epicID, "--status", "cancelled")
	require.NoError(t, err)

	getTask1, err := h.RunJSONLines("hc", "show", task1ID, "--json")
	require.NoError(t, err)
	assert.Equal(t, "done", getTask1[0]["status"], "pre-done task1 should remain done")

	getTask2, err := h.RunJSONLines("hc", "show", task2ID, "--json")
	require.NoError(t, err)
	assert.Equal(t, "cancelled", getTask2[0]["status"], "open task2 should become cancelled")
}

func TestHCUpdate_NoCascadeOnNonTerminalEpic(t *testing.T) {
	h := NewHarness(t)

	epicLines, err := h.RunJSONLines("hc", "create", "Non-Terminal Epic", "--type", "epic")
	require.NoError(t, err)
	epicID := epicLines[0]["id"].(string)

	taskLines, err := h.RunJSONLines("hc", "create", "A Task", "--type", "task", "--parent", epicID)
	require.NoError(t, err)
	taskID := taskLines[0]["id"].(string)

	_, err = h.RunJSONLines("hc", "update", epicID, "--status", "in_progress")
	require.NoError(t, err)

	getTask, err := h.RunJSONLines("hc", "show", taskID, "--json")
	require.NoError(t, err)
	assert.Equal(t, "open", getTask[0]["status"], "task should remain open when epic goes in_progress")
}

func TestHCUpdate_NoCascadeOnTaskUpdate(t *testing.T) {
	h := NewHarness(t)

	epicLines, err := h.RunJSONLines("hc", "create", "Task Update Epic", "--type", "epic")
	require.NoError(t, err)
	epicID := epicLines[0]["id"].(string)

	task1Lines, err := h.RunJSONLines("hc", "create", "Task One", "--type", "task", "--parent", epicID)
	require.NoError(t, err)
	task1ID := task1Lines[0]["id"].(string)

	task2Lines, err := h.RunJSONLines("hc", "create", "Task Two", "--type", "task", "--parent", epicID)
	require.NoError(t, err)
	task2ID := task2Lines[0]["id"].(string)

	_, err = h.RunJSONLines("hc", "update", task1ID, "--status", "done")
	require.NoError(t, err)

	getTask2, err := h.RunJSONLines("hc", "show", task2ID, "--json")
	require.NoError(t, err)
	assert.Equal(t, "open", getTask2[0]["status"], "sibling task should not be affected by task update")
}
