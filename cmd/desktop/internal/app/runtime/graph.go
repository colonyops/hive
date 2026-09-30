package runtime

import (
	"fmt"
	"maps"
	"slices"

	"github.com/colonyops/hive/cmd/desktop/internal/app/flow"
)

// Graph is the execution-order index derived once from a flow: which nodes
// are entry points, which wires leave each output port, and the topological
// order Run walks. Building it is the only place a flow's shape is inspected,
// so Run itself never searches the node or wire slices.
//
// Wires with either endpoint unknown are dropped here rather than reported:
// flow.LoadFlow owns reference diagnostics, and an engine that refused to run
// on a dangling wire would take a flow offline for a problem its editor
// already surfaces.
type Graph struct {
	// order is every node id in topological order.
	order []string
	// nodes indexes the flow's nodes by id.
	nodes map[string]*flow.Node
	// out maps a node id to its outbound wires, indexed by output port.
	out map[string]map[int][]flow.Wire
	// entries are the node ids with no inbound wire, in flow declaration
	// order — the nodes a batch's messages are offered to.
	entries []string
}

// NewGraph indexes f for execution. It returns an error only for a cycle:
// a flow that is not a DAG has no execution order at all, and looping forever
// on one is worse than refusing it.
func NewGraph(f flow.Flow) (*Graph, error) {
	known := make(map[string]*flow.Node, len(f.Nodes))
	for i := range f.Nodes {
		known[f.Nodes[i].ID] = &f.Nodes[i]
	}

	degrees := make(map[string]int, len(f.Nodes))
	for id := range known {
		degrees[id] = 0
	}
	out := make(map[string]map[int][]flow.Wire)
	for _, wire := range f.Wires {
		if known[wire.From] == nil || known[wire.To] == nil {
			continue
		}
		degrees[wire.To]++
		byPort := out[wire.From]
		if byPort == nil {
			byPort = make(map[int][]flow.Wire)
			out[wire.From] = byPort
		}
		byPort[wire.Out] = append(byPort[wire.Out], wire)
	}

	g := &Graph{nodes: known, out: out}
	for i := range f.Nodes {
		if degrees[f.Nodes[i].ID] == 0 {
			g.entries = append(g.entries, f.Nodes[i].ID)
		}
	}

	// Kahn's algorithm, seeded in declaration order so the resulting order —
	// and therefore every ordered field of a CommitBatch — is stable across
	// runs of the same flow.
	remaining := make(map[string]int, len(degrees))
	maps.Copy(remaining, degrees)
	queue := append([]string(nil), g.entries...)
	for i := 0; i < len(queue); i++ {
		id := queue[i]
		g.order = append(g.order, id)
		for _, wires := range portsInOrder(out[id]) {
			for _, wire := range wires {
				remaining[wire.To]--
				if remaining[wire.To] == 0 {
					queue = append(queue, wire.To)
				}
			}
		}
	}
	if len(g.order) != len(f.Nodes) {
		return nil, fmt.Errorf("flow %q is not a DAG (cycle detected) — cannot execute", f.ID)
	}
	return g, nil
}

// Order returns the topological execution order.
func (g *Graph) Order() []string { return g.order }

// Entries returns the entry node ids in flow declaration order.
func (g *Graph) Entries() []string { return g.entries }

// Node returns the node with this id, or nil.
func (g *Graph) Node(id string) *flow.Node { return g.nodes[id] }

// Wires returns the wires leaving nodeID on port.
func (g *Graph) Wires(nodeID string, port int) []flow.Wire { return g.out[nodeID][port] }

// portsInOrder returns a node's outbound wire groups ordered by port number,
// so map iteration order never reaches the execution order.
func portsInOrder(byPort map[int][]flow.Wire) [][]flow.Wire {
	if len(byPort) == 0 {
		return nil
	}
	ports := make([]int, 0, len(byPort))
	for port := range byPort {
		ports = append(ports, port)
	}
	slices.Sort(ports)
	groups := make([][]flow.Wire, 0, len(ports))
	for _, port := range ports {
		groups = append(groups, byPort[port])
	}
	return groups
}
