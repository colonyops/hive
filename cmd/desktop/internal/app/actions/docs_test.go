package actions

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/desktop/internal/app/docstest"
)

// TestActionDocsCoverEveryRegisteredType is the coupling that lets the actions
// prompt be registry-driven: every registered type must document itself, and
// no orphan doc may linger for a type that was removed.
func TestActionDocsCoverEveryRegisteredType(t *testing.T) {
	t.Parallel()
	docstest.AssertCoverTypes(t, "action", Types(), Doc, docsFS)
}

// TestExampleYAMLIsValid keeps the actions prompt's worked example honest: the
// bytes an agent is shown as a model document have to load cleanly.
func TestExampleYAMLIsValid(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
	types := Types()
	require.Len(t, types, len(registry))
	assert.IsIncreasing(t, types)
}
