//go:build integration

package integration

import (
	"encoding/json"
	"os/exec"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type detectResult struct {
	Session string             `json:"session"`
	Panes   []detectPaneResult `json:"panes"`
}

type detectPaneResult struct {
	PaneID      string `json:"paneID"`
	PanePID     int64  `json:"panePID"`
	WindowIndex string `json:"windowIndex"`
	WindowName  string `json:"windowName"`
	IsAgent     bool   `json:"isAgent"`
	Tool        string `json:"tool"`
	Confidence  string `json:"confidence"`
	Tier        int    `json:"tier"`
}

func TestDetect_NoSession(t *testing.T) {
	h := NewHarness(t)
	_, err := h.RunStdout("detect", "nonexistent")
	require.Error(t, err)
}

func TestDetect_SinglePaneSession(t *testing.T) {
	h := NewHarness(t)
	repo := createBareRepo(t, "detect-repo")
	cleanupTmuxSession(t, "detect-single")

	_, err := h.Run("new", "--remote", repo, "detect-single")
	require.NoError(t, err)
	assertTmuxSessionExists(t, "detect-single")

	out, err := h.RunStdout("detect", "detect-single")
	require.NoError(t, err)

	var got detectResult
	require.NoError(t, json.Unmarshal([]byte(out), &got), out)
	assert.Equal(t, "detect-single", got.Session)
	require.NotEmpty(t, got.Panes)
	for _, pane := range got.Panes {
		assert.NotEmpty(t, pane.PaneID)
		assert.NotZero(t, pane.PanePID)
		assert.NotEmpty(t, pane.WindowIndex)
	}
}

func TestDetect_MultiPaneSession(t *testing.T) {
	h := NewHarness(t)
	repo := createBareRepo(t, "detect-multi-repo")
	cleanupTmuxSession(t, "detect-multi")

	_, err := h.Run("new", "--remote", repo, "detect-multi")
	require.NoError(t, err)
	assertTmuxSessionExists(t, "detect-multi")

	out, err := exec.Command("tmux", "split-window", "-t", "detect-multi", "sleep 600").CombinedOutput()
	require.NoError(t, err, "tmux split-window: %s", out)
	t.Cleanup(func() { _ = exec.Command("tmux", "kill-session", "-t", "detect-multi").Run() })

	outJSON, err := h.RunStdout("detect", "detect-multi")
	require.NoError(t, err)

	var got detectResult
	require.NoError(t, json.Unmarshal([]byte(outJSON), &got), outJSON)
	require.GreaterOrEqual(t, len(got.Panes), 2)
	var nonAgent bool
	for _, pane := range got.Panes {
		if !pane.IsAgent {
			nonAgent = true
			assert.Empty(t, pane.Tool)
			assert.Zero(t, pane.Tier)
		}
	}
	assert.True(t, nonAgent, "expected at least one non-agent pane: %#v", got.Panes)
}
