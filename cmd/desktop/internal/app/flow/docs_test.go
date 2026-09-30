package flow

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNodeDocsCoverEveryRegisteredType is the coupling that lets the flows
// prompt be registry-driven: every registered type must document itself, and
// no orphan doc may linger for a type that was removed. Without this a new
// node type would silently render as a gap in the prompt.
func TestNodeDocsCoverEveryRegisteredType(t *testing.T) {
	for _, nodeType := range NodeTypes() {
		doc, err := NodeDoc(nodeType)
		require.NoErrorf(t, err, "node type %q has no docs/%s.md", nodeType, nodeType)
		assert.NotEmptyf(t, strings.TrimSpace(doc), "docs/%s.md is empty", nodeType)
		assert.Truef(t, strings.HasPrefix(strings.TrimSpace(doc), "# "),
			"docs/%s.md must open with an H1 naming the node type", nodeType)
	}

	entries, err := docsFS.ReadDir("docs")
	require.NoError(t, err)
	for _, entry := range entries {
		nodeType := strings.TrimSuffix(entry.Name(), ".md")
		assert.Containsf(t, registry, nodeType,
			"docs/%s documents %q, which is not a registered node type", entry.Name(), nodeType)
	}
}

func TestCategoryOfDerivesFromPortCounts(t *testing.T) {
	for nodeType, want := range map[string]NodeCategory{
		"sources.github":  CategorySources,
		"sources.webhook": CategorySources,
		"github-filter":   CategoryProcess,
		"function":        CategoryProcess,
		"feed":            CategoryDestinations,
		"action":          CategoryDestinations,
		"notify":          CategoryDestinations,
	} {
		got, err := CategoryOf(nodeType)
		require.NoErrorf(t, err, "node type %q", nodeType)
		assert.Equalf(t, want, got, "node type %q", nodeType)
	}

	_, err := CategoryOf("not-a-real-type")
	require.Error(t, err)
}

func TestNodeTypesIsSorted(t *testing.T) {
	types := NodeTypes()
	require.Len(t, types, len(registry))
	assert.IsIncreasing(t, types)
}
