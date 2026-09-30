package actions

import (
	"embed"
	"fmt"
	"sort"
)

// docsFS holds one markdown file per registered action type, named
// docs/<type>.md. internal/app/prompts renders these into the actions.yml
// authoring prompt, so an action type documents itself next to the config
// struct and validation rules it describes.
//
// Adding an action type means adding its registry entry and its
// docs/<type>.md; TestActionDocsCoverEveryRegisteredType fails if either is
// missing.
//
//go:embed docs/*.md
var docsFS embed.FS

// exampleFS holds the canonical actions.yml sample. See ExampleYAML.
//
//go:embed examples/actions.yml
var exampleFS embed.FS

// launcherDoc documents the `launchers:` list. It sits outside docs/ because
// that directory is strictly one file per registered action type — a bijection
// TestActionDocsCoverEveryRegisteredType enforces — and a launcher is not an
// action type. The actions prompt renders it as a section of its own.
//
//go:embed launchers.md
var launcherDoc string

// LauncherDoc returns the markdown documenting the launchers list.
func LauncherDoc() string { return launcherDoc }

// ExampleYAML is the canonical actions.yml document: one entry per registered
// action type, exercising the fields each one cares about.
//
// It has exactly one home because it has two jobs that must never disagree.
// TestExampleYAMLIsValid parses and validates these bytes, and
// internal/app/prompts embeds them in the actions authoring prompt as the
// concrete example an agent works from — so a prompt example that no longer
// parses fails the build rather than misleading an agent.
func ExampleYAML() string {
	data, err := exampleFS.ReadFile("examples/actions.yml")
	if err != nil {
		// Unreachable: the file is embedded at compile time.
		panic(fmt.Sprintf("actions: reading embedded example: %v", err))
	}
	return string(data)
}

// Types returns every registered action type in sorted order. Sorting keeps
// prompt output byte-stable across runs — Go map iteration is randomized.
func Types() []string {
	types := make([]string, 0, len(registry))
	for actionType := range registry {
		types = append(types, actionType)
	}
	sort.Strings(types)
	return types
}

// Doc returns the markdown documentation for a registered action type.
func Doc(actionType string) (string, error) {
	data, err := docsFS.ReadFile("docs/" + actionType + ".md")
	if err != nil {
		return "", fmt.Errorf("actions: no documentation for action type %q", actionType)
	}
	return string(data), nil
}
