//go:build integration

package integration

import (
	"encoding/json"
	"os/exec"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDetect_AssessmentAndInMode proves the real 10-field `tmux list-panes`
// format parses end-to-end: `hive detect` reports a one-shot Stage 1
// assessment (state + rule ID) for an agent pane, and inMode flips to true
// once the pane enters copy-mode (`#{pane_in_mode}`).
func TestDetect_AssessmentAndInMode(t *testing.T) {
	config := `version: "0.2.4"
git_path: git
agents:
  default: testbash
  testbash:
    command: bash
tmux:
  preview_window_matcher: ["bash"]
rules:
  - windows:
      - name: agent
        command: bash
`
	h := NewHarness(t).WithConfig(config)
	repo := createBareRepo(t, "detect-assess-repo")
	cleanupTmuxSession(t, "detect-assess")

	newOut, err := h.Run("new", "--background", "--remote", repo, "detect-assess")
	require.NoError(t, err, "hive new failed: %s", newOut)
	assertTmuxSessionExists(t, "detect-assess")

	// Render agent-like content: a bare "(y/n)" confirmation is the generic
	// rule set's approval shape, independent of any tool-specific vocabulary.
	out, err := exec.Command("tmux", "send-keys", "-t", "detect-assess", "echo 'Continue? (y/n)'", "Enter").CombinedOutput()
	require.NoError(t, err, "tmux send-keys: %s", out)

	var paneID string
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		outJSON, runErr := h.RunStdout("detect", "detect-assess")
		if !assert.NoError(c, runErr) {
			return
		}
		var got detectResult
		if !assert.NoError(c, json.Unmarshal([]byte(outJSON), &got), outJSON) {
			return
		}
		var agentPane *detectPaneResult
		for i := range got.Panes {
			if got.Panes[i].IsAgent {
				agentPane = &got.Panes[i]
				break
			}
		}
		if !assert.NotNil(c, agentPane, "expected an agent pane: %#v", got.Panes) {
			return
		}
		assert.Equal(c, "approval", agentPane.Assessment)
		assert.Equal(c, "generic/yes-no-prompt", agentPane.RuleID)
		assert.False(c, agentPane.InMode, "pane must not report copy-mode before entering it")
		paneID = agentPane.PaneID
	}, 5*time.Second, 200*time.Millisecond)
	require.NotEmpty(t, paneID)

	out, err = exec.Command("tmux", "copy-mode", "-t", paneID).CombinedOutput()
	require.NoError(t, err, "tmux copy-mode: %s", out)

	require.EventuallyWithT(t, func(c *assert.CollectT) {
		outJSON, runErr := h.RunStdout("detect", "detect-assess")
		if !assert.NoError(c, runErr) {
			return
		}
		var got detectResult
		if !assert.NoError(c, json.Unmarshal([]byte(outJSON), &got), outJSON) {
			return
		}
		var pane *detectPaneResult
		for i := range got.Panes {
			if got.Panes[i].PaneID == paneID {
				pane = &got.Panes[i]
				break
			}
		}
		if !assert.NotNil(c, pane, "expected pane %s to still be listed: %#v", paneID, got.Panes) {
			return
		}
		assert.True(c, pane.InMode, "pane must report inMode=true after entering copy-mode")
	}, 5*time.Second, 200*time.Millisecond)
}
