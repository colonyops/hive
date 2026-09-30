package flow

import _ "embed"

// WorkedExampleYAML is the canonical sample flow: a sources.github feeding a
// github-filter, a two-output function node, and both terminal kinds
// (sources.github -> github-filter -> function -> {feed, action}).
//
// It has exactly one home because it has two jobs that must never disagree.
// TestLoadFlow_WorkedExample loads these bytes and asserts they parse and
// validate, and internal/app/prompts embeds them in the flows authoring
// prompt as the concrete example an agent works from. A prompt example that
// no longer parses is a broken prompt, so the loader test is what keeps it
// honest.
//
//go:embed examples/worked-example.yaml
var WorkedExampleYAML string
