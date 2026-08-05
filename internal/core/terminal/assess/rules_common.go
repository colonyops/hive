package assess

import (
	"regexp"
	"strings"
)

// approvalWindowLines bounds how far back approval/question rules look in
// bottomLines. Kept small deliberately: a dialog is always near the current
// cursor, and a small window plus abovePromptBox's contiguous-block
// restriction is what keeps a dismissed dialog's leftover scrollback text
// from re-triggering the rule (the corpus-documented failure class).
const approvalWindowLines = 6

// liveDialogWindowLines bounds how far back liveDialogQuestionRule looks in
// bottomLines. Wider than approvalWindowLines: the modern (no ╭│╰ box)
// AskUserQuestion dialog renders a header chip, question line, several
// numbered options with description lines, a second rule, and an extra
// option below it before the footer — comfortably under 20 lines, but well
// past a dialog rendered inside a box.
const liveDialogWindowLines = 20

var (
	spinnerShapePattern = regexp.MustCompile(buildSpinnerShapePattern())

	// tokenStatsPattern matches Claude/Codex's running-total status line,
	// e.g. "(35s · 1234 tokens)" or "(12s · ↑ 673 tokens · ctrl+c to interrupt)".
	tokenStatsPattern = regexp.MustCompile(`\(\s*\d+(?:\.\d+)?(?:ms|s|m|h)(?:\s+\d+(?:\.\d+)?(?:ms|s|m|h))*\s*·\s*(?:[↑↓]\s*)?\d[\d,]*(?:\.\d+)?[kKmM]?\s+tokens(?:\s*·\s*(?:ctrl\+c|esc)\s+to interrupt)?\s*\)`)

	// questionOptionPattern matches a numbered/lettered option list line,
	// the shape AskUserQuestion-style UIs render answer choices with. Both
	// selector glyphs observed in the wild are optional and interchangeable
	// here: Claude Code uses ❯, Codex uses › (e.g. its boot-time directory-
	// trust dialog's "› 1. Yes, continue").
	questionOptionPattern = regexp.MustCompile(`(?m)^\s*[❯›]?\s*\d+[.)]\s+\S`)

	// genericYesNoPattern matches bare "(y/n)" style confirmation prompts
	// that are not tool-specific dialog vocabulary.
	genericYesNoPattern = regexp.MustCompile(`[([](?:[Yy]/[Nn]|[Yy]es/[Nn]o)[)\]]`)

	// bottomPromptGlyphPattern matches a line that is *only* a bare prompt
	// glyph — the shape a box-less idle prompt takes (e.g. plain shells, or
	// older/degraded tool UIs without a rendered input box).
	bottomPromptGlyphPattern = regexp.MustCompile(`(?m)^\s*[❯>›]\s*$`)
)

// buildSpinnerShapePattern turns SpinnerGlyphs into "any spinner glyph +
// gerund + ellipsis" — e.g. "✳ Gusting… (35s · ↑ 673 tokens)" or pi's
// " ⠧ Working... " (ASCII three-dot ellipsis, indented). It matches the
// shape of the status line, not specific vocabulary, so it survives new
// word choices across tool releases. Leading whitespace before the glyph
// and "…" vs "..." are both tolerated for the same reason: they're
// rendering-dialect differences between tools (pi renders bare-terminal
// ASCII dots and left-pads its status line; Claude/Codex use the Unicode
// ellipsis flush left), not the shape the rule is actually keying on.
func buildSpinnerShapePattern() string {
	var glyphs strings.Builder
	for _, r := range SpinnerGlyphs {
		glyphs.WriteRune(r)
	}
	return `(?m)^\s*[` + glyphs.String() + `] \S+ing.*(?:…|\.{3})`
}

// holdRule builds a transient-screen rule: if marker appears anywhere in the
// viewport, the engine reports StateUnknown with Hold true so Stage 2 keeps
// whatever status was last published instead of misreading a search
// overlay, transcript viewer, or picker as a real state change.
func holdRule(id, marker string) rule {
	return rule{
		id:    id,
		state: StateUnknown,
		hold:  true,
		match: func(r regions) (Signal, bool) {
			if !strings.Contains(r.viewport(), marker) {
				return Signal{}, false
			}
			return Signal{RuleID: id, Region: "viewport", Matched: marker}, true
		},
	}
}

// phraseRule matches tool-specific approval/permission vocabulary against
// the prompt box body plus the immediately-preceding contiguous block, never
// the full scrollback-tainted viewport — a dismissed dialog's leftover text
// sits above a blank-line boundary and is excluded by abovePromptBox.
func phraseRule(id string, state State, phrases []string) rule {
	return rule{
		id:    id,
		state: state,
		match: func(r regions) (Signal, bool) {
			text := r.promptBoxBody() + "\n" + r.abovePromptBox()
			for _, phrase := range phrases {
				if strings.Contains(text, phrase) {
					return Signal{RuleID: id, Region: "promptBoxBody+abovePromptBox", Matched: phrase}, true
				}
			}
			return Signal{}, false
		},
	}
}

// questionRule matches AskUserQuestion-style UIs: a question line plus a
// selectable option list, scoped to the same anti-staleness regions as
// phraseRule. Rule-set ordering (question checked after the tool's approval
// rule) is what keeps a real permission dialog from also matching here —
// approval vocabulary returns first.
func questionRule(id string) rule {
	return rule{
		id:    id,
		state: StateQuestion,
		match: func(r regions) (Signal, bool) {
			text := r.promptBoxBody() + "\n" + r.abovePromptBox()
			if !strings.Contains(text, "?") {
				return Signal{}, false
			}
			m := questionOptionPattern.FindString(text)
			if m == "" {
				return Signal{}, false
			}
			return Signal{RuleID: id, Region: "promptBoxBody+abovePromptBox", Matched: m}, true
		},
	}
}

// liveDialogQuestionRule matches the modern AskUserQuestion dialog: Claude
// Code renders it as bare text between two horizontal rules rather than
// inside a ╭│╰ box, so questionRule's promptBoxBody+abovePromptBox scoping
// can't see it — with no box, abovePromptBox falls back to the last
// contiguous block, which is only the trailing footer chrome, well below
// the blank-line boundary that separates it from the question/options
// block.
//
// This rule instead scans bottomLines (not contiguous-block-restricted) for
// three signals together: the dialog's footer chrome ("Enter to select" or
// "↑/↓ to navigate"), a "?", and a numbered option line. The footer chrome
// is what keeps this anti-stale despite the wider window: Claude Code
// replaces the entire dialog with a "User answered Claude's questions:"
// summary line once answered, so the chrome cannot linger in scrollback the
// way phrase/spinner text can — unlike approvalWindowLines-scoped rules,
// this doesn't need the contiguous-block restriction to stay safe.
func liveDialogQuestionRule(id string) rule {
	return rule{
		id:    id,
		state: StateQuestion,
		match: func(r regions) (Signal, bool) {
			window := r.bottomLines(liveDialogWindowLines)
			if !strings.Contains(window, "Enter to select") && !strings.Contains(window, "to navigate") {
				return Signal{}, false
			}
			if !strings.Contains(window, "?") {
				return Signal{}, false
			}
			m := questionOptionPattern.FindString(window)
			if m == "" {
				return Signal{}, false
			}
			return Signal{RuleID: id, Region: "bottomLines", Matched: m}, true
		},
	}
}

func spinnerShapeRule(id string) rule {
	return rule{
		id:    id,
		state: StateWorking,
		match: func(r regions) (Signal, bool) {
			m := spinnerShapePattern.FindString(r.abovePromptBox())
			if m == "" {
				return Signal{}, false
			}
			return Signal{RuleID: id, Region: "abovePromptBox", Matched: m}, true
		},
	}
}

func tokenStatsRule(id string) rule {
	return rule{
		id:    id,
		state: StateWorking,
		match: func(r regions) (Signal, bool) {
			m := tokenStatsPattern.FindString(r.abovePromptBox())
			if m == "" {
				return Signal{}, false
			}
			return Signal{RuleID: id, Region: "abovePromptBox", Matched: m}, true
		},
	}
}

// promptEmptyRule reports idle when a prompt box exists and is empty or
// shows only the bare prompt marker with no typed text. It must not fire
// when there is no box at all (hasPromptBox guards that) — otherwise a tool
// with no box and no other match would misreport idle instead of unknown.
func promptEmptyRule(id string) rule {
	return rule{
		id:    id,
		state: StateIdle,
		match: func(r regions) (Signal, bool) {
			if !r.hasPromptBox() {
				return Signal{}, false
			}
			if !isPlaceholderPromptBody(strings.TrimSpace(r.promptBoxBody())) {
				return Signal{}, false
			}
			return Signal{RuleID: id, Region: "promptBoxBody", Matched: "empty prompt box"}, true
		},
	}
}

// typedInputRule reports idle when the prompt box holds real typed-but-
// unsubmitted text. Ordered after every approval/question/working rule so
// none of those can fire on arbitrary typed content — this rule only ever
// gets a chance once nothing more specific matched.
func typedInputRule(id string) rule {
	return rule{
		id:    id,
		state: StateIdle,
		match: func(r regions) (Signal, bool) {
			if !r.hasPromptBox() {
				return Signal{}, false
			}
			body := strings.TrimSpace(r.promptBoxBody())
			if isPlaceholderPromptBody(body) {
				return Signal{}, false
			}
			return Signal{RuleID: id, Region: "promptBoxBody", Matched: body}, true
		},
	}
}

func isPlaceholderPromptBody(body string) bool {
	if body == "" {
		return true
	}
	trimmed := strings.TrimSpace(strings.TrimPrefix(body, ">"))
	return trimmed == ""
}

// bottomPromptGlyphRule reports idle on a bare prompt glyph with nothing
// else on its line — the box-less fallback for tools/screens that render a
// plain prompt instead of a bordered input box.
func bottomPromptGlyphRule(id string) rule {
	return rule{
		id:    id,
		state: StateIdle,
		match: func(r regions) (Signal, bool) {
			m := bottomPromptGlyphPattern.FindString(r.bottomLines(approvalWindowLines))
			if m == "" {
				return Signal{}, false
			}
			return Signal{RuleID: id, Region: "bottomLines", Matched: strings.TrimSpace(m)}, true
		},
	}
}

func genericYesNoRule(id string) rule {
	return rule{
		id:    id,
		state: StateApproval,
		match: func(r regions) (Signal, bool) {
			m := genericYesNoPattern.FindString(r.bottomLines(approvalWindowLines))
			if m == "" {
				return Signal{}, false
			}
			return Signal{RuleID: id, Region: "bottomLines", Matched: m}, true
		},
	}
}
