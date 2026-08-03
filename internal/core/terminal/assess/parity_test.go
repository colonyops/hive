package assess_test

import (
	"strings"
	"testing"

	"github.com/colonyops/hive/internal/core/terminal"
	"github.com/colonyops/hive/internal/core/terminal/assess"
	"github.com/stretchr/testify/assert"
)

// divergence documents one accepted old-detector-vs-new-engine mismatch,
// keyed by fixture name. Both pipelines are still live during this phase —
// nothing is wired or deleted — so this table is the parity contract: any
// mismatch not listed here is a regression, and any listed entry that stops
// diverging is stale and must be removed.
type divergence struct {
	oldStatus terminal.Status
	newState  assess.State
	reason    string
}

var expectedDivergences = map[string]divergence{
	"claude-question-alpha-beta": {
		oldStatus: terminal.StatusReady,
		newState:  assess.StateQuestion,
		reason:    "the old detector has no question concept and falls through to its default-ready fallback; the new engine distinguishes question from approval and idle.",
	},
	"claude-hold-search-prompt": {
		oldStatus: terminal.StatusReady,
		newState:  assess.StateUnknown,
		reason:    "the old detector has no transient-screen concept and falls through to default-ready; the new engine correctly holds (unknown+hold) instead of misreporting ready.",
	},
	"claude-hold-transcript-viewer": {
		oldStatus: terminal.StatusReady,
		newState:  assess.StateUnknown,
		reason:    "same as claude-hold-search-prompt: old detector default-ready fallback vs new engine's explicit hold.",
	},
	"codex-approval-post-denial-stale": {
		oldStatus: terminal.StatusApproval,
		newState:  assess.StateIdle,
		reason:    "regression #1 (this is the bug the redesign exists to fix): old detector's fixed recent-lines window still contains the dismissed dialog's text and reports approval; the new engine's region-scoped matching correctly excludes it and reports idle.",
	},
	"codex-question-deploy-target": {
		oldStatus: terminal.StatusReady,
		newState:  assess.StateQuestion,
		reason:    "same class as claude-question-alpha-beta: no old-detector equivalent for question.",
	},
	"codex-hold-transcript-viewer": {
		oldStatus: terminal.StatusReady,
		newState:  assess.StateUnknown,
		reason:    "same as claude-hold-transcript-viewer: old default-ready fallback vs new explicit hold.",
	},
}

// oldEquivalent maps a new State to the old terminal.Status it corresponds
// to, per the plan's decision: Active↔working, Approval↔approval,
// Ready↔idle. question and unknown have no old equivalent — they are new
// classifications the old detector cannot express, so any fixture landing on
// one of those two states must appear in expectedDivergences.
func oldEquivalent(s assess.State) (terminal.Status, bool) {
	switch s {
	case assess.StateWorking:
		return terminal.StatusActive, true
	case assess.StateApproval:
		return terminal.StatusApproval, true
	case assess.StateIdle:
		return terminal.StatusReady, true
	default:
		return "", false
	}
}

// TestParity_OldVsNewEngine runs every claude/codex fixture through both the
// still-live old terminal.Detector and the new assess.Engine. This is the
// self-verifying gate the plan requires: mise run test alone is the verdict.
// Any divergence not present in expectedDivergences fails the test, and (kept
// honest in both directions) any table entry that no longer diverges also
// fails it.
func TestParity_OldVsNewEngine(t *testing.T) {
	engine := assess.NewEngine()

	for _, f := range fixtures {
		// Parity only applies to claude/codex fixtures — the edge and
		// generic-tool (pi) fixtures exercise engine behavior the old
		// per-tool Detector was never asked to handle.
		if !strings.HasPrefix(f.file, "claude/") && !strings.HasPrefix(f.file, "codex/") {
			continue
		}

		t.Run(f.name, func(t *testing.T) {
			content := fixtureContent(t, f.file)

			oldStatus := terminal.NewDetector(f.tool).DetectStatus(content)
			newAssessment := engine.Assess(assess.Snapshot{Content: content, Tool: f.tool})

			mappedOldEquivalent, comparable := oldEquivalent(newAssessment.State)
			actuallyDivergent := !comparable || mappedOldEquivalent != oldStatus

			div, isDocumented := expectedDivergences[f.name]

			switch {
			case isDocumented && !actuallyDivergent:
				t.Fatalf("fixture %q is listed in expectedDivergences (old=%s new=%s) but old and new now agree (old=%s, new=%s) — remove the stale entry",
					f.name, div.oldStatus, div.newState, oldStatus, newAssessment.State)
			case isDocumented:
				assert.Equal(t, div.oldStatus, oldStatus, "documented divergence %q: old status changed", f.name)
				assert.Equal(t, div.newState, newAssessment.State, "documented divergence %q: new state changed", f.name)
			case actuallyDivergent:
				t.Fatalf("undocumented divergence for fixture %q: old detector=%s, new engine state=%s (ruleID=%q) — add an entry to expectedDivergences or fix the mismatch",
					f.name, oldStatus, newAssessment.State, newAssessment.RuleID)
			}
		})
	}
}
