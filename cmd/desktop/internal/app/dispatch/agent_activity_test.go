package dispatch

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// These are synthetic capture-pane fixtures, not the raw-pty ring tails
// hc-alqns469 spiked and falsified — ClassifyAgentScreen's input is a tmux
// capture-pane -p -J screen (see tmuxcc.Manager.CapturePane), which is what
// Hive's assessment engine is tuned against. Real capture-pane fixtures need a live
// tmux session and are out of scope here (see hc-ou4o02zx).

func TestClassifyAgentScreenReady(t *testing.T) {
	t.Parallel()

	screen := strings.Join([]string{
		"Welcome to Claude Code!",
		"",
		"╭──────────────────────────────╮",
		"│ >                              │",
		"╰──────────────────────────────╯",
		"❯",
	}, "\n")

	assert.Equal(t, AgentActivityReady, ClassifyAgentScreen("claude", screen))
}

func TestClassifyAgentScreenActive(t *testing.T) {
	t.Parallel()

	screen := strings.Join([]string{
		"✳ cogitating… (12s · 480 tokens · esc to interrupt)",
		"",
	}, "\n")

	assert.Equal(t, AgentActivityActive, ClassifyAgentScreen("claude", screen))
}

func TestClassifyAgentScreenApproval(t *testing.T) {
	t.Parallel()

	screen := strings.Join([]string{
		"Do you want to make this edit to main.go?",
		"❯ Yes",
		"  Yes, allow always",
		"  No, and tell Claude what to do differently",
	}, "\n")

	assert.Equal(t, AgentActivityApproval, ClassifyAgentScreen("claude", screen))
}

// TestClassifyAgentScreenApprovalDoesNotStickAfterAnAnswer asserts the
// answered-fixture half of the spike's falsifying pair (hc-alqns469): once a
// prompt is answered and the CLI moves on to a fresh input prompt, the
// screen must classify as ready again, not approval.
func TestClassifyAgentScreenApprovalDoesNotStickAfterAnAnswer(t *testing.T) {
	t.Parallel()

	lines := []string{
		"Do you want to make this edit to main.go?",
		"❯ Yes",
		"  No, and tell Claude what to do differently",
	}
	// Detector only looks at the last 15 non-empty lines (the staleness
	// window the spike's fixtures assert against), so enough scrolled-past
	// output has to separate the answered prompt from the final screen for
	// this to be a real test of recency rather than a fixture too short to
	// exercise it.
	for i := range 20 {
		lines = append(lines, "edited line "+strings.Repeat("x", i%3+1))
	}
	lines = append(lines, "Applied 1 edit to main.go", "", "❯")
	screen := strings.Join(lines, "\n")

	assert.Equal(t, AgentActivityReady, ClassifyAgentScreen("claude", screen))
}

// TestClassifyAgentScreenBusyOutranksApproval asserts the assessment engine's
// priority: a screen carrying both a busy indicator and
// leftover approval-shaped text (e.g. mid-scroll) classifies as active, never
// approval.
func TestClassifyAgentScreenBusyOutranksApproval(t *testing.T) {
	t.Parallel()

	screen := strings.Join([]string{
		"Do you want to proceed?",
		"⠋ working… (esc to interrupt)",
	}, "\n")

	assert.Equal(t, AgentActivityActive, ClassifyAgentScreen("claude", screen))
}

func TestClassifyAgentScreenCodexApproval(t *testing.T) {
	t.Parallel()

	screen := "Would you like to run the following command?\nPress enter to confirm or esc to cancel"

	assert.Equal(t, AgentActivityApproval, ClassifyAgentScreen("codex", screen))
}
