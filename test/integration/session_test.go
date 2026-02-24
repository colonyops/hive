//go:build integration

package integration

import (
	"strings"
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

	out, err := h.Run("ls", "--json")
	require.NoError(t, err, "hive ls --json: %s", out)

	lines, err := parseJSONLines(strings.TrimSpace(out))
	require.NoError(t, err, "parse JSON lines")
	require.Len(t, lines, 1)

	entry := lines[0]
	assert.Equal(t, "json-test", entry["name"])
	assert.Equal(t, "active", entry["state"])
	assert.Contains(t, entry, "id")
	assert.Contains(t, entry, "repo")
	assert.Contains(t, entry, "inbox")
}
