package runtime

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/desktop/internal/app/flow"
)

func node(id, nodeType string) flow.Node {
	return flow.Node{ID: id, Type: nodeType, Config: &flow.FeedConfig{}}
}

func TestNewGraphOrdersTopologically(t *testing.T) {
	t.Parallel()

	g, err := NewGraph(flow.Flow{
		ID:    "f",
		Nodes: []flow.Node{node("c", "feed"), node("a", "feed"), node("b", "feed")},
		Wires: []flow.Wire{{From: "a", To: "b"}, {From: "b", To: "c"}},
	})
	require.NoError(t, err)
	require.Equal(t, []string{"a", "b", "c"}, g.Order())
	require.Equal(t, []string{"a"}, g.Entries())
}

// The order has to be stable, not merely valid: it decides the order of every
// ordered field in a CommitBatch, and a batch that reshuffles between runs is
// impossible to write a fixture for.
func TestNewGraphSeedsEntriesInDeclarationOrder(t *testing.T) {
	t.Parallel()

	f := flow.Flow{
		ID:    "f",
		Nodes: []flow.Node{node("z", "feed"), node("y", "feed"), node("x", "feed")},
	}
	for range 20 {
		g, err := NewGraph(f)
		require.NoError(t, err)
		require.Equal(t, []string{"z", "y", "x"}, g.Order())
	}
}

func TestNewGraphIndexesWiresByPort(t *testing.T) {
	t.Parallel()

	g, err := NewGraph(flow.Flow{
		ID:    "f",
		Nodes: []flow.Node{node("a", "feed"), node("pass", "feed"), node("fail", "feed")},
		Wires: []flow.Wire{{From: "a", Out: 1, To: "fail"}, {From: "a", Out: 0, To: "pass"}},
	})
	require.NoError(t, err)
	require.Equal(t, []flow.Wire{{From: "a", Out: 0, To: "pass"}}, g.Wires("a", 0))
	require.Equal(t, []flow.Wire{{From: "a", Out: 1, To: "fail"}}, g.Wires("a", 1))
	require.Empty(t, g.Wires("a", 2))
}

// A cycle has no execution order at all. Refusing it is the only alternative
// to looping forever, and flow validation reports it properly long before the
// engine is asked to run one.
func TestNewGraphRejectsACycle(t *testing.T) {
	t.Parallel()

	_, err := NewGraph(flow.Flow{
		ID:    "loop",
		Nodes: []flow.Node{node("a", "feed"), node("b", "feed")},
		Wires: []flow.Wire{{From: "a", To: "b"}, {From: "b", To: "a"}},
	})
	require.ErrorContains(t, err, "not a DAG")
}

// A wire to a node that is not in the flow is the editor's problem to report,
// not a reason to take a running flow offline.
func TestNewGraphDropsWiresWithUnknownEndpoints(t *testing.T) {
	t.Parallel()

	g, err := NewGraph(flow.Flow{
		ID:    "f",
		Nodes: []flow.Node{node("a", "feed")},
		Wires: []flow.Wire{{From: "a", To: "ghost"}, {From: "ghost", To: "a"}},
	})
	require.NoError(t, err)
	require.Equal(t, []string{"a"}, g.Order())
	require.Empty(t, g.Wires("a", 0))
}
