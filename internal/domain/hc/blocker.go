package hc

// WouldCycle reports whether adding the edge from → to would introduce a cycle
// in the blocker graph. It performs a DFS from 'to' following existing edges;
// if 'from' is reachable, adding the edge would close a cycle.
func WouldCycle(edges [][2]string, from, to string) bool {
	adj := make(map[string][]string)
	for _, e := range edges {
		adj[e[0]] = append(adj[e[0]], e[1])
	}

	visited := make(map[string]bool)
	stack := []string{to}
	for len(stack) > 0 {
		n := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if n == from {
			return true
		}
		if visited[n] {
			continue
		}
		visited[n] = true
		stack = append(stack, adj[n]...)
	}
	return false
}
