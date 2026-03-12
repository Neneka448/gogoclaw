package tools

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestListDirToolReadsWorkspaceRoot(t *testing.T) {
	workspace := t.TempDir()
	if err := os.Mkdir(filepath.Join(workspace, "sessions"), 0755); err != nil {
		t.Fatalf("os.Mkdir() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "notes.txt"), []byte("hello"), 0644); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	descriptor := NewListDirTool(workspace)
	result, err := descriptor.Tool.Execute(`{"path":""}`)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	var parsed struct {
		Files []fileDesc `json:"files"`
		Error string     `json:"error"`
	}
	if err := json.Unmarshal([]byte(result), &parsed); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if parsed.Error != "" {
		t.Fatalf("parsed.Error = %q, want empty", parsed.Error)
	}
	if len(parsed.Files) != 2 {
		t.Fatalf("len(parsed.Files) = %d, want 2", len(parsed.Files))
	}
	if parsed.Files[0].FileName != "notes.txt" || parsed.Files[0].IsDir {
		t.Fatalf("parsed.Files[0] = %#v, want notes.txt file", parsed.Files[0])
	}
	if parsed.Files[1].FileName != "sessions" || !parsed.Files[1].IsDir {
		t.Fatalf("parsed.Files[1] = %#v, want sessions dir", parsed.Files[1])
	}
}

func TestListDirToolRejectsAbsolutePath(t *testing.T) {
	workspace := t.TempDir()
	descriptor := NewListDirTool(workspace)

	result, err := descriptor.Tool.Execute(`{"path":"/tmp"}`)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	var parsed struct {
		Files []fileDesc `json:"files"`
		Error string     `json:"error"`
	}
	if err := json.Unmarshal([]byte(result), &parsed); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if len(parsed.Files) != 0 {
		t.Fatalf("len(parsed.Files) = %d, want 0", len(parsed.Files))
	}
	if parsed.Error != "must not use absolute path" {
		t.Fatalf("parsed.Error = %q, want absolute path error", parsed.Error)
	}
}

func TestListDirToolRejectsOutsideWorkspace(t *testing.T) {
	workspace := t.TempDir()
	descriptor := NewListDirTool(workspace)

	result, err := descriptor.Tool.Execute(`{"path":"../other"}`)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	var parsed struct {
		Files []fileDesc `json:"files"`
		Error string     `json:"error"`
	}
	if err := json.Unmarshal([]byte(result), &parsed); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if len(parsed.Files) != 0 {
		t.Fatalf("len(parsed.Files) = %d, want 0", len(parsed.Files))
	}
	if parsed.Error != "path is outside the workspace" {
		t.Fatalf("parsed.Error = %q, want outside workspace error", parsed.Error)
	}
}

func TestListDirToolReturnsReadErrorInResult(t *testing.T) {
	workspace := t.TempDir()
	descriptor := NewListDirTool(workspace)

	result, err := descriptor.Tool.Execute(`{"path":"missing"}`)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	var parsed struct {
		Files []fileDesc `json:"files"`
		Error string     `json:"error"`
	}
	if err := json.Unmarshal([]byte(result), &parsed); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if len(parsed.Files) != 0 {
		t.Fatalf("len(parsed.Files) = %d, want 0", len(parsed.Files))
	}
	if parsed.Error == "" {
		t.Fatal("parsed.Error = empty, want read error")
	}
}
