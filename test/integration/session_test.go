//go:build integration

package integration

import (
	"os"
	"path/filepath"
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

func TestSessionCreateSubcommandJSON(t *testing.T) {
	h := NewHarness(t)
	repo := createBareRepo(t, "create-json-repo")

	entry, err := h.RunJSON("session", "create", "--json", "--remote", repo, "orchestrated")
	require.NoError(t, err)

	assert.Equal(t, "orchestrated", entry["name"])
	assert.Equal(t, "active", entry["state"])
	assert.NotEmpty(t, entry["id"])
	assert.NotEmpty(t, entry["path"])
	assert.NotEmpty(t, entry["inbox"])
	assert.NotEmpty(t, entry["slug"])

	// The created session must be discoverable by the returned ID.
	id := entry["id"].(string)
	shown, err := h.RunJSON("session", "show", id, "--json")
	require.NoError(t, err)
	assert.Equal(t, id, shown["id"])
}

func TestSessionShow(t *testing.T) {
	h := NewHarness(t)
	repo := createBareRepo(t, "show-repo")

	created, err := h.RunJSON("session", "create", "--json", "--remote", repo, "show-me", "--tags", "a", "--tags", "b")
	require.NoError(t, err)
	id := created["id"].(string)

	entry, err := h.RunJSON("session", "show", id, "--json")
	require.NoError(t, err)
	assert.Equal(t, "show-me", entry["name"])
	assert.Equal(t, "active", entry["state"])
	assert.ElementsMatch(t, []any{"a", "b"}, entry["tags"])
}

func TestSessionShowUnknownID(t *testing.T) {
	h := NewHarness(t)

	_, err := h.Run("session", "show", "nonexistent")
	require.Error(t, err, "show with unknown ID should fail")
}

func TestSessionUpdateName(t *testing.T) {
	h := NewHarness(t)
	repo := createBareRepo(t, "update-repo")

	created, err := h.RunJSON("session", "create", "--json", "--remote", repo, "old-name")
	require.NoError(t, err)
	id := created["id"].(string)

	entry, err := h.RunJSON("session", "update", id, "--name", "new-name", "--json")
	require.NoError(t, err)
	assert.Equal(t, "new-name", entry["name"])
	assert.Equal(t, "new-name", entry["slug"])
}

func TestSessionUpdateGroup(t *testing.T) {
	h := NewHarness(t)
	repo := createBareRepo(t, "group-repo")

	created, err := h.RunJSON("session", "create", "--json", "--remote", repo, "grouped")
	require.NoError(t, err)
	id := created["id"].(string)

	entry, err := h.RunJSON("session", "update", id, "--group", "backend", "--json")
	require.NoError(t, err)
	assert.Equal(t, "backend", entry["group"])

	entry, err = h.RunJSON("session", "update", id, "--clear-group", "--json")
	require.NoError(t, err)
	_, hasGroup := entry["group"]
	assert.False(t, hasGroup, "group should be omitted after clearing")
}

func TestSessionUpdateNoFlags(t *testing.T) {
	h := NewHarness(t)
	repo := createBareRepo(t, "noflags-repo")

	created, err := h.RunJSON("session", "create", "--json", "--remote", repo, "noop")
	require.NoError(t, err)
	id := created["id"].(string)

	_, err = h.Run("session", "update", id)
	require.Error(t, err, "update without modification flags should fail")
}

func TestSessionDeleteRiskGate(t *testing.T) {
	h := NewHarness(t)
	repo := createBareRepo(t, "risk-repo")

	created, err := h.RunJSON("session", "create", "--json", "--remote", repo, "risky")
	require.NoError(t, err)
	id := created["id"].(string)
	path := created["path"].(string)

	// Introduce uncommitted changes.
	require.NoError(t, os.WriteFile(filepath.Join(path, "dirty.txt"), []byte("wip"), 0o644))

	_, err = h.Run("session", "delete", id)
	require.Error(t, err, "delete of dirty session without --force should fail")

	entry, err := h.RunJSON("session", "delete", id, "--force", "--json")
	require.NoError(t, err)
	assert.Equal(t, true, entry["deleted"])

	lines, err := h.RunJSONLines("ls", "--json")
	require.NoError(t, err)
	assert.Empty(t, lines, "session should be gone after forced delete")
}

func TestSessionRecycleJSON(t *testing.T) {
	h := NewHarness(t)
	repo := createBareRepo(t, "recycle-json-repo")

	created, err := h.RunJSON("session", "create", "--json", "--remote", repo, "recycle-json")
	require.NoError(t, err)
	id := created["id"].(string)

	entry, err := h.RunJSON("session", "recycle", id, "--json")
	require.NoError(t, err)
	assert.Equal(t, "recycled", entry["state"])
	assert.Equal(t, id, entry["id"])
}
