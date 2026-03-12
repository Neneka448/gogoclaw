package systemprompt

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Neneka448/gogoclaw/internal/skills"
)

func TestServiceBuildCachesWorkspaceFiles(t *testing.T) {
	workspacePath := t.TempDir()
	agentsPath := filepath.Join(workspacePath, "AGENTS.md")
	if err := os.WriteFile(agentsPath, []byte("first version"), 0644); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	service := NewService(workspacePath)
	firstPrompt := service.Build(nil)
	if !strings.Contains(firstPrompt, "first version") {
		t.Fatalf("Build() = %q, want cached AGENTS.md content", firstPrompt)
	}

	if err := os.WriteFile(agentsPath, []byte("second version"), 0644); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	secondPrompt := service.Build(nil)
	if !strings.Contains(secondPrompt, "first version") {
		t.Fatalf("Build() after rewrite = %q, want first cached version", secondPrompt)
	}
	if strings.Contains(secondPrompt, "second version") {
		t.Fatalf("Build() after rewrite = %q, want cache to prevent reload", secondPrompt)
	}
}

func TestServiceBuildIncludesXMLSectionsAndSkills(t *testing.T) {
	workspacePath := t.TempDir()
	for fileName, content := range map[string]string{
		"AGENTS.md": "agent rules",
		"SOUL.md":   "assistant soul",
		"TOOLS.md":  "tool notes",
	} {
		if err := os.WriteFile(filepath.Join(workspacePath, fileName), []byte(content), 0644); err != nil {
			t.Fatalf("os.WriteFile(%s) error = %v", fileName, err)
		}
	}

	registry := skills.NewRegistry()
	if err := registry.Register(skills.Skill{Name: "article-summarize", FrontMatter: map[string]any{"description": "summarize links"}}); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	prompt := NewService(workspacePath).Build(registry)
	for _, fragment := range []string{"<agents>", "agent rules", "<soul>", "assistant soul", "<tools>", "tool notes", "<skills>", "article-summarize"} {
		if !strings.Contains(prompt, fragment) {
			t.Fatalf("prompt missing %q: %q", fragment, prompt)
		}
	}
}
