package flow

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/desktop/internal/app/docstest"
)

// TestNodeDocsCoverEveryRegisteredType is the coupling that lets the flows
// prompt be registry-driven: every registered type must document itself, and
// no orphan doc may linger for a type that was removed. Without this a new
// node type would silently render as a gap in the prompt.
func TestNodeDocsCoverEveryRegisteredType(t *testing.T) {
	docstest.AssertCoverTypes(t, "node", NodeTypes(), NodeDoc, docsFS)
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
		"launch-session":  CategoryDestinations,
		"launch-chat":     CategoryDestinations,
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
