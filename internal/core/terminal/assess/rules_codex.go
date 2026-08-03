package assess

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

// codexRules mirrors claudeRules' priority ordering (hold, approval,
// question, working, idle); codex has no dedicated model-picker overlay.
var codexRules = ruleSet{
	holdRule("codex/search-prompt", "⌕ Search…"),
	holdRule("codex/transcript-viewer", "ctrl+r to toggle"),

	phraseRule("codex/permission-dialog", StateApproval, codexApprovalPhrases),
	questionRule("codex/question"),

	spinnerShapeRule("codex/spinner-shape"),
	tokenStatsRule("codex/token-stats"),

	promptEmptyRule("codex/prompt-empty"),
	typedInputRule("codex/typed-input"),
	bottomPromptGlyphRule("codex/prompt-glyph"),
}
