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
