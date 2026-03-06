//go:build integration

package integration

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// createBlockerFixture creates an epic with two tasks and returns (epicID, task1ID, task2ID).
// task1 is the blocker, task2 is the blocked item.
func createBlockerFixture(t *testing.T, h *Harness) (epicID, task1ID, task2ID string) {
	t.Helper()

	epicLines, err := h.RunJSONLines("hc", "create", "Blocker Epic", "--type", "epic")
	require.NoError(t, err)
	epicID = epicLines[0]["id"].(string)

	t1Lines, err := h.RunJSONLines("hc", "create", "Blocker Task", "--type", "task", "--parent", epicID)
	require.NoError(t, err)
	task1ID = t1Lines[0]["id"].(string)

	t2Lines, err := h.RunJSONLines("hc", "create", "Blocked Task", "--type", "task", "--parent", epicID)
	require.NoError(t, err)
	task2ID = t2Lines[0]["id"].(string)

	return epicID, task1ID, task2ID
}

func TestHCUpdateAddBlocker(t *testing.T) {
	h := NewHarness(t)
	_, blockerID, blockedID := createBlockerFixture(t, h)

	// Add blocker: blockerID blocks blockedID
	lines, err := h.RunJSONLines("hc", "update", blockedID, "--add-blocker", blockerID)
	require.NoError(t, err)
	require.Len(t, lines, 1)

	// Fetch the blocked item and verify blocker_ids
	showLines, err := h.RunJSONLines("hc", "show", blockedID, "--json")
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(showLines), 1)

	blockerIDs, ok := showLines[0]["blocker_ids"].([]any)
	require.True(t, ok, "blocker_ids should be a list")
	require.Len(t, blockerIDs, 1)
	assert.Equal(t, blockerID, blockerIDs[0])
}

func TestHCUpdateRemoveBlocker(t *testing.T) {
	h := NewHarness(t)
	_, blockerID, blockedID := createBlockerFixture(t, h)

	// Add then remove
	_, err := h.RunJSONLines("hc", "update", blockedID, "--add-blocker", blockerID)
	require.NoError(t, err)

	_, err = h.RunJSONLines("hc", "update", blockedID, "--remove-blocker", blockerID)
	require.NoError(t, err)

	// Verify blocker_ids is now absent/empty
	showLines, err := h.RunJSONLines("hc", "show", blockedID, "--json")
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(showLines), 1)

	// blocker_ids should be absent or empty after removal
	if ids, ok := showLines[0]["blocker_ids"]; ok && ids != nil {
		idList, _ := ids.([]any)
		assert.Empty(t, idList, "blocker_ids should be empty after removal")
	}
}

func TestHCUpdateAddBlocker_Cycle(t *testing.T) {
	h := NewHarness(t)
	_, task1ID, task2ID := createBlockerFixture(t, h)

	// task1 blocks task2
	_, err := h.RunJSONLines("hc", "update", task2ID, "--add-blocker", task1ID)
	require.NoError(t, err)

	// Now try task2 blocks task1 — should fail with cycle error
	_, err = h.Run("hc", "update", task1ID, "--add-blocker", task2ID)
	require.Error(t, err, "adding a cyclic blocker should fail")
	assert.Contains(t, err.Error(), "cyclic")
}

func TestHCUpdateAddBlocker_MutuallyExclusive(t *testing.T) {
	h := NewHarness(t)
	_, task1ID, task2ID := createBlockerFixture(t, h)

	_, err := h.Run("hc", "update", task2ID, "--add-blocker", task1ID, "--remove-blocker", task1ID)
	require.Error(t, err, "--add-blocker and --remove-blocker together should fail")
	assert.Contains(t, err.Error(), "mutually exclusive")
}

func TestHCShowBlockers(t *testing.T) {
	h := NewHarness(t)
	_, blockerID, blockedID := createBlockerFixture(t, h)

	// Wire the blocker relationship
	_, err := h.RunJSONLines("hc", "update", blockedID, "--add-blocker", blockerID)
	require.NoError(t, err)

	// Plain text show should contain "Blockers"
	out, err := h.RunStdout("hc", "show", blockedID)
	require.NoError(t, err)
	assert.Contains(t, out, "Blockers (1)", "plain text show should render blockers section")
	assert.Contains(t, out, blockerID, "blocker ID should appear in output")
}

func TestHCCreateBulk_Blockers(t *testing.T) {
	h := NewHarness(t)

	input := `{
		"title": "Blocker Bulk Epic",
		"type": "epic",
		"children": [
			{"ref": "jwt", "title": "JWT middleware", "type": "task"},
			{"title": "Login endpoint", "type": "task", "blockers": ["jwt"]}
		]
	}`

	out, err := h.RunWithStdin(input, "hc", "create")
	require.NoError(t, err)

	lines, err := parseJSONLines(strings.TrimSpace(out))
	require.NoError(t, err)
	require.Len(t, lines, 3, "should have epic + 2 tasks")

	// Find the Login endpoint task (it has the blocker)
	var loginID, jwtID string
	for _, l := range lines {
		switch l["title"] {
		case "Login endpoint":
			loginID = l["id"].(string)
		case "JWT middleware":
			jwtID = l["id"].(string)
		}
	}
	require.NotEmpty(t, loginID, "Login endpoint task should be created")
	require.NotEmpty(t, jwtID, "JWT middleware task should be created")

	// Verify that Login endpoint has JWT middleware as a blocker
	showLines, err := h.RunJSONLines("hc", "show", loginID, "--json")
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(showLines), 1)

	blockerIDs, ok := showLines[0]["blocker_ids"].([]any)
	require.True(t, ok, "Login endpoint should have blocker_ids")
	require.Len(t, blockerIDs, 1)
	assert.Equal(t, jwtID, blockerIDs[0])
}

func TestHCNext_ExplicitBlockerExcluded(t *testing.T) {
	h := NewHarness(t)
	epicID, blockerID, blockedID := createBlockerFixture(t, h)

	// Wire: blockerID blocks blockedID
	_, err := h.RunJSONLines("hc", "update", blockedID, "--add-blocker", blockerID)
	require.NoError(t, err)

	// Next should return only the non-blocked task (blockerID itself)
	nextLines, err := h.RunJSONLines("hc", "next", epicID)
	require.NoError(t, err)
	require.Len(t, nextLines, 1)
	assert.Equal(t, blockerID, nextLines[0]["id"], "next should return the blocker task, not the blocked one")
}
