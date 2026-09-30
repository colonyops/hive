package actions

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The decoder mutates a config in place, so a factory that returns a shared
// pointer would have two actions of the same type editing one value. Mirrors
// sources.TestNewConfigReturnsADistinctValue — ranging Types() means an
// action type registered later is covered automatically, with no per-type
// list to remember to update.
//
// registry is unexported, so this lives as an internal test in package
// actions rather than reaching it through an exported constructor — there is
// no public way to build an ActionConfig from a bare type string outside of a
// full YAML action decode.
func TestActionFactoryReturnsADistinctValue(t *testing.T) {
	t.Parallel()

	for _, actionType := range Types() {
		factory, ok := registry[actionType]
		require.Truef(t, ok, "Types() returned %q but the registry does not know it", actionType)

		first, second := factory(), factory()
		assert.NotSamef(t, first, second,
			"action type %q factory returns the same config value twice; the decoder mutates it in place", actionType)
	}
}
