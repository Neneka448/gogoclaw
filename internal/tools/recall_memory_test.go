package tools

import (
	"encoding/json"
	"testing"

	"github.com/Neneka448/gogoclaw/internal/memory"
	openai "github.com/sashabaranov/go-openai"
)

type fakeRecallMemoryService struct {
	query         string
	topK          int
	minSimilarity float64
	nodes         []memory.MemoryNode
}

func (service *fakeRecallMemoryService) Initialize() error {
	return nil
}

func (service *fakeRecallMemoryService) IngestSession(sessionID string, messages []openai.ChatCompletionMessage) error {
	return nil
}

func (service *fakeRecallMemoryService) Recall(queryText string, topK int, minSimilarity float64) ([]memory.MemoryNode, error) {
	service.query = queryText
	service.topK = topK
	service.minSimilarity = minSimilarity
	return service.nodes, nil
}

func (service *fakeRecallMemoryService) GetNode(nodeID string) (*memory.MemoryNode, error) {
	return nil, nil
}

func TestRecallMemoryToolNilService(t *testing.T) {
	descriptor := NewRecallMemoryTool(nil)
	tool := descriptor.Tool

	result, err := tool.Execute(`{"query": "test"}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var parsed recallMemoryResult
	if err := json.Unmarshal([]byte(result), &parsed); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if parsed.Error == "" {
		t.Fatal("expected error message for nil service")
	}
}

func TestRecallMemoryToolEmptyQuery(t *testing.T) {
	descriptor := NewRecallMemoryTool(nil)
	tool := descriptor.Tool

	result, err := tool.Execute(`{"query": ""}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var parsed recallMemoryResult
	if err := json.Unmarshal([]byte(result), &parsed); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if parsed.Error == "" {
		t.Fatal("expected error for empty query")
	}
}

func TestRecallMemoryToolInvalidJSON(t *testing.T) {
	descriptor := NewRecallMemoryTool(nil)
	tool := descriptor.Tool

	result, err := tool.Execute(`{invalid`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var parsed recallMemoryResult
	if err := json.Unmarshal([]byte(result), &parsed); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if parsed.Error == "" {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestRecallMemoryToolDescriptor(t *testing.T) {
	descriptor := NewRecallMemoryTool(nil)
	if descriptor.Name != "recall_memory" {
		t.Fatalf("expected name 'recall_memory', got '%s'", descriptor.Name)
	}
	if descriptor.ToolForLLM.Function == nil {
		t.Fatal("expected function definition")
	}
	if descriptor.ToolForLLM.Function.Name != "recall_memory" {
		t.Fatalf("expected function name 'recall_memory', got '%s'", descriptor.ToolForLLM.Function.Name)
	}
}

func TestRecallMemoryToolUsesConfiguredThresholdWhenMinSimilarityOmitted(t *testing.T) {
	service := &fakeRecallMemoryService{}
	descriptor := NewRecallMemoryTool(service)

	if _, err := descriptor.Tool.Execute(`{"query":"test memory"}`); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if service.query != "test memory" {
		t.Fatalf("service.query = %q, want test memory", service.query)
	}
	if service.minSimilarity != -1 {
		t.Fatalf("service.minSimilarity = %v, want -1", service.minSimilarity)
	}
}

func TestRecallMemoryToolPreservesExplicitZeroMinSimilarity(t *testing.T) {
	service := &fakeRecallMemoryService{}
	descriptor := NewRecallMemoryTool(service)

	if _, err := descriptor.Tool.Execute(`{"query":"test memory","min_similarity":0}`); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if service.minSimilarity != 0 {
		t.Fatalf("service.minSimilarity = %v, want 0", service.minSimilarity)
	}
}
