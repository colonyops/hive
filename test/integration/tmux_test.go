//go:build integration

package integration

import (
	"fmt"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTmuxSessionCreated(t *testing.T) {
	h := NewHarness(t)
	repo := createBareRepo(t, "tmux-repo")
	cleanupTmuxSession(t, "tmux-test")

	_, err := h.Run("new", "--remote", repo, "tmux-test")
	require.NoError(t, err)

	pollFor(t, 5*time.Second, 200*time.Millisecond, func() error {
		out, err := exec.Command("tmux", "list-sessions", "-F", "#{session_name}").CombinedOutput()
		if err != nil {
			return fmt.Errorf("tmux list-sessions: %w: %s", err, out)
		}
		if !strings.Contains(string(out), "tmux-test") {
			return fmt.Errorf("tmux session 'tmux-test' not found in: %s", out)
		}
		return nil
	})
}

func TestTmuxWindows(t *testing.T) {
	h := NewHarness(t)
	repo := createBareRepo(t, "tmux-win-repo")
	cleanupTmuxSession(t, "tmux-win-test")

	_, err := h.Run("new", "--remote", repo, "tmux-win-test")
	require.NoError(t, err)

	pollFor(t, 5*time.Second, 200*time.Millisecond, func() error {
		out, err := exec.Command("tmux", "list-windows", "-t", "tmux-win-test", "-F", "#{window_name}").CombinedOutput()
		if err != nil {
			return fmt.Errorf("tmux list-windows: %w: %s", err, out)
		}
		if strings.TrimSpace(string(out)) == "" {
			return fmt.Errorf("no windows found")
		}
		return nil
	})
}

func TestTmuxCapture(t *testing.T) {
	h := NewHarness(t)
	repo := createBareRepo(t, "tmux-cap-repo")
	cleanupTmuxSession(t, "tmux-cap-test")

	_, err := h.Run("new", "--remote", repo, "tmux-cap-test")
	require.NoError(t, err)

	pollFor(t, 5*time.Second, 200*time.Millisecond, func() error {
		out, err := exec.Command("tmux", "list-sessions", "-F", "#{session_name}").CombinedOutput()
		if err != nil {
			return fmt.Errorf("tmux: %w: %s", err, out)
		}
		if !strings.Contains(string(out), "tmux-cap-test") {
			return fmt.Errorf("session not found")
		}
		return nil
	})

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

	pollFor(t, 5*time.Second, 200*time.Millisecond, func() error {
		out, err := exec.Command("tmux", "list-sessions", "-F", "#{session_name}").CombinedOutput()
		if err != nil {
			return fmt.Errorf("tmux: %w: %s", err, out)
		}
		output := string(out)
		if !strings.Contains(output, "tmux-list-a") {
			return fmt.Errorf("session a not found")
		}
		if !strings.Contains(output, "tmux-list-b") {
			return fmt.Errorf("session b not found")
		}
		return nil
	})

	lsOut, err := h.Run("ls")
	require.NoError(t, err)
	assert.Contains(t, lsOut, "tmux-list-a")
	assert.Contains(t, lsOut, "tmux-list-b")
}
