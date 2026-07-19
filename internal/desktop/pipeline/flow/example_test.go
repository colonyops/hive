package flow

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// exampleFlowRefs resolves every reference ExampleFlow() uses — now only the
// action node's actions.yml id.
func exampleFlowRefs() MapRefs {
	return MapRefs{
		Actions: map[string]bool{"review-pr": true},
	}
}

func TestExampleFlow_ParsesAndValidatesClean(t *testing.T) {
	f, warnings, err := parseFlow("example", []byte(ExampleFlow()), exampleFlowRefs())
	require.NoError(t, err)
	assert.Empty(t, warnings)

	assert.Equal(t, "example", f.ID)
	assert.Equal(t, "Frontend Triage", f.Name)
	assert.True(t, f.Enabled)
	require.Len(t, f.Nodes, 5)
	require.Len(t, f.Wires, 4)

	var tag *Node
	for i := range f.Nodes {
		if f.Nodes[i].ID == "tag" {
			tag = &f.Nodes[i]
		}
	}
	require.NotNil(t, tag)
	fc, ok := tag.Config.(*FunctionConfig)
	require.True(t, ok)
	assert.Equal(t, 2, fc.Outputs())
}
