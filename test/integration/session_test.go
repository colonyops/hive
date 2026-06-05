//go:build integration

package integration

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSessionCreate(t *testing.T) {
	h := NewHarness(t)
	repo := createBareRepo(t, "test-repo")

	out, err := h.Run("new", "--remote", repo, "test-session")
	require.NoError(t, err, "hive new should succeed: %s", out)
	assert.Contains(t, out, "Session created")
}

func TestSessionCreateMultiple(t *testing.T) {
	h := NewHarness(t)
	repo := createBareRepo(t, "test-repo")

	out1, err := h.Run("new", "--remote", repo, "session-one")
	require.NoError(t, err, "first session: %s", out1)

	out2, err := h.Run("new", "--remote", repo, "session-two")
	require.NoError(t, err, "second session: %s", out2)

	// Both should show up in ls
	out, err := h.Run("ls")
	require.NoError(t, err, "hive ls: %s", out)
	assert.Contains(t, out, "session-one")
	assert.Contains(t, out, "session-two")
}

func TestSessionCreateNoName(t *testing.T) {
	h := NewHarness(t)

	_, err := h.Run("new")
	require.Error(t, err, "hive new without name should fail")
}

func TestSessionLsEmpty(t *testing.T) {
	h := NewHarness(t)

	out, err := h.Run("ls")
	require.NoError(t, err, "hive ls on empty should succeed: %s", out)
	assert.Contains(t, out, "No sessions found")
}

func TestSessionLsJSON(t *testing.T) {
	h := NewHarness(t)
	repo := createBareRepo(t, "json-repo")

	_, err := h.Run("new", "--remote", repo, "json-test")
	require.NoError(t, err)

	lines, err := h.RunJSONLines("ls", "--json")
	require.NoError(t, err)
	require.Len(t, lines, 1)

	entry := lines[0]
	assert.Equal(t, "json-test", entry["name"])
	assert.Equal(t, "active", entry["state"])
	assert.Contains(t, entry, "id")
	assert.Contains(t, entry, "repo")
	assert.Contains(t, entry, "inbox")
}

func TestSessionDelete(t *testing.T) {
	h := NewHarness(t)
	repo := createBareRepo(t, "delete-repo")

	_, err := h.Run("new", "--remote", repo, "to-delete")
	require.NoError(t, err)

	lines, err := h.RunJSONLines("ls", "--json")
	require.NoError(t, err)
	require.Len(t, lines, 1)
	id := lines[0]["id"].(string)

	_, err = h.Run("session", "delete", id)
	require.NoError(t, err)

	lines, err = h.RunJSONLines("ls", "--json")
	require.NoError(t, err)
	assert.Empty(t, lines, "session should be gone after delete")
}

func TestSessionDeleteMissingID(t *testing.T) {
	h := NewHarness(t)

	_, err := h.Run("session", "delete")
	require.Error(t, err, "delete without ID should fail")
}

func TestSessionRecycle(t *testing.T) {
	h := NewHarness(t)
	repo := createBareRepo(t, "recycle-repo")

	_, err := h.Run("new", "--remote", repo, "to-recycle")
	require.NoError(t, err)

	lines, err := h.RunJSONLines("ls", "--json")
	require.NoError(t, err)
	require.Len(t, lines, 1)
	id := lines[0]["id"].(string)

	_, err = h.Run("session", "recycle", id)
	require.NoError(t, err)

	lines, err = h.RunJSONLines("ls", "--json")
	require.NoError(t, err)
	require.Len(t, lines, 1)
	assert.Equal(t, "recycled", lines[0]["state"], "session state should be recycled")
}

func TestSessionRecycleMissingID(t *testing.T) {
	h := NewHarness(t)

	_, err := h.Run("session", "recycle")
	require.Error(t, err, "recycle without ID should fail")
}
