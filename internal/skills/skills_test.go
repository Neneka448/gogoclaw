package skills

import (
	"path/filepath"
	"testing"

	"os"
)

func TestLoadWorkspaceSkillsLoadsFrontMatterAndContent(t *testing.T) {
	workspace := t.TempDir()
	skillDir := filepath.Join(workspace, "skills", "article-summarize")
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		t.Fatalf("os.MkdirAll() error = %v", err)
	}
	content := "---\ndescription: summarize article links\ntriggers:\n  - summarize article\n  - read this link\n---\n# Skill\n\nUse this skill.\n"
	if err := os.WriteFile(filepath.Join(skillDir, skillFileName), []byte(content), 0644); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	registry, err := LoadWorkspaceSkills(workspace)
	if err != nil {
		t.Fatalf("LoadWorkspaceSkills() error = %v", err)
	}
	if got, want := registry.Len(), 1; got != want {
		t.Fatalf("registry.Len() = %d, want %d", got, want)
	}

	skill, err := registry.Get("article-summarize")
	if err != nil {
		t.Fatalf("registry.Get() error = %v", err)
	}
	if skill.Content != content {
		t.Fatalf("skill.Content = %q, want %q", skill.Content, content)
	}
	if got, want := skill.FrontMatter["description"], "summarize article links"; got != want {
		t.Fatalf("skill.FrontMatter[description] = %v, want %q", got, want)
	}
	triggers, ok := skill.FrontMatter["triggers"].([]any)
	if !ok {
		t.Fatalf("skill.FrontMatter[triggers] type = %T, want []any", skill.FrontMatter["triggers"])
	}
	if len(triggers) != 2 {
		t.Fatalf("len(triggers) = %d, want 2", len(triggers))
	}
}

func TestLoadWorkspaceSkillsRejectsSkillDirectoryWithoutSkillFile(t *testing.T) {
	workspace := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workspace, "skills", "broken-skill"), 0755); err != nil {
		t.Fatalf("os.MkdirAll() error = %v", err)
	}

	_, err := LoadWorkspaceSkills(workspace)
	if err == nil || err.Error() != "skill broken-skill is missing SKILL.md" {
		t.Fatalf("LoadWorkspaceSkills() error = %v, want missing SKILL.md", err)
	}
}
