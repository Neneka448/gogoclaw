package pathutil

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveWithinWorkspaceAllowsAbsolutePathInsideWorkspace(t *testing.T) {
	workspace := t.TempDir()
	target := filepath.Join(workspace, "notes.txt")
	if err := os.WriteFile(target, []byte("ok"), 0644); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}
	expected, err := filepath.EvalSymlinks(target)
	if err != nil {
		t.Fatalf("filepath.EvalSymlinks() error = %v", err)
	}

	resolved, err := ResolveWithinWorkspace(target, workspace)
	if err != nil {
		t.Fatalf("ResolveWithinWorkspace() error = %v", err)
	}
	if resolved != expected {
		t.Fatalf("ResolveWithinWorkspace() = %q, want %q", resolved, expected)
	}
}

func TestResolveWithinWorkspaceRejectsPathOutsideWorkspace(t *testing.T) {
	workspace := t.TempDir()

	if _, err := ResolveWithinWorkspace("../secret.txt", workspace); err == nil {
		t.Fatal("ResolveWithinWorkspace() error = nil, want outside workspace error")
	}
}

func TestResolveWithinWorkspaceRejectsSymlinkEscape(t *testing.T) {
	workspace := t.TempDir()
	outside := t.TempDir()
	linkPath := filepath.Join(workspace, "escape")

	if err := os.Symlink(outside, linkPath); err != nil {
		t.Fatalf("os.Symlink() error = %v", err)
	}

	if _, err := ResolveWithinWorkspace(filepath.Join("escape", "secret.txt"), workspace); err == nil {
		t.Fatal("ResolveWithinWorkspace() error = nil, want symlink escape error")
	}
}

func TestResolveRelativeOnlyRejectsAbsolutePath(t *testing.T) {
	workspace := t.TempDir()
	absolute := filepath.Join(workspace, "notes.txt")

	if _, err := ResolveRelativeOnly(absolute, workspace); err == nil {
		t.Fatal("ResolveRelativeOnly() error = nil, want absolute path error")
	}
}

func TestResolveRelativeOnlyResolvesWorkspaceRootForEmptyPath(t *testing.T) {
	workspace := t.TempDir()
	expected, err := filepath.EvalSymlinks(workspace)
	if err != nil {
		t.Fatalf("filepath.EvalSymlinks() error = %v", err)
	}

	resolved, err := ResolveRelativeOnly("", workspace)
	if err != nil {
		t.Fatalf("ResolveRelativeOnly() error = %v", err)
	}
	if resolved != expected {
		t.Fatalf("ResolveRelativeOnly() = %q, want %q", resolved, expected)
	}
}
