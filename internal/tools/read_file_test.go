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
		Error     string `json:"error"`
	}
	if err := json.Unmarshal([]byte(result), &parsed); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if parsed.Error != "" {
		t.Fatalf("parsed.Error = %q, want empty", parsed.Error)
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
	result, err := descriptor.Tool.Execute(`{"path":"../secret.txt"}`)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	var parsed struct {
		Path  string `json:"path"`
		Error string `json:"error"`
	}
	if err := json.Unmarshal([]byte(result), &parsed); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if parsed.Path != "../secret.txt" {
		t.Fatalf("parsed.Path = %q, want ../secret.txt", parsed.Path)
	}
	if parsed.Error == "" {
		t.Fatal("parsed.Error = empty, want outside workspace error")
	}
}

func TestReadFileToolReturnsArgumentErrorsInResult(t *testing.T) {
	workspace := t.TempDir()
	filePath := filepath.Join(workspace, "notes.txt")
	if err := os.WriteFile(filePath, []byte("a\nb\nc\n"), 0644); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}
	descriptor := NewReadFileTool(workspace)

	result, err := descriptor.Tool.Execute(`{"path":"notes.txt","start_line":3,"end_line":2}`)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	var parsed struct {
		Path      string `json:"path"`
		StartLine int    `json:"start_line"`
		EndLine   int    `json:"end_line"`
		Error     string `json:"error"`
	}
	if err := json.Unmarshal([]byte(result), &parsed); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if parsed.Path != "notes.txt" {
		t.Fatalf("parsed.Path = %q, want notes.txt", parsed.Path)
	}
	if parsed.StartLine != 3 || parsed.EndLine != 2 {
		t.Fatalf("line range = (%d, %d), want (3, 2)", parsed.StartLine, parsed.EndLine)
	}
	if parsed.Error != "end_line must be greater than or equal to start_line" {
		t.Fatalf("parsed.Error = %q, want end_line validation error", parsed.Error)
	}
}
