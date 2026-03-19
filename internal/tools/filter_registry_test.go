package tools

import (
	"testing"

	openai "github.com/sashabaranov/go-openai"
)

func newTestToolRegistry(names ...string) ToolRegistry {
	registry := NewToolRegistry()
	for _, name := range names {
		_ = registry.RegisterTool(name, ToolDescriptor{
			Name: name,
			Tool: nil,
			ToolForLLM: openai.Tool{
				Type:     openai.ToolTypeFunction,
				Function: &openai.FunctionDefinition{Name: name},
			},
		})
	}
	return registry
}

func toolNames(descriptors []ToolDescriptor) map[string]struct{} {
	m := make(map[string]struct{}, len(descriptors))
	for _, d := range descriptors {
		m[d.Name] = struct{}{}
	}
	return m
}

func TestFilteredRegistryAllowsAllByDefault(t *testing.T) {
	inner := newTestToolRegistry("read_file", "terminal", "message")
	filtered := NewFilteredRegistry(inner, nil, nil)

	all := filtered.GetAllTools()
	if len(all) != 3 {
		t.Fatalf("len(GetAllTools()) = %d, want 3", len(all))
	}
}

func TestFilteredRegistryForbiddenRemovesTools(t *testing.T) {
	inner := newTestToolRegistry("read_file", "terminal", "message")
	filtered := NewFilteredRegistry(inner, nil, []string{"message"})

	all := filtered.GetAllTools()
	names := toolNames(all)
	if _, ok := names["message"]; ok {
		t.Fatal("message should be forbidden")
	}
	if len(all) != 2 {
		t.Fatalf("len(GetAllTools()) = %d, want 2", len(all))
	}

	if _, err := filtered.GetTool("message"); err == nil {
		t.Fatal("GetTool(message) should return error for forbidden tool")
	}
	if _, err := filtered.GetTool("read_file"); err != nil {
		t.Fatalf("GetTool(read_file) error = %v", err)
	}
}

func TestFilteredRegistryAllowedRestrictsToSubset(t *testing.T) {
	inner := newTestToolRegistry("read_file", "terminal", "message")
	filtered := NewFilteredRegistry(inner, []string{"read_file", "terminal"}, nil)

	all := filtered.GetAllTools()
	names := toolNames(all)
	if _, ok := names["message"]; ok {
		t.Fatal("message should not be in allowed set")
	}
	if len(all) != 2 {
		t.Fatalf("len(GetAllTools()) = %d, want 2", len(all))
	}
}

func TestFilteredRegistryForbiddenOverridesAllowed(t *testing.T) {
	inner := newTestToolRegistry("read_file", "terminal", "message")
	filtered := NewFilteredRegistry(inner, []string{"read_file", "message"}, []string{"message"})

	all := filtered.GetAllTools()
	names := toolNames(all)
	if _, ok := names["message"]; ok {
		t.Fatal("message should be forbidden even if in allowed")
	}
	if len(all) != 1 {
		t.Fatalf("len(GetAllTools()) = %d, want 1", len(all))
	}
}

func TestFilteredRegistryWildcardForbiddenBlocksAll(t *testing.T) {
	inner := newTestToolRegistry("read_file", "terminal", "message")
	filtered := NewFilteredRegistry(inner, []string{"*"}, []string{"*"})

	all := filtered.GetAllTools()
	if len(all) != 0 {
		t.Fatalf("len(GetAllTools()) = %d, want 0", len(all))
	}
}

func TestFilteredRegistryWildcardAllowedEqualsAll(t *testing.T) {
	inner := newTestToolRegistry("read_file", "terminal", "message")
	filtered := NewFilteredRegistry(inner, []string{"*"}, nil)

	all := filtered.GetAllTools()
	if len(all) != 3 {
		t.Fatalf("len(GetAllTools()) = %d, want 3", len(all))
	}
}

func TestFilteredRegistryRegisterToolDelegatesToInner(t *testing.T) {
	inner := newTestToolRegistry("read_file")
	filtered := NewFilteredRegistry(inner, nil, nil)

	err := filtered.RegisterTool("new_tool", ToolDescriptor{Name: "new_tool"})
	if err != nil {
		t.Fatalf("RegisterTool() error = %v", err)
	}

	// Inner should have the new tool
	if _, err := inner.GetTool("new_tool"); err != nil {
		t.Fatalf("inner.GetTool(new_tool) error = %v", err)
	}
}
