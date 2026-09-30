package flow

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The decoder mutates a config in place, so a factory that returns a shared
// pointer would have two nodes of the same type editing one value: save one
// source node and a second one silently changes with it. Mirrors
// sources.TestNewConfigReturnsADistinctValue — ranging NodeTypes() means a
// node type registered later is covered automatically, with no per-type list
// to remember to update.
//
// registry is unexported, so this lives as an internal test in package flow
// rather than reaching it through an exported constructor — there is no
// public way to build a NodeConfig from a bare type string outside of a full
// YAML node decode.
func TestNodeFactoryReturnsADistinctValue(t *testing.T) {
	t.Parallel()

	for _, nodeType := range NodeTypes() {
		factory, ok := registry[nodeType]
		require.Truef(t, ok, "NodeTypes() returned %q but the registry does not know it", nodeType)

		first, second := factory(), factory()
		assert.NotSamef(t, first, second,
			"node type %q factory returns the same config value twice; the decoder mutates it in place", nodeType)
	}
}
