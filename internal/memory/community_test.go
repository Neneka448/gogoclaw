package memory

import (
	"testing"
)

func TestFindConnectedComponentsSingleNode(t *testing.T) {
	nodeIDs := []string{"a"}
	edges := []MemoryEdge{}

	communities := FindConnectedComponents(nodeIDs, edges)

	if len(communities) != 1 {
		t.Fatalf("expected 1 community, got %d", len(communities))
	}
	if communities[0][0] != "a" {
		t.Fatalf("expected community containing 'a', got %v", communities[0])
	}
}

func TestFindConnectedComponentsTwoCommunities(t *testing.T) {
	nodeIDs := []string{"a", "b", "c", "d"}
	edges := []MemoryEdge{
		{SourceID: "a", TargetID: "b", Weight: 0.9},
		{SourceID: "c", TargetID: "d", Weight: 0.8},
	}

	communities := FindConnectedComponents(nodeIDs, edges)

	if len(communities) != 2 {
		t.Fatalf("expected 2 communities, got %d", len(communities))
	}

	sizes := map[int]int{}
	for _, c := range communities {
		sizes[len(c)]++
	}
	if sizes[2] != 2 {
		t.Fatalf("expected 2 communities of size 2, got sizes: %v", sizes)
	}
}

func TestFindConnectedComponentsSingleCommunity(t *testing.T) {
	nodeIDs := []string{"a", "b", "c"}
	edges := []MemoryEdge{
		{SourceID: "a", TargetID: "b", Weight: 0.9},
		{SourceID: "b", TargetID: "c", Weight: 0.8},
	}

	communities := FindConnectedComponents(nodeIDs, edges)

	if len(communities) != 1 {
		t.Fatalf("expected 1 community, got %d", len(communities))
	}
	if len(communities[0]) != 3 {
		t.Fatalf("expected community size 3, got %d", len(communities[0]))
	}
}

func TestFindConnectedComponentsIgnoresExternalEdges(t *testing.T) {
	nodeIDs := []string{"a", "b"}
	edges := []MemoryEdge{
		{SourceID: "a", TargetID: "x", Weight: 0.9},
	}

	communities := FindConnectedComponents(nodeIDs, edges)

	if len(communities) != 2 {
		t.Fatalf("expected 2 isolated communities, got %d", len(communities))
	}
}

func TestFindConnectedComponentsEmpty(t *testing.T) {
	communities := FindConnectedComponents(nil, nil)
	if communities != nil {
		t.Fatalf("expected nil, got %v", communities)
	}
}

func TestFindCommunityContaining(t *testing.T) {
	nodeIDs := []string{"a", "b", "c", "d"}
	edges := []MemoryEdge{
		{SourceID: "a", TargetID: "b", Weight: 0.9},
		{SourceID: "c", TargetID: "d", Weight: 0.8},
	}

	community := FindCommunityContaining("a", nodeIDs, edges)
	if len(community) != 2 {
		t.Fatalf("expected community size 2 for 'a', got %d", len(community))
	}

	found := false
	for _, id := range community {
		if id == "b" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected 'b' in community containing 'a', got %v", community)
	}
}

func TestFindCommunityContainingNotPresent(t *testing.T) {
	nodeIDs := []string{"a", "b"}
	edges := []MemoryEdge{}

	community := FindCommunityContaining("x", nodeIDs, edges)
	if community != nil {
		t.Fatalf("expected nil for missing node, got %v", community)
	}
}
