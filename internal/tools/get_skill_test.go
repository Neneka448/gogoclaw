package tools

import (
	"encoding/json"
	"testing"

	"github.com/Neneka448/gogoclaw/internal/skills"
)

func TestGetSkillToolReturnsFullSkillContent(t *testing.T) {
	registry := skills.NewRegistry()
	content := "---\ndescription: summarize\n---\n# Skill\n"
	if err := registry.Register(skills.Skill{
		Name:        "article-summarize",
		FilePath:    "/tmp/workspace/skills/article-summarize/SKILL.md",
		FrontMatter: map[string]any{"description": "summarize"},
		Content:     content,
	}); err != nil {
		t.Fatalf("registry.Register() error = %v", err)
	}

	descriptor := NewGetSkillTool(registry)
	result, err := descriptor.Tool.Execute(`{"name":"article-summarize"}`)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	var parsed getSkillResult
	if err := json.Unmarshal([]byte(result), &parsed); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if parsed.Name != "article-summarize" {
		t.Fatalf("parsed.Name = %q, want article-summarize", parsed.Name)
	}
	if parsed.Content != content {
		t.Fatalf("parsed.Content = %q, want %q", parsed.Content, content)
	}
	if parsed.FrontMatter["description"] != "summarize" {
		t.Fatalf("parsed.FrontMatter[description] = %v, want summarize", parsed.FrontMatter["description"])
	}
}

func TestGetSkillToolReturnsErrorForUnknownSkill(t *testing.T) {
	descriptor := NewGetSkillTool(skills.NewRegistry())
	result, err := descriptor.Tool.Execute(`{"name":"missing"}`)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	var parsed getSkillResult
	if err := json.Unmarshal([]byte(result), &parsed); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if parsed.Error != "skill not found: missing" {
		t.Fatalf("parsed.Error = %q, want skill not found", parsed.Error)
	}
}
