package tools

import (
	"fmt"
	"sync"

	Openai "github.com/sashabaranov/go-openai"
)

type Tool interface {
	Execute(args string) (string, error)
}

type ToolDescriptor struct {
	Name       string
	Tool       Tool
	ToolForLLM Openai.Tool
}

type ToolRegistry interface {
	RegisterTool(name string, tool ToolDescriptor) error
	GetTool(name string) (ToolDescriptor, error)
	GetAllTools() []ToolDescriptor
}

type toolRegistry struct {
	mu    sync.RWMutex
	tools map[string]ToolDescriptor
}

func NewToolRegistry() ToolRegistry {
	return &toolRegistry{tools: make(map[string]ToolDescriptor)}
}

func (registry *toolRegistry) RegisterTool(name string, tool ToolDescriptor) error {
	registry.mu.Lock()
	defer registry.mu.Unlock()

	if _, exists := registry.tools[name]; exists {
		return fmt.Errorf("tool already registered: %s", name)
	}
	registry.tools[name] = tool
	return nil
}

func (registry *toolRegistry) GetTool(name string) (ToolDescriptor, error) {
	registry.mu.RLock()
	defer registry.mu.RUnlock()

	tool, ok := registry.tools[name]
	if !ok {
		return ToolDescriptor{}, fmt.Errorf("tool not found: %s", name)
	}

	return tool, nil
}

func (registry *toolRegistry) GetAllTools() []ToolDescriptor {
	registry.mu.RLock()
	defer registry.mu.RUnlock()

	tools := make([]ToolDescriptor, 0, len(registry.tools))
	for _, tool := range registry.tools {
		tools = append(tools, tool)
	}

	return tools
}
