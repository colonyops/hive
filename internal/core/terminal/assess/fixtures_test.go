package assess_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/colonyops/hive/internal/core/terminal/assess"
	"github.com/stretchr/testify/require"
)

// fixture mirrors the pattern in
// internal/core/terminal/content/scorer_fixtures_test.go: a table of
// {name, expected..., purpose}, except content lives in a committed
// testdata/<tool>/<scenario>.txt file instead of an inline string literal.
type fixture struct {
	name          string
	file          string // path relative to testdata/, e.g. "claude/working-spinner.txt"
	tool          string
	expectedState assess.State
	expectedHold  bool
	purpose       string
}

func fixtureContent(t *testing.T, file string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", file))
	require.NoError(t, err, "loading fixture %s", file)
	return string(data)
}

// fixtures is the shared corpus consumed by both TestEngine_Assess (does the
// engine classify correctly?) and TestParity_OldVsNewEngine (does the new
// classification agree with the old detector, except where documented?).
//
// Six regression fixtures reproduce the corpus-documented failure class
// (2026-07-21 handoff): matching substrings anywhere in a fixed recent-lines
// window cannot distinguish current UI from transcript history. Three edge
// fixtures cover degenerate/malformed input. All content below is synthetic
// and sanitized — no real paths, repo names, or captured session data.
var fixtures = []fixture{
	// --- claude ---
	{
		name:          "claude-working-spinner",
		file:          "claude/working-spinner.txt",
		tool:          "claude",
		expectedState: assess.StateWorking,
		purpose:       "spinner-shape rule replaces the old whimsical-word list: any spinner glyph + gerund + ellipsis is 'working'.",
	},
	{
		name:          "claude-working-token-stats",
		file:          "claude/working-token-stats.txt",
		tool:          "claude",
		expectedState: assess.StateWorking,
		purpose:       "a token-stats status line ('(45s · 1876 tokens)') is 'working' even with no spinner glyph on the line.",
	},
	{
		name:          "claude-idle-empty-box",
		file:          "claude/idle-empty-box.txt",
		tool:          "claude",
		expectedState: assess.StateIdle,
		purpose:       "an empty/placeholder prompt box is idle once a turn has completed.",
	},
	{
		name:          "claude-approval-permission-dialog",
		file:          "claude/approval-permission-dialog.txt",
		tool:          "claude",
		expectedState: assess.StateApproval,
		purpose:       "a live permission dialog rendered inside the prompt box is approval.",
	},
	{
		name:          "claude-question-alpha-beta",
		file:          "claude/question-alpha-beta.txt",
		tool:          "claude",
		expectedState: assess.StateQuestion,
		purpose:       "regression #3: an AskUserQuestion-style question with selectable options is 'question', distinct from 'approval'.",
	},
	{
		name:          "claude-typed-unsubmitted",
		file:          "claude/typed-unsubmitted.txt",
		tool:          "claude",
		expectedState: assess.StateIdle,
		purpose:       "regression #6: typed-but-unsubmitted input in the prompt box is idle, not working/approval.",
	},
	{
		name:          "claude-hold-search-prompt",
		file:          "claude/hold-search-prompt.txt",
		tool:          "claude",
		expectedState: assess.StateUnknown,
		expectedHold:  true,
		purpose:       "the file-search overlay is a transient screen: hold the previously published status instead of guessing.",
	},
	{
		name:          "claude-hold-transcript-viewer",
		file:          "claude/hold-transcript-viewer.txt",
		tool:          "claude",
		expectedState: assess.StateUnknown,
		expectedHold:  true,
		purpose:       "the transcript viewer ('ctrl+r to toggle') is a transient screen: hold, don't classify.",
	},
	{
		name:          "claude-bare-prompt-working",
		file:          "claude/bare-prompt-working.txt",
		tool:          "claude",
		expectedState: assess.StateWorking,
		purpose:       "modern bare-prompt UI regression (found live 2026-08-04): a spinner above a rule-delimited ❯ prompt (no ╭│╰ box) is working, not idle via the box-less prompt-glyph fallback.",
	},
	{
		name:          "claude-bare-prompt-idle",
		file:          "claude/bare-prompt-idle.txt",
		tool:          "claude",
		expectedState: assess.StateIdle,
		purpose:       "modern bare-prompt UI regression (found live 2026-08-04): a completed turn with no spinner above a bare rule-delimited ❯ prompt is idle.",
	},
	{
		name:          "claude-bare-prompt-typed",
		file:          "claude/bare-prompt-typed.txt",
		tool:          "claude",
		expectedState: assess.StateIdle,
		purpose:       "modern bare-prompt UI regression (found live 2026-08-04): typed-but-unsubmitted text on the rule-delimited ❯ line is idle, not working/approval.",
	},
	{
		name:          "claude-bare-prompt-question",
		file:          "claude/bare-prompt-question.txt",
		tool:          "claude",
		expectedState: assess.StateQuestion,
		purpose:       "modern AskUserQuestion live-dialog regression (found live 2026-08-04): the question/options block rendered as bare text between two rules (no ╭│╰ box) is question, detected via the dialog's own footer chrome since promptBoxBody+abovePromptBox can't see it.",
	},
	{
		name:          "claude-bare-prompt-after-answer",
		file:          "claude/bare-prompt-after-answer.txt",
		tool:          "claude",
		expectedState: assess.StateWorking,
		purpose:       "counter-fixture for the AskUserQuestion live-dialog regression: once answered, the dialog is fully replaced by a summary line and the footer chrome is gone, so this must NOT be question even though the summary still mentions the numbered option.",
	},

	// --- codex ---
	{
		name:          "codex-working-spinner",
		file:          "codex/working-spinner.txt",
		tool:          "codex",
		expectedState: assess.StateWorking,
		purpose:       "spinner-shape rule applies identically to codex's status line.",
	},
	{
		name:          "codex-idle-empty-box",
		file:          "codex/idle-empty-box.txt",
		tool:          "codex",
		expectedState: assess.StateIdle,
		purpose:       "an empty prompt box is idle for codex just as for claude.",
	},
	{
		name:          "codex-approval-live",
		file:          "codex/approval-live.txt",
		tool:          "codex",
		expectedState: assess.StateApproval,
		purpose:       "a live codex command-approval dialog ('Press enter to confirm or esc to cancel') is approval.",
	},
	{
		name:          "codex-approval-post-denial-stale",
		file:          "codex/approval-post-denial-stale.txt",
		tool:          "codex",
		expectedState: assess.StateIdle,
		purpose:       "regression #1: stale approval text left in scrollback after a denial must not re-trigger approval; the current empty box is idle.",
	},
	{
		name:          "codex-question-deploy-target",
		file:          "codex/question-deploy-target.txt",
		tool:          "codex",
		expectedState: assess.StateQuestion,
		purpose:       "regression #4: codex's AskUserQuestion-style dialog is 'question'.",
	},
	{
		name:          "codex-hold-transcript-viewer",
		file:          "codex/hold-transcript-viewer.txt",
		tool:          "codex",
		expectedState: assess.StateUnknown,
		expectedHold:  true,
		purpose:       "codex's transcript viewer is a transient screen: hold.",
	},
	{
		name:          "codex-bare-working",
		file:          "codex/bare-working.txt",
		tool:          "codex",
		expectedState: assess.StateWorking,
		purpose:       "codex stuck-idle regression (found live 2026-08-04): real Codex (v0.146.0) renders a persistent ╭│╰ welcome banner (not the prompt) plus a bare '• Working (Ns • esc to interrupt)' status line with no box and no rule around it; the banner must be excluded from box detection and the status line matched via codex/working-status-line.",
	},
	{
		name:          "codex-bare-idle",
		file:          "codex/bare-idle.txt",
		tool:          "codex",
		expectedState: assess.StateIdle,
		purpose:       "codex stuck-idle regression (found live 2026-08-04): codex's real idle prompt is a bare '›'-prefixed placeholder line, no box, no rule — bottomPromptGlyphPattern can't match it (it isn't bare), so codex/bare-prompt must.",
	},
	{
		name:          "codex-bare-done",
		file:          "codex/bare-done.txt",
		tool:          "codex",
		expectedState: assess.StateIdle,
		purpose:       "codex stuck-idle regression (found live 2026-08-04): a longer post-turn transcript (multiple prior '›' turns, rule-delimited command output, a welcome banner) must not confuse the small bottomLines window into matching stale content — only the current bare '›' placeholder line drives idle.",
	},
	{
		name:          "codex-trust-dialog",
		file:          "codex/trust-dialog.txt",
		tool:          "codex",
		expectedState: assess.StateQuestion,
		purpose:       "codex boot-time trust-dialog regression (found live 2026-08-04): 'Do you trust the contents of this directory?' renders with no box and each visual section (context line, question, options, footer) separated by its own blank line, so questionRule's contiguous-block scoping can't see the whole shape; codex/trust-dialog anchors on the dialog's own vocabulary instead.",
	},

	// --- generic (tools with no dedicated rule set, e.g. pi) ---
	{
		name:          "generic-post-interrupt-stale",
		file:          "generic/post-interrupt-stale.txt",
		tool:          "pi",
		expectedState: assess.StateIdle,
		purpose:       "regression #2: stale spinner/working markers left visible after an interrupt must not re-trigger working; the current bare prompt is idle.",
	},
	{
		name:          "generic-question-alpha-beta",
		file:          "generic/question-alpha-beta.txt",
		tool:          "pi",
		expectedState: assess.StateQuestion,
		purpose:       "regression #5: pi has no dedicated rule set, but the generic question rule (a question line plus a numbered option list) still catches it.",
	},
	{
		name:          "generic-yes-no-prompt",
		file:          "generic/yes-no-prompt.txt",
		tool:          "pi",
		expectedState: assess.StateApproval,
		purpose:       "generic rule set: a bare '(y/n)' confirmation is approval even with no tool-specific vocabulary.",
	},
	{
		name:          "generic-idle-bare-prompt",
		file:          "generic/idle-bare-prompt.txt",
		tool:          "pi",
		expectedState: assess.StateIdle,
		purpose:       "generic rule set: a bare prompt glyph with nothing else on the line is idle.",
	},
	{
		name:          "generic-pi-bare-working",
		file:          "generic/pi-bare-working.txt",
		tool:          "pi",
		expectedState: assess.StateWorking,
		purpose:       "pi stuck-active regression (found live 2026-08-04): pi's spinner line uses an ASCII three-dot ellipsis and is indented ('⠧ Working...'), not the Unicode ellipsis flush-left shape the pattern originally required.",
	},
	{
		name:          "generic-pi-bare-idle",
		file:          "generic/pi-bare-idle.txt",
		tool:          "pi",
		expectedState: assess.StateIdle,
		purpose:       "pi stuck-active regression (found live 2026-08-04): pi's idle input area is a rule-delimited box with a blank (glyph-less) middle line, not a ❯/> prompt; detectRulePromptBox and the generic rule set's promptEmptyRule must both recognize it as an empty box.",
	},
	{
		name:          "generic-pi-bare-done",
		file:          "generic/pi-bare-done.txt",
		tool:          "pi",
		expectedState: assess.StateIdle,
		purpose:       "pi stuck-active regression (found live 2026-08-04): after a turn completes, pi's rule-delimited box is empty again; this is what lets status.Tracker de-escalate active back to ready instead of holding active forever.",
	},

	// --- edge cases ---
	{
		name:          "edge-whitespace-only",
		file:          "edge/whitespace.txt",
		tool:          "claude",
		expectedState: assess.StateUnknown,
		purpose:       "edge (a): whitespace-only content matches no rule and must never panic.",
	},
	{
		name:          "edge-fullscreen-no-box",
		file:          "edge/fullscreen-vim.txt",
		tool:          "claude",
		expectedState: assess.StateUnknown,
		purpose:       "edge (b): a fullscreen TUI with no prompt box (vim) falls back to whole-viewport scanning and matches no claude rule.",
	},
	{
		name:          "edge-clipped-box-borders",
		file:          "edge/clipped-box.txt",
		tool:          "claude",
		expectedState: assess.StateUnknown,
		purpose:       "edge (c): a clipped pane (top ╭ border scrolled out) must degrade to no-box behavior, not misparse a partial box.",
	},
}
