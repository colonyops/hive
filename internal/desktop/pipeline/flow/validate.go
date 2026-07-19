package flow

import (
	"fmt"
	"strings"
)

// validateFlow checks node ids, per-node config (including cross-file refs),
// and wires, returning soft warnings alongside a nil error on success. The
// first hard-error condition encountered fails the whole flow; warnings are
// only computed once every hard check has passed.
func validateFlow(f *Flow, refs Refs) ([]string, error) {
	nodeByID := make(map[string]*Node, len(f.Nodes))
	for i := range f.Nodes {
		node := &f.Nodes[i]
		if node.ID == "" {
			return nil, fmt.Errorf("node: id is required")
		}
		if !validSlug(node.ID) {
			return nil, fmt.Errorf("node %q: id is not a valid slug (lowercase letters, digits, hyphens, starting with a letter or digit, max %d chars)", node.ID, maxSlugLen)
		}
		if _, dup := nodeByID[node.ID]; dup {
			return nil, fmt.Errorf("node %q: duplicate node id", node.ID)
		}
		nodeByID[node.ID] = node
	}

	for _, node := range f.Nodes {
		if node.Config == nil {
			return nil, fmt.Errorf("node %q: no config decoded", node.ID)
		}
		if err := node.Config.Validate(refs); err != nil {
			return nil, fmt.Errorf("node %q (%s): %w", node.ID, node.Type, err)
		}
	}

	seenWire := make(map[string]bool, len(f.Wires))
	inbound := make(map[string]int, len(f.Nodes))
	adjacency := make(map[string][]string, len(f.Nodes))
	for _, w := range f.Wires {
		from, ok := nodeByID[w.From]
		if !ok {
			return nil, fmt.Errorf("wire %s:%d -> %s: unknown source node %q", w.From, w.Out, w.To, w.From)
		}
		to, ok := nodeByID[w.To]
		if !ok {
			return nil, fmt.Errorf("wire %s:%d -> %s: unknown target node %q", w.From, w.Out, w.To, w.To)
		}
		if from.Config.Outputs() == 0 {
			return nil, fmt.Errorf("wire %s:%d -> %s: node %q (%s) is a terminal and cannot be a wire source", w.From, w.Out, w.To, w.From, from.Type)
		}
		if to.Config.Inputs() == 0 {
			return nil, fmt.Errorf("wire %s:%d -> %s: node %q (%s) is a source and cannot be a wire target", w.From, w.Out, w.To, w.To, to.Type)
		}
		if w.Out < 0 || w.Out >= from.Config.Outputs() {
			return nil, fmt.Errorf("wire %s:%d -> %s: node %q has %d output port(s); out %d is out of range", w.From, w.Out, w.To, w.From, from.Config.Outputs(), w.Out)
		}

		key := wireKey(w)
		if seenWire[key] {
			return nil, fmt.Errorf("wire %s:%d -> %s: duplicate wire", w.From, w.Out, w.To)
		}
		seenWire[key] = true

		inbound[w.To]++
		adjacency[w.From] = append(adjacency[w.From], w.To)
	}

	if cycle, ok := findCycle(f.Nodes, adjacency); ok {
		return nil, fmt.Errorf("flow contains a cycle: %s", strings.Join(cycle, " -> "))
	}

	var warnings []string
	hasTerminal := false
	for _, node := range f.Nodes {
		if node.Disabled {
			warnings = append(warnings, fmt.Sprintf("node %q is disabled", node.ID))
		}
		if node.Config.Outputs() == 0 {
			hasTerminal = true
			if inbound[node.ID] == 0 {
				warnings = append(warnings, fmt.Sprintf("terminal node %q (%s) has no inbound wire", node.ID, node.Type))
			}
		}
	}
	if !hasTerminal {
		warnings = append(warnings, "flow has no terminal node (feed or action)")
	}
	return warnings, nil
}

func wireKey(w Wire) string {
	return w.From + "\x00" + fmt.Sprint(w.Out) + "\x00" + w.To
}

// findCycle runs a DFS over the node graph looking for a back edge (a white/
// gray/black coloring), returning the cyclic path (from the repeated node
// back to itself) when one is found.
func findCycle(nodes []Node, adjacency map[string][]string) ([]string, bool) {
	const (
		white = 0
		gray  = 1
		black = 2
	)
	color := make(map[string]int, len(nodes))
	var path []string
	var cyclePath []string

	var visit func(id string) bool
	visit = func(id string) bool {
		color[id] = gray
		path = append(path, id)
		for _, next := range adjacency[id] {
			switch color[next] {
			case white:
				if visit(next) {
					return true
				}
			case gray:
				idx := indexOf(path, next)
				cyclePath = append(append([]string{}, path[idx:]...), next)
				return true
			}
		}
		path = path[:len(path)-1]
		color[id] = black
		return false
	}

	for _, node := range nodes {
		if color[node.ID] == white {
			if visit(node.ID) {
				return cyclePath, true
			}
		}
	}
	return nil, false
}

func indexOf(s []string, v string) int {
	for i, x := range s {
		if x == v {
			return i
		}
	}
	return -1
}
