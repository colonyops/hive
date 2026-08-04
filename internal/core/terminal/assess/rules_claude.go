package assess

// claudeApprovalPhrases are Claude Code's permission-dialog vocabulary.
// Scoped to phraseRule's anti-staleness regions, so these strings appearing
// in scrollback history (e.g. a dismissed dialog) do not re-fire the rule.
var claudeApprovalPhrases = []string{
	"Do you want to",
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
