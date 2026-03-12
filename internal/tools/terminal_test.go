package tools

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestTerminalToolRunsCommandInWorkspace(t *testing.T) {
	workspace := t.TempDir()
	if err := os.Mkdir(filepath.Join(workspace, "subdir"), 0755); err != nil {
		t.Fatalf("os.Mkdir() error = %v", err)
	}

	descriptor := NewTerminalTool(workspace, time.Second)
	result, err := descriptor.Tool.Execute(`{"command":"pwd","cwd":"subdir"}`)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	var parsed terminalResult
	if err := json.Unmarshal([]byte(result), &parsed); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if parsed.Error != "" {
		t.Fatalf("parsed.Error = %q, want empty", parsed.Error)
	}
	if parsed.ExitCode != 0 {
		t.Fatalf("parsed.ExitCode = %d, want 0", parsed.ExitCode)
	}
	if parsed.Cwd != filepath.Join(workspace, "subdir") {
		t.Fatalf("parsed.Cwd = %q, want %q", parsed.Cwd, filepath.Join(workspace, "subdir"))
	}
	if strings.TrimSpace(parsed.Stdout) != filepath.Join(workspace, "subdir") {
		t.Fatalf("parsed.Stdout = %q, want pwd output", parsed.Stdout)
	}
}

func TestTerminalToolReturnsStderrAndExitCode(t *testing.T) {
	workspace := t.TempDir()
	descriptor := NewTerminalTool(workspace, time.Second)
	result, err := descriptor.Tool.Execute(`{"command":"echo err 1>&2; exit 7"}`)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	var parsed terminalResult
	if err := json.Unmarshal([]byte(result), &parsed); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if parsed.Error != "" {
		t.Fatalf("parsed.Error = %q, want empty", parsed.Error)
	}
	if parsed.ExitCode != 7 {
		t.Fatalf("parsed.ExitCode = %d, want 7", parsed.ExitCode)
	}
	if strings.TrimSpace(parsed.Stderr) != "err" {
		t.Fatalf("parsed.Stderr = %q, want err", parsed.Stderr)
	}
}

func TestTerminalToolTimesOut(t *testing.T) {
	workspace := t.TempDir()
	descriptor := NewTerminalTool(workspace, 50*time.Millisecond)
	result, err := descriptor.Tool.Execute(`{"command":"sleep 1"}`)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	var parsed terminalResult
	if err := json.Unmarshal([]byte(result), &parsed); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if !parsed.TimedOut {
		t.Fatal("parsed.TimedOut = false, want true")
	}
	if parsed.ExitCode != -1 {
		t.Fatalf("parsed.ExitCode = %d, want -1", parsed.ExitCode)
	}
	if parsed.Error == "" {
		t.Fatal("parsed.Error = empty, want timeout message")
	}
}

func TestTerminalToolRejectsEmptyCommand(t *testing.T) {
	workspace := t.TempDir()
	descriptor := NewTerminalTool(workspace, time.Second)
	result, err := descriptor.Tool.Execute(`{"command":"   "}`)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	var parsed terminalResult
	if err := json.Unmarshal([]byte(result), &parsed); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if parsed.Error != "terminal command is required" {
		t.Fatalf("parsed.Error = %q, want terminal command is required", parsed.Error)
	}
}

func TestTerminalToolRejectsOutsideWorkspaceCwd(t *testing.T) {
	workspace := t.TempDir()
	descriptor := NewTerminalTool(workspace, time.Second)
	result, err := descriptor.Tool.Execute(`{"command":"pwd","cwd":"../other"}`)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	var parsed terminalResult
	if err := json.Unmarshal([]byte(result), &parsed); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if parsed.Error == "" {
		t.Fatal("parsed.Error = empty, want outside workspace error")
	}
	if parsed.ExitCode != -1 {
		t.Fatalf("parsed.ExitCode = %d, want -1", parsed.ExitCode)
	}
}
