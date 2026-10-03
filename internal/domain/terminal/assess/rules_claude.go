package assess

// claudeApprovalPhrases are permission-specific Claude Code dialog markers.
// Generic question wording is deliberately excluded because selectable
// questions can also begin with "Do you want to".
var claudeApprovalPhrases = []string{
	"Needs your permission",
	"Would you like to run",
	"Yes, allow once",
	"Yes, allow always",
	"No, and tell Claude what to do differently",
}

// claudeRules is priority-ordered: hold rules for transient overlays first
// (they must win regardless of what is underneath), then approval/question
// (the most actionable states), then working, then idle. See rules_common.go
// for what each rule builder checks.
var claudeRules = ruleSet{
	holdRule("claude/search-prompt", "⌕ Search…"),
	holdRule("claude/transcript-viewer", "ctrl+r to toggle"),
	holdRule("claude/model-picker", "Select model"),

	phraseRule("claude/permission-dialog", StateApproval, claudeApprovalPhrases),
	questionRule("claude/question"),
	liveDialogQuestionRule("claude/question-live-dialog"),

	spinnerShapeRule("claude/spinner-shape"),
	tokenStatsRule("claude/token-stats"),

	promptEmptyRule("claude/prompt-empty"),
	typedInputRule("claude/typed-input"),
	bottomPromptGlyphRule("claude/prompt-glyph"),
}
