package actions

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestActionDocsCoverEveryRegisteredType is the coupling that lets the actions
// prompt be registry-driven: every registered type must document itself, and
// no orphan doc may linger for a type that was removed.
func TestActionDocsCoverEveryRegisteredType(t *testing.T) {
	for _, actionType := range Types() {
		doc, err := Doc(actionType)
		require.NoErrorf(t, err, "action type %q has no docs/%s.md", actionType, actionType)
		assert.NotEmptyf(t, strings.TrimSpace(doc), "docs/%s.md is empty", actionType)
		assert.Truef(t, strings.HasPrefix(strings.TrimSpace(doc), "# "),
			"docs/%s.md must open with an H1 naming the action type", actionType)
	}

	entries, err := docsFS.ReadDir("docs")
	require.NoError(t, err)
	for _, entry := range entries {
		actionType := strings.TrimSuffix(entry.Name(), ".md")
		assert.Containsf(t, registry, actionType,
			"docs/%s documents %q, which is not a registered action type", entry.Name(), actionType)
	}
}

// TestExampleYAMLIsValid keeps the actions prompt's worked example honest: the
// bytes an agent is shown as a model document have to load cleanly.
func TestExampleYAMLIsValid(t *testing.T) {
	parsed, err := parseCatalog([]byte(ExampleYAML()))
	require.NoError(t, err)
	require.NotEmpty(t, parsed.Actions)
	require.NotEmpty(t, parsed.Launchers, "the example does not demonstrate the launchers list")

	// Every registered type should appear, so the example never silently stops
	// demonstrating a type that was added later.
	seen := make(map[string]bool, len(parsed.Actions))
	for _, action := range parsed.Actions {
		seen[action.Type] = true
	}
	for _, actionType := range Types() {
		assert.Containsf(t, seen, actionType, "example actions.yml does not demonstrate type %q", actionType)
	}
}

func TestTypesIsSorted(t *testing.T) {
	types := Types()
	require.Len(t, types, len(registry))
	assert.IsIncreasing(t, types)
}
