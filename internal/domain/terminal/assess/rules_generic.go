package assess

// genericRules is used for any snap.Tool without a dedicated rule set
// (unclassified tools, "agent", "shell", pi, etc.): spinner shape, a bare
// "(y/n)"-style confirmation, a question, an empty/typed rule-delimited or
// ╭│╰ prompt box, and a bare prompt glyph.
//
// A question rule is included too, beyond the tool-agnostic set the plan
// originally scoped: "a question line followed by a numbered option list" is
// itself a generic UI shape (not tool-specific vocabulary), and without it
// any AskUserQuestion-style UI from a tool with no dedicated rule set (e.g.
// pi) would silently degrade to unknown. Reusing questionRule keeps this a
// one-line addition rather than a parallel implementation.
//
// promptEmptyRule/typedInputRule are included for the same reason: pi's
// idle input area is a rule-delimited box with a blank (glyph-less) middle
// line, detected by detectRulePromptBox exactly like Claude Code's bare ❯
// line — without these, hasPromptBox() being true but never checked would
// leave that box's idle state undetected, and the tracker would hold the
// previous published status (observed live: stuck at "active" forever).
// They're placed after questionRule/spinnerShapeRule (idle is checked only
// once nothing more specific matched) and before bottomPromptGlyphRule, the
// box-less bare-prompt fallback for tools with no box at all.
var genericRules = ruleSet{
	spinnerShapeRule("generic/spinner-shape"),
	genericYesNoRule("generic/yes-no-prompt"),
	questionRule("generic/question"),

	promptEmptyRule("generic/prompt-empty"),
	typedInputRule("generic/typed-input"),
	bottomPromptGlyphRule("generic/prompt-glyph"),
}
