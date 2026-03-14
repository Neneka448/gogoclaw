package tools

import (
	"encoding/json"
	"testing"
)

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
