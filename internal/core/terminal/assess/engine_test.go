package assess_test

import (
	"testing"

	"github.com/colonyops/hive/internal/core/terminal/assess"
	"github.com/stretchr/testify/assert"
)

func TestEngine_Assess(t *testing.T) {
	engine := assess.NewEngine()
	for _, f := range fixtures {
		t.Run(f.name, func(t *testing.T) {
			content := fixtureContent(t, f.file)
			got := engine.Assess(assess.Snapshot{Content: content, Tool: f.tool})
			assert.Equal(t, f.expectedState, got.State, "state mismatch — %s", f.purpose)
			assert.Equal(t, f.expectedHold, got.Hold, "hold mismatch — %s", f.purpose)
		})
	}
}

// TestEngine_Assess_EmptyStringNeverPanics covers edge (a)'s literal empty
// string case directly (the whitespace-only fixture in the table above
// covers the file-based variant); AboveBox must be "" in both, never a panic.
func TestEngine_Assess_EmptyStringNeverPanics(t *testing.T) {
	engine := assess.NewEngine()
	got := engine.Assess(assess.Snapshot{Content: "", Tool: "claude"})
	assert.Equal(t, assess.StateUnknown, got.State)
	assert.Empty(t, got.AboveBox)
}

func TestEngine_Assess_WhitespaceOnlyAboveBoxEmpty(t *testing.T) {
	engine := assess.NewEngine()
	content := fixtureContent(t, "edge/whitespace.txt")
	got := engine.Assess(assess.Snapshot{Content: content, Tool: "claude"})
	assert.Empty(t, got.AboveBox)
}

// TestEngine_Assess_NoBoxAboveBoxFallsBackToViewportBlock locks in edge (b)'s
// documented fallback: with no prompt box, abovePromptBox degrades to the
// last contiguous block of the viewport rather than an empty string. This
// vim fixture happens to be a single contiguous block from top to bottom
// (the "~" filler lines are non-blank, so nothing separates them from the
// buffer content) — it proves the fallback engages and returns real content,
// not that the fallback returns the "whole viewport" in general; see
// TestComputeRegions_NoBoxFallsBackToLastContiguousBlock in regions_test.go
// for the case where stale content above a blank-line boundary exists and
// must be excluded.
func TestEngine_Assess_NoBoxAboveBoxFallsBackToViewportBlock(t *testing.T) {
	engine := assess.NewEngine()
	content := fixtureContent(t, "edge/fullscreen-vim.txt")
	got := engine.Assess(assess.Snapshot{Content: content, Tool: "claude"})
	assert.Contains(t, got.AboveBox, "func main")
	assert.Contains(t, got.AboveBox, "internal/example/main.go")
}

// TestEngine_Assess_UnknownRuleIDEmpty confirms a no-match Assessment carries
// no misleading rule attribution.
func TestEngine_Assess_UnknownRuleIDEmpty(t *testing.T) {
	engine := assess.NewEngine()
	got := engine.Assess(assess.Snapshot{Content: "", Tool: "claude"})
	assert.Empty(t, got.RuleID)
	assert.Empty(t, got.Signals)
}

func TestEngine_Assess_UnclassifiedToolUsesGenericRules(t *testing.T) {
	engine := assess.NewEngine()
	content := fixtureContent(t, "generic/yes-no-prompt.txt")
	got := engine.Assess(assess.Snapshot{Content: content, Tool: "some-unclassified-tool"})
	assert.Equal(t, assess.StateApproval, got.State,
		"a tool with no dedicated rule set must still fall back to the generic rule set, not silently unknown")
}
