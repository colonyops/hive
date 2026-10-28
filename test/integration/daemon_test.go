//go:build integration

package integration

import (
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newDaemonHarness turns experimental.daemon on over the standard fixture and
// stops whatever daemon the test started before its data dir goes away.
func newDaemonHarness(t *testing.T) *Harness {
	t.Helper()
	fixture, err := os.ReadFile(testdataConfig())
	require.NoError(t, err)
	h := NewHarness(t).WithConfig(string(fixture) + "experimental:\n  daemon: true\n")
	t.Cleanup(func() {
		_, _ = h.Run("daemon", "stop")
	})
	return h
}

func daemonPID(t *testing.T, h *Harness) int {
	t.Helper()
	status, err := h.RunJSON("daemon", "status", "--json", "--no-start")
	require.NoError(t, err)
	return int(status["pid"].(float64))
}

func TestDaemonSurvivesAKillAndKeepsTheAgent(t *testing.T) {
	h := newDaemonHarness(t)
	repo := createBareRepo(t, "daemon-repo")
	cleanupTmuxSession(t, "d1")

	created, err := h.RunJSON("session", "create", "--json", "--background", "--remote", repo, "d1")
	require.NoError(t, err)
	id := created["id"].(string)
	assert.Equal(t, "active", created["state"])

	status, err := h.RunJSON("daemon", "status", "--json", "--no-start")
	require.NoError(t, err)
	assert.Equal(t, true, status["db_ok"])
	sessions := status["sessions"].(map[string]any)
	assert.InDelta(t, 1, sessions["active"], 0)

	listed, err := h.RunJSONLines("session", "list", "--json")
	require.NoError(t, err)
	require.Len(t, listed, 1)
	assert.Equal(t, "d1", listed[0]["name"])

	firstPID := daemonPID(t, h)
	require.NoError(t, syscall.Kill(firstPID, syscall.SIGKILL))
	require.Eventually(t, func() bool {
		_, err := h.Run("daemon", "status", "--json", "--no-start")
		return err != nil
	}, 5*time.Second, 50*time.Millisecond, "the killed daemon still answers")

	listed, err = h.RunJSONLines("session", "list", "--json")
	require.NoError(t, err, "a session call starts a new daemon")
	require.Len(t, listed, 1)
	assert.NotEqual(t, firstPID, daemonPID(t, h))
	require.NoError(t, exec.Command("tmux", "has-session", "-t", "=d1").Run(), "killing the daemon must not take the agent with it")

	out, err := h.Run("session", "delete", "--force", id)
	require.NoError(t, err, "delete: %s", out)
	listed, err = h.RunJSONLines("session", "list", "--json")
	require.NoError(t, err)
	assert.Empty(t, listed)
}

func TestDaemonRunRefusesASecondInstance(t *testing.T) {
	h := newDaemonHarness(t)
	_, err := h.RunJSON("daemon", "status", "--json")
	require.NoError(t, err, "status starts the daemon")

	out, err := h.Run("daemon", "run")
	require.Error(t, err)
	assert.Contains(t, out, "already running")
	assert.FileExists(t, filepath.Join(h.DataDir(), "hived.lock"))
}
