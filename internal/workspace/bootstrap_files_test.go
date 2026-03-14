package workspace

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEnsureBootstrapFilesCreatesAllTemplates(t *testing.T) {
	workspacePath := t.TempDir()

	if err := EnsureBootstrapFiles(workspacePath); err != nil {
		t.Fatalf("EnsureBootstrapFiles() error = %v", err)
	}

	for _, fileName := range BootstrapFileNames() {
		if _, err := os.Stat(filepath.Join(workspacePath, fileName)); err != nil {
			t.Fatalf("Stat(%s) error = %v", fileName, err)
		}
	}
}

func TestEnsureBootstrapFilesPreservesExistingContent(t *testing.T) {
	workspacePath := t.TempDir()
	targetPath := filepath.Join(workspacePath, agentsFile)
	if err := os.WriteFile(targetPath, []byte("custom agents"), 0644); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	if err := EnsureBootstrapFiles(workspacePath); err != nil {
		t.Fatalf("EnsureBootstrapFiles() error = %v", err)
	}

	content, err := os.ReadFile(targetPath)
	if err != nil {
		t.Fatalf("os.ReadFile() error = %v", err)
	}
	if string(content) != "custom agents" {
		t.Fatalf("AGENTS.md = %q, want custom content preserved", string(content))
	}
}

func TestEnsureDefaultSkillsCreatesSkillCreator(t *testing.T) {
	workspacePath := t.TempDir()

	if err := EnsureDefaultSkills(workspacePath); err != nil {
		t.Fatalf("EnsureDefaultSkills() error = %v", err)
	}

	skillPath := filepath.Join(workspacePath, "skills", "skill-creator", skillFileName)
	info, err := os.Stat(skillPath)
	if err != nil {
		t.Fatalf("Stat(%s) error = %v", skillPath, err)
	}
	if info.Size() == 0 {
		t.Fatalf("skill-creator SKILL.md is empty")
	}
}

func TestEnsureDefaultSkillsPreservesExistingSkill(t *testing.T) {
	workspacePath := t.TempDir()
	skillDir := filepath.Join(workspacePath, "skills", "skill-creator")
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		t.Fatalf("os.MkdirAll() error = %v", err)
	}
	skillPath := filepath.Join(skillDir, skillFileName)
	if err := os.WriteFile(skillPath, []byte("custom skill content"), 0644); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	if err := EnsureDefaultSkills(workspacePath); err != nil {
		t.Fatalf("EnsureDefaultSkills() error = %v", err)
	}

	content, err := os.ReadFile(skillPath)
	if err != nil {
		t.Fatalf("os.ReadFile() error = %v", err)
	}
	if string(content) != "custom skill content" {
		t.Fatalf("skill-creator SKILL.md = %q, want custom content preserved", string(content))
	}
}

func TestEnsureDefaultSkillsRejectsEmptyPath(t *testing.T) {
	if err := EnsureDefaultSkills(""); err == nil {
		t.Fatal("EnsureDefaultSkills('') should return error")
	}
}
