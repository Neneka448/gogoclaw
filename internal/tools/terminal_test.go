package tools

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	messagebus "github.com/Neneka448/gogoclaw/internal/message_bus"
)

func TestTerminalToolRunsCommandInWorkspace(t *testing.T) {
	workspace := t.TempDir()
	if err := os.Mkdir(filepath.Join(workspace, "subdir"), 0755); err != nil {
		t.Fatalf("os.Mkdir() error = %v", err)
	}
	expectedCwd, err := filepath.EvalSymlinks(filepath.Join(workspace, "subdir"))
	if err != nil {
		t.Fatalf("filepath.EvalSymlinks() error = %v", err)
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
	if parsed.Cwd != expectedCwd {
		t.Fatalf("parsed.Cwd = %q, want %q", parsed.Cwd, expectedCwd)
	}
	if strings.TrimSpace(parsed.Stdout) != expectedCwd {
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
	if !strings.Contains(parsed.Stderr, "err") {
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

func TestTerminalToolRejectsAbsoluteCwd(t *testing.T) {
	workspace := t.TempDir()
	subdir := filepath.Join(workspace, "subdir")
	if err := os.Mkdir(subdir, 0755); err != nil {
		t.Fatalf("os.Mkdir() error = %v", err)
	}
	descriptor := NewTerminalTool(workspace, time.Second)
	result, err := descriptor.Tool.Execute(`{"command":"pwd","cwd":"` + subdir + `"}`)
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
	if strings.TrimSpace(parsed.Stdout) != parsed.Cwd {
		t.Fatalf("parsed.Stdout = %q, want %q", parsed.Stdout, parsed.Cwd)
	}
}

func TestTerminalToolRejectsAbsoluteCwdOutsideWorkspace(t *testing.T) {
	workspace := t.TempDir()
	descriptor := NewTerminalTool(workspace, time.Second)
	result, err := descriptor.Tool.Execute(`{"command":"pwd","cwd":"/tmp"}`)
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
}

func TestTerminalToolInjectsMessageContextEnv(t *testing.T) {
	workspace := t.TempDir()
	descriptor := NewTerminalTool(workspace, time.Second)
	contextTool, ok := descriptor.Tool.(MessageContextTool)
	if !ok {
		t.Fatal("terminal tool does not implement MessageContextTool")
	}
	contextTool.SetMessageContext(messagebus.Message{
		ChannelID:   "feishu",
		ChatID:      "oc_chat_1",
		MessageID:   "om_1",
		MessageType: "text",
		SenderID:    "ou_user_1",
		ReplyTo:     "om_parent",
		Metadata: map[string]string{
			"agent_profile": "default",
			"foo":           "bar",
		},
	})

	result, err := descriptor.Tool.Execute(`{"command":"printf '%s|%s|%s|%s' \"$GOGOCLAW_CHANNEL_ID\" \"$GOGOCLAW_CHAT_ID\" \"$GOGOCLAW_SESSION_ID\" \"$GOGOCLAW_MESSAGE_METADATA_JSON\""}`)
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
	if !strings.Contains(parsed.Stdout, "feishu|oc_chat_1|feishu:oc_chat_1|") {
		t.Fatalf("parsed.Stdout = %q, want injected channel/chat/session env", parsed.Stdout)
	}
	if !strings.Contains(parsed.Stdout, `"foo":"bar"`) {
		t.Fatalf("parsed.Stdout = %q, want metadata json", parsed.Stdout)
	}
}

func TestTerminalToolInjectsWorkspaceIntoPythonPath(t *testing.T) {
	workspace := t.TempDir()
	skillDir := filepath.Join(workspace, "skills", "pkgtest")
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		t.Fatalf("os.MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "__init__.py"), []byte(""), 0644); err != nil {
		t.Fatalf("os.WriteFile(__init__.py) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "mod.py"), []byte("VALUE = 'ok'\n"), 0644); err != nil {
		t.Fatalf("os.WriteFile(mod.py) error = %v", err)
	}
	if err := os.Mkdir(filepath.Join(workspace, "invocations"), 0755); err != nil {
		t.Fatalf("os.Mkdir(invocations) error = %v", err)
	}

	descriptor := NewTerminalTool(workspace, time.Second)
	result, err := descriptor.Tool.Execute(`{"command":"python3 -c \"from skills.pkgtest.mod import VALUE; print(VALUE)\"","cwd":"invocations"}`)
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
	if strings.TrimSpace(parsed.Stdout) != "ok" {
		t.Fatalf("parsed.Stdout = %q, want ok", parsed.Stdout)
	}
}
