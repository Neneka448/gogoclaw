package tools

import "fmt"

type filteredRegistry struct {
	inner   ToolRegistry
	allowed map[string]struct{}
}

// NewFilteredRegistry wraps inner with allow/forbid filtering.
// allowed empty/nil or containing "*" means all tools are allowed.
// forbidden containing "*" means all tools are forbidden.
// Forbidden takes precedence over allowed.
func NewFilteredRegistry(inner ToolRegistry, allowed, forbidden []string) ToolRegistry {
	allTools := inner.GetAllTools()
	allNames := make(map[string]struct{}, len(allTools))
	for _, t := range allTools {
		allNames[t.Name] = struct{}{}
	}

	allowSet := allNames
	if len(allowed) > 0 && !containsWildcard(allowed) {
		allowSet = toSet(allowed)
	}

	forbidSet := make(map[string]struct{})
	if containsWildcard(forbidden) {
		forbidSet = allNames
	} else {
		forbidSet = toSet(forbidden)
	}

	result := make(map[string]struct{}, len(allowSet))
	for name := range allowSet {
		if _, blocked := forbidSet[name]; !blocked {
			result[name] = struct{}{}
		}
	}

	return &filteredRegistry{inner: inner, allowed: result}
}

func (r *filteredRegistry) RegisterTool(name string, tool ToolDescriptor) error {
	return r.inner.RegisterTool(name, tool)
}

func (r *filteredRegistry) GetTool(name string) (ToolDescriptor, error) {
	if _, ok := r.allowed[name]; !ok {
		return ToolDescriptor{}, fmt.Errorf("tool not found: %s", name)
	}
	return r.inner.GetTool(name)
}

func (r *filteredRegistry) GetAllTools() []ToolDescriptor {
	all := r.inner.GetAllTools()
	filtered := make([]ToolDescriptor, 0, len(r.allowed))
	for _, t := range all {
		if _, ok := r.allowed[t.Name]; ok {
			filtered = append(filtered, t)
		}
	}
	return filtered
}

func containsWildcard(items []string) bool {
	for _, item := range items {
		if item == "*" {
			return true
		}
	}
	return false
}

func toSet(items []string) map[string]struct{} {
	s := make(map[string]struct{}, len(items))
	for _, item := range items {
		s[item] = struct{}{}
	}
	return s
}
