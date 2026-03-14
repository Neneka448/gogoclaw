package memory

// FindConnectedComponents performs BFS on the given edges to find all
// connected components among the provided node IDs.
// Returns a slice of communities, each community being a slice of node IDs.
func FindConnectedComponents(nodeIDs []string, edges []MemoryEdge) [][]string {
	if len(nodeIDs) == 0 {
		return nil
	}

	adjacency := make(map[string][]string, len(nodeIDs))
	for _, id := range nodeIDs {
		adjacency[id] = nil
	}
	for _, edge := range edges {
		if _, ok := adjacency[edge.SourceID]; !ok {
			continue
		}
		if _, ok := adjacency[edge.TargetID]; !ok {
			continue
		}
		adjacency[edge.SourceID] = append(adjacency[edge.SourceID], edge.TargetID)
		adjacency[edge.TargetID] = append(adjacency[edge.TargetID], edge.SourceID)
	}

	visited := make(map[string]bool, len(nodeIDs))
	var communities [][]string

	for _, id := range nodeIDs {
		if visited[id] {
			continue
		}

		component := bfs(id, adjacency, visited)
		if len(component) > 0 {
			communities = append(communities, component)
		}
	}

	return communities
}

func bfs(start string, adjacency map[string][]string, visited map[string]bool) []string {
	queue := []string{start}
	visited[start] = true
	var component []string

	for head := 0; head < len(queue); head++ {
		current := queue[head]
		component = append(component, current)

		for _, neighbor := range adjacency[current] {
			if !visited[neighbor] {
				visited[neighbor] = true
				queue = append(queue, neighbor)
			}
		}
	}

	return component
}

// FindCommunityContaining returns the connected component that contains nodeID,
// or nil if nodeID is not present. An isolated node returns a single-element slice.
func FindCommunityContaining(nodeID string, nodeIDs []string, edges []MemoryEdge) []string {
	communities := FindConnectedComponents(nodeIDs, edges)
	for _, community := range communities {
		for _, id := range community {
			if id == nodeID {
				return community
			}
		}
	}
	return nil
}
