package assess

import (
	"regexp"
	"strings"
)

// codexApprovalPhrases are Codex CLI's approval-dialog vocabulary. Scoped to
// phraseRule's anti-staleness regions — this is exactly what fixes the
// documented "post-permission-denial idle stayed approval" regression: the
// dismissed dialog's leftover text lives above a blank-line boundary and is
// excluded by abovePromptBox, so only a live dialog can match.
var codexApprovalPhrases = []string{
	"Would you like to run the following command?",
	"Press enter to confirm or esc to cancel",
	"Allow command?",
}

// codexStatusWindowLines bounds how far back the bare-UI codex rules
// (codexWorkingStatusLineRule, codexBarePromptRule) look in bottomLines.
// Real Codex (v0.146.0+) renders no ╭│╰ input box and no ─ rule around its
// working indicator or its bare "›" prompt line — both regions.box-based
// scoping strategies this engine otherwise relies on find nothing to anchor
// to here, and abovePromptBox's box-less fallback (last contiguous block)
// only ever reaches the trailing footer line, well below either signal. A
// small window is what keeps this safe despite scanning outside the
// contiguous-block restriction: both signals sit within a couple of lines
// of the footer, while older transcript turns (which also render "›" lines)
// are always separated from the current turn by far more than this many
// lines of output, a rule, or both.
const codexStatusWindowLines = 8

var (
	// codexWorkingStatusPattern matches Codex's mid-turn status line, e.g.
	// "• Working (3s • esc to interrupt)". The leading "•" deliberately
	// isn't added to SpinnerGlyphs: Codex reuses it as the bullet for every
	// transcript entry ("• hello", "• Ran git log …"), so a bare-glyph match
	// would poison churn normalization and risk false positives against
	// ordinary transcript lines. Anchoring on the literal "Working (" plus
	// "esc to interrupt" instead keys on this specific status line's shape,
	// not the bullet.
	codexWorkingStatusPattern = regexp.MustCompile(`(?m)^\s*• Working \(.*esc to interrupt`)

	// codexBarePromptPattern matches Codex's bare "›"-prefixed prompt line
	// (placeholder hint text or typed-but-unsubmitted input), rendered with
	// no ╭│╰ box and no rule around it — a third prompt dialect distinct
	// from both of Claude's shapes and the classic boxed prompt
	// promptEmptyRule/typedInputRule still cover for codexRules' synthetic
	// fixtures. bottomPromptGlyphPattern can't match it: that pattern
	// requires nothing after the glyph, but Codex's line always carries
	// placeholder or typed text.
	codexBarePromptPattern = regexp.MustCompile(`(?m)^\s*›.+$`)
)

func codexWorkingStatusLineRule(id string) rule {
	return rule{
		id:    id,
		state: StateWorking,
		match: func(r regions, _ string) (Signal, bool) {
			m := codexWorkingStatusPattern.FindString(r.bottomLines(codexStatusWindowLines))
			if m == "" {
				return Signal{}, false
			}
			return Signal{RuleID: id, Region: "bottomLines", Matched: m}, true
		},
	}
}

func codexBarePromptRule(id string) rule {
	return rule{
		id:    id,
		state: StateIdle,
		match: func(r regions, _ string) (Signal, bool) {
			m := codexBarePromptPattern.FindString(r.bottomLines(codexStatusWindowLines))
			if m == "" {
				return Signal{}, false
			}
			return Signal{RuleID: id, Region: "bottomLines", Matched: strings.TrimSpace(m)}, true
		},
	}
}

// codexTrustDialogRule matches Codex's boot-time directory-trust dialog
// ("Do you trust the contents of this directory? …" + numbered options +
// "Press enter to continue"). Unlike every other dialog this engine parses,
// Codex renders each visual section of this screen — the invoking command
// line, the question, the options, the footer — separated by its own blank
// line, so no single contiguous block contains the whole shape and
// questionRule's promptBoxBody+abovePromptBox scoping (verified against the
// real capture) never sees the question text at all. This rule instead
// anchors on the dialog's own highly specific boot-time vocabulary within
// bottomLines: safe despite the wider-than-contiguous-block scan because
// the phrase exists only until the directory is trusted, after which
// Codex's normal banner, tip, and prompt entirely replace this screen — by
// then comfortably exceeding codexStatusWindowLines before the phrase
// could ever reappear stale.
func codexTrustDialogRule(id string) rule {
	const trustDialogWindowLines = 12
	const anchor = "Do you trust the contents of this directory?"
	return rule{
		id:    id,
		state: StateQuestion,
		match: func(r regions, _ string) (Signal, bool) {
			if !strings.Contains(r.bottomLines(trustDialogWindowLines), anchor) {
				return Signal{}, false
			}
			return Signal{RuleID: id, Region: "bottomLines", Matched: anchor}, true
		},
	}
}

// codexRules mirrors claudeRules' priority ordering (hold, approval,
// question, working, idle); codex has no dedicated model-picker overlay.
//
// Two dialects of the same tier coexist deliberately, oldest-first: the
// box-based promptEmptyRule/typedInputRule (this rule set's original
// synthetic fixtures, modeling a classic boxed-prompt UI) and the bare-UI
// rules added for real Codex v0.146.0 (codexWorkingStatusLineRule,
// codexBarePromptRule, codexTrustDialogRule), which renders no real input
// box at all — detectBorderedPromptBox excludes Codex's persistent welcome
// banner box (see isWelcomeBannerBox) as not being the prompt, so
// hasPromptBox() is false for the real UI and the box-based rules simply
// don't fire, leaving the bare-UI rules to handle it. Neither dialect is
// rewritten or deleted for the other.
var codexRules = ruleSet{
	holdRule("codex/search-prompt", "⌕ Search…"),
	holdRule("codex/transcript-viewer", "ctrl+r to toggle"),

	phraseRule("codex/permission-dialog", StateApproval, codexApprovalPhrases),
	questionRule("codex/question"),
	codexTrustDialogRule("codex/trust-dialog"),

	spinnerShapeRule("codex/spinner-shape"),
	tokenStatsRule("codex/token-stats"),
	codexWorkingStatusLineRule("codex/working-status-line"),

	promptEmptyRule("codex/prompt-empty"),
	typedInputRule("codex/typed-input"),
	bottomPromptGlyphRule("codex/prompt-glyph"),
	codexBarePromptRule("codex/bare-prompt"),
}
