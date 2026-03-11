package tools

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestReadFileToolReadsSelectedLines(t *testing.T) {
	workspace := t.TempDir()
	filePath := filepath.Join(workspace, "notes.txt")
	if err := os.WriteFile(filePath, []byte("a\nb\nc\nd\n"), 0644); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	descriptor := NewReadFileTool(workspace)
	result, err := descriptor.Tool.Execute(`{"path":"notes.txt","start_line":2,"end_line":3}`)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	var parsed struct {
		Path      string `json:"path"`
		StartLine int    `json:"start_line"`
		EndLine   int    `json:"end_line"`
		Content   string `json:"content"`
	}
	if err := json.Unmarshal([]byte(result), &parsed); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if parsed.Path != "notes.txt" {
		t.Fatalf("parsed.Path = %q, want notes.txt", parsed.Path)
	}
	if parsed.StartLine != 2 || parsed.EndLine != 3 {
		t.Fatalf("line range = (%d, %d), want (2, 3)", parsed.StartLine, parsed.EndLine)
	}
	if parsed.Content != "b\nc" {
		t.Fatalf("parsed.Content = %q, want b\\nc", parsed.Content)
	}
}

func TestReadFileToolRejectsOutsideWorkspace(t *testing.T) {
	workspace := t.TempDir()
	descriptor := NewReadFileTool(workspace)
	_, err := descriptor.Tool.Execute(`{"path":"../secret.txt"}`)
	if err == nil {
		t.Fatal("Execute() error = nil, want outside workspace error")
	}
}