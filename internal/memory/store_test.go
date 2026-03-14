package memory

import (
	"database/sql"
	"testing"

	_ "github.com/mattn/go-sqlite3"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	store := NewStore(db)
	if err := store.Initialize(); err != nil {
		t.Fatalf("initialize store: %v", err)
	}
	return store
}

func TestStoreInsertAndGetNode(t *testing.T) {
	store := newTestStore(t)

	node := MemoryNode{
		ID:     "test-1",
		Kind:   NodeKindShortTerm,
		Status: NodeStatusActive,
		Level:  0,
		Who:    "user",
		What:   "test task",
	}
	if err := store.InsertNode(node); err != nil {
		t.Fatalf("insert node: %v", err)
	}

	got, err := store.GetNode("test-1")
	if err != nil {
		t.Fatalf("get node: %v", err)
	}
	if got == nil {
		t.Fatal("expected node, got nil")
	}
	if got.Who != "user" {
		t.Fatalf("expected who=user, got %s", got.Who)
	}
	if got.Kind != NodeKindShortTerm {
		t.Fatalf("expected kind=short_term, got %s", got.Kind)
	}
}

func TestStoreUpdateNodeStatus(t *testing.T) {
	store := newTestStore(t)

	node := MemoryNode{
		ID:     "test-2",
		Kind:   NodeKindShortTerm,
		Status: NodeStatusActive,
		Level:  0,
	}
	if err := store.InsertNode(node); err != nil {
		t.Fatalf("insert node: %v", err)
	}

	if err := store.UpdateNodeStatus("test-2", NodeStatusConsolidated); err != nil {
		t.Fatalf("update status: %v", err)
	}

	got, err := store.GetNode("test-2")
	if err != nil {
		t.Fatalf("get node: %v", err)
	}
	if got.Status != NodeStatusConsolidated {
		t.Fatalf("expected consolidated, got %s", got.Status)
	}
}

func TestStoreIncrementRefCount(t *testing.T) {
	store := newTestStore(t)

	node := MemoryNode{
		ID:     "test-3",
		Kind:   NodeKindShortTerm,
		Status: NodeStatusActive,
		Level:  0,
	}
	if err := store.InsertNode(node); err != nil {
		t.Fatalf("insert node: %v", err)
	}

	if err := store.IncrementRefCount("test-3"); err != nil {
		t.Fatalf("increment ref: %v", err)
	}
	if err := store.IncrementRefCount("test-3"); err != nil {
		t.Fatalf("increment ref again: %v", err)
	}

	got, err := store.GetNode("test-3")
	if err != nil {
		t.Fatalf("get node: %v", err)
	}
	if got.RefCount != 2 {
		t.Fatalf("expected ref_count=2, got %d", got.RefCount)
	}
}

func TestStoreEdgesRoundtrip(t *testing.T) {
	store := newTestStore(t)

	for _, id := range []string{"a", "b", "c"} {
		if err := store.InsertNode(MemoryNode{
			ID: id, Kind: NodeKindShortTerm, Status: NodeStatusActive,
		}); err != nil {
			t.Fatalf("insert node %s: %v", id, err)
		}
	}

	if err := store.InsertEdge(MemoryEdge{SourceID: "a", TargetID: "b", Weight: 0.9}); err != nil {
		t.Fatalf("insert edge: %v", err)
	}
	if err := store.InsertEdge(MemoryEdge{SourceID: "b", TargetID: "c", Weight: 0.8}); err != nil {
		t.Fatalf("insert edge: %v", err)
	}

	edges, err := store.ListEdgesForNodes([]string{"a", "b", "c"})
	if err != nil {
		t.Fatalf("list edges: %v", err)
	}
	if len(edges) != 2 {
		t.Fatalf("expected 2 edges, got %d", len(edges))
	}
}

func TestStoreListActiveNodesByKindAndLevel(t *testing.T) {
	store := newTestStore(t)

	if err := store.InsertNode(MemoryNode{
		ID: "st-1", Kind: NodeKindShortTerm, Status: NodeStatusActive, Level: 0,
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.InsertNode(MemoryNode{
		ID: "st-2", Kind: NodeKindShortTerm, Status: NodeStatusConsolidated, Level: 0,
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.InsertNode(MemoryNode{
		ID: "lt-1", Kind: NodeKindLongTerm, Status: NodeStatusActive, Level: 1,
	}); err != nil {
		t.Fatal(err)
	}

	nodes, err := store.ListActiveNodesByKindAndLevel(NodeKindShortTerm, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes) != 1 {
		t.Fatalf("expected 1 active short-term node, got %d", len(nodes))
	}
	if nodes[0].ID != "st-1" {
		t.Fatalf("expected st-1, got %s", nodes[0].ID)
	}
}

func TestStoreInsertEdgeIgnoresDuplicate(t *testing.T) {
	store := newTestStore(t)

	if err := store.InsertNode(MemoryNode{ID: "a", Kind: NodeKindShortTerm, Status: NodeStatusActive}); err != nil {
		t.Fatal(err)
	}
	if err := store.InsertNode(MemoryNode{ID: "b", Kind: NodeKindShortTerm, Status: NodeStatusActive}); err != nil {
		t.Fatal(err)
	}

	if err := store.InsertEdge(MemoryEdge{SourceID: "a", TargetID: "b", Weight: 0.9}); err != nil {
		t.Fatal(err)
	}
	// Should not error on duplicate
	if err := store.InsertEdge(MemoryEdge{SourceID: "a", TargetID: "b", Weight: 0.95}); err != nil {
		t.Fatal(err)
	}
}

func TestStoreGetNodesByIDsPreservesRequestedOrder(t *testing.T) {
	store := newTestStore(t)

	for _, id := range []string{"a", "b", "c"} {
		if err := store.InsertNode(MemoryNode{ID: id, Kind: NodeKindShortTerm, Status: NodeStatusActive}); err != nil {
			t.Fatalf("insert node %s: %v", id, err)
		}
	}

	nodes, err := store.GetNodesByIDs([]string{"c", "a", "b"})
	if err != nil {
		t.Fatalf("GetNodesByIDs() error = %v", err)
	}
	if len(nodes) != 3 {
		t.Fatalf("len(nodes) = %d, want 3", len(nodes))
	}
	if nodes[0].ID != "c" || nodes[1].ID != "a" || nodes[2].ID != "b" {
		t.Fatalf("node order = [%s %s %s], want [c a b]", nodes[0].ID, nodes[1].ID, nodes[2].ID)
	}
}
