package assess

// genericRules is used for any snap.Tool without a dedicated rule set
// (unclassified tools, "agent", "shell", pi, etc.): spinner shape, a bare
// "(y/n)"-style confirmation, and a bare prompt glyph.
//
// A question rule is included too, beyond the tool-agnostic set the plan
// originally scoped: "a question line followed by a numbered option list" is
// itself a generic UI shape (not tool-specific vocabulary), and without it
// any AskUserQuestion-style UI from a tool with no dedicated rule set (e.g.
// pi) would silently degrade to unknown. Reusing questionRule keeps this a
// one-line addition rather than a parallel implementation.
var genericRules = ruleSet{
	spinnerShapeRule("generic/spinner-shape"),
	genericYesNoRule("generic/yes-no-prompt"),
	questionRule("generic/question"),
	bottomPromptGlyphRule("generic/prompt-glyph"),
}
