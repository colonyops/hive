//go:build integration

package integration

import (
	"os/exec"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTmuxSessionCreated(t *testing.T) {
	h := NewHarness(t)
	repo := createBareRepo(t, "tmux-repo")
	cleanupTmuxSession(t, "tmux-test")

	_, err := h.Run("new", "--remote", repo, "tmux-test")
	require.NoError(t, err)

	assertTmuxSessionExists(t, "tmux-test")
}

func TestTmuxWindows(t *testing.T) {
	h := NewHarness(t)
	repo := createBareRepo(t, "tmux-win-repo")
	cleanupTmuxSession(t, "tmux-win-test")

	_, err := h.Run("new", "--remote", repo, "tmux-win-test")
	require.NoError(t, err)

	assertTmuxHasWindows(t, "tmux-win-test")
}

func TestTmuxCapture(t *testing.T) {
	h := NewHarness(t)
	repo := createBareRepo(t, "tmux-cap-repo")
	cleanupTmuxSession(t, "tmux-cap-test")

	_, err := h.Run("new", "--remote", repo, "tmux-cap-test")
	require.NoError(t, err)

	assertTmuxSessionExists(t, "tmux-cap-test")

	out, err := exec.Command("tmux", "capture-pane", "-t", "tmux-cap-test", "-p").CombinedOutput()
	require.NoError(t, err, "tmux capture-pane: %s", out)
}

func TestTmuxListAll(t *testing.T) {
	h := NewHarness(t)
	repo := createBareRepo(t, "tmux-list-repo")
	cleanupTmuxSession(t, "tmux-list-a")
	cleanupTmuxSession(t, "tmux-list-b")

	_, err := h.Run("new", "--remote", repo, "tmux-list-a")
	require.NoError(t, err)
	_, err = h.Run("new", "--remote", repo, "tmux-list-b")
	require.NoError(t, err)

	assertTmuxSessionExists(t, "tmux-list-a", "tmux-list-b")

	lsOut, err := h.Run("ls")
	require.NoError(t, err)
	assert.Contains(t, lsOut, "tmux-list-a")
	assert.Contains(t, lsOut, "tmux-list-b")
}
