package workspace

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Neneka448/gogoclaw/internal/config"
)

func TestCronTaskCreateRejectsUnknownProfile(t *testing.T) {
	workspacePath := t.TempDir()
	configPath := writeWorkspaceTestConfig(t, workspacePath)

	output, err := runTemplatePythonModule(
		t,
		"skills.cron_task.scripts.create",
		"--workspace", workspacePath,
		"--cron-id", "unknown-profile",
		"--cron-expression", "*/5 * * * *",
		"--task", "test task",
		"--profile-name", "missing",
		"--config-path", configPath,
	)
	if err == nil {
		t.Fatal("create.py error = nil, want unknown profile failure")
	}
	if !strings.Contains(output, `"error"`) || !strings.Contains(output, `unknown profile 'missing'`) {
		t.Fatalf("create.py output = %q, want unknown profile error", output)
	}

	if _, err := os.Stat(filepath.Join(workspacePath, "crons", "unknown-profile")); !os.IsNotExist(err) {
		t.Fatalf("cron directory created unexpectedly, stat error = %v", err)
	}
}

func TestCronTaskUpdateRejectsUnknownProfileWithoutMutatingConfig(t *testing.T) {
	workspacePath := t.TempDir()
	configPath := writeWorkspaceTestConfig(t, workspacePath)
	cronDir := filepath.Join(workspacePath, "crons", "existing-cron")
	if err := os.MkdirAll(cronDir, 0o755); err != nil {
		t.Fatalf("os.MkdirAll() error = %v", err)
	}
	initialConfig := `{
  "cronID": "existing-cron",
  "cronExpression": "*/5 * * * *",
  "enabled": true,
  "profileName": "default",
  "invocationMode": "cron"
}`
	if err := os.WriteFile(filepath.Join(cronDir, "config.json"), []byte(initialConfig), 0o644); err != nil {
		t.Fatalf("os.WriteFile(config.json) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(cronDir, "task.md"), []byte("task"), 0o644); err != nil {
		t.Fatalf("os.WriteFile(task.md) error = %v", err)
	}

	output, err := runTemplatePythonModule(
		t,
		"skills.cron_task.scripts.update",
		"--workspace", workspacePath,
		"--cron-id", "existing-cron",
		"--profile-name", "missing",
		"--config-path", configPath,
	)
	if err == nil {
		t.Fatal("update.py error = nil, want unknown profile failure")
	}
	if !strings.Contains(output, `"error"`) || !strings.Contains(output, `unknown profile 'missing'`) {
		t.Fatalf("update.py output = %q, want unknown profile error", output)
	}

	content, err := os.ReadFile(filepath.Join(cronDir, "config.json"))
	if err != nil {
		t.Fatalf("os.ReadFile(config.json) error = %v", err)
	}
	if string(content) != initialConfig {
		t.Fatalf("config.json mutated after failed update: %s", string(content))
	}
}

func TestNotifyCompletionQueuesFailureMetadata(t *testing.T) {
	workspacePath := t.TempDir()
	runtimeDir := filepath.Join(workspacePath, ".gogoclaw", "agent_bus")
	outboxDir := filepath.Join(runtimeDir, "outbox")
	sentDir := filepath.Join(runtimeDir, "sent")
	failedDir := filepath.Join(runtimeDir, "failed")
	if err := os.MkdirAll(outboxDir, 0o755); err != nil {
		t.Fatalf("os.MkdirAll(outbox) error = %v", err)
	}
	if err := os.MkdirAll(sentDir, 0o755); err != nil {
		t.Fatalf("os.MkdirAll(sent) error = %v", err)
	}
	if err := os.MkdirAll(failedDir, 0o755); err != nil {
		t.Fatalf("os.MkdirAll(failed) error = %v", err)
	}

	runtimeConfig := map[string]string{
		"runtime_dir":        runtimeDir,
		"outbox_dir":         outboxDir,
		"sent_dir":           sentDir,
		"failed_dir":         failedDir,
		"source_profile":     "default",
		"source_instance_id": "default@test-machine",
	}
	encodedRuntimeConfig, err := json.Marshal(runtimeConfig)
	if err != nil {
		t.Fatalf("json.Marshal(runtimeConfig) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(runtimeDir, "config.json"), encodedRuntimeConfig, 0o644); err != nil {
		t.Fatalf("os.WriteFile(runtime config) error = %v", err)
	}

	invocationDir := filepath.Join(workspacePath, "invocations", "inv-test-001")
	if err := os.MkdirAll(filepath.Join(invocationDir, "reports"), 0o755); err != nil {
		t.Fatalf("os.MkdirAll(invocation reports) error = %v", err)
	}
	manifest := map[string]string{
		"invocation_id":         "inv-test-001",
		"caller_profile":        "front",
		"target_profile":        "worker",
		"task_summary":          "count files",
		"return_channel_id":     "mq",
		"return_chat_id":        "chat-1",
		"return_message_id":     "msg-root",
		"return_message_type":   "direct",
		"return_sender_id":      "user-1",
		"return_reply_to":       "msg-root",
		"return_correlation_id": "msg-root",
		"return_session_id":     "mq:chat-1",
		"return_workspace":      workspacePath,
	}
	writeWorkspaceJSON(t, filepath.Join(invocationDir, "manifest.json"), manifest)
	writeWorkspaceJSON(t, filepath.Join(invocationDir, "status.json"), map[string]string{
		"status":      "failed",
		"started_at":  "2026-03-21T10:00:00Z",
		"finished_at": "2026-03-21T10:01:00Z",
		"error":       "count command failed",
	})
	if err := os.WriteFile(filepath.Join(invocationDir, "reports", "final.md"), []byte("task failed: count command failed\n"), 0o644); err != nil {
		t.Fatalf("os.WriteFile(final.md) error = %v", err)
	}

	output, err := runTemplatePythonModule(
		t,
		"skills.invoke_agent.scripts.notify_completion",
		"--workspace", workspacePath,
		"--invocation-dir", invocationDir,
	)
	if err != nil {
		t.Fatalf("notify_completion.py error = %v, output = %s", err, output)
	}

	matches, err := filepath.Glob(filepath.Join(outboxDir, "*.json"))
	if err != nil {
		t.Fatalf("filepath.Glob() error = %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("len(outbox files) = %d, want 1", len(matches))
	}

	content, err := os.ReadFile(matches[0])
	if err != nil {
		t.Fatalf("os.ReadFile(outbox message) error = %v", err)
	}
	var envelope map[string]any
	if err := json.Unmarshal(content, &envelope); err != nil {
		t.Fatalf("json.Unmarshal(envelope) error = %v", err)
	}

	if got := envelope["target_profile"]; got != "front" {
		t.Fatalf("target_profile = %v, want front", got)
	}
	if got := envelope["source_profile"]; got != "worker" {
		t.Fatalf("source_profile = %v, want worker", got)
	}
	body, _ := envelope["body"].(string)
	if !strings.Contains(body, "Status: failed") || !strings.Contains(body, "Error: count command failed") {
		t.Fatalf("body = %q, want failed status and error", body)
	}

	metadata, ok := envelope["metadata"].(map[string]any)
	if !ok {
		t.Fatalf("metadata type = %T, want map[string]any", envelope["metadata"])
	}
	if got := metadata["status"]; got != "failed" {
		t.Fatalf("metadata.status = %v, want failed", got)
	}
	if got := metadata["error"]; got != "count command failed" {
		t.Fatalf("metadata.error = %v, want count command failed", got)
	}
	if got := metadata["return_message_id"]; got != "msg-root" {
		t.Fatalf("metadata.return_message_id = %v, want msg-root", got)
	}
}

func runTemplatePythonModule(t *testing.T, module string, args ...string) (string, error) {
	t.Helper()

	commandArgs := append([]string{"-m", module}, args...)
	cmd := exec.Command("python3", commandArgs...)
	cmd.Env = append(os.Environ(), "PYTHONPATH="+filepath.Join(repoRoot(t), "internal", "workspace", "templates"))
	cmd.Dir = repoRoot(t)

	var combined bytes.Buffer
	cmd.Stdout = &combined
	cmd.Stderr = &combined

	err := cmd.Run()
	return combined.String(), err
}

func repoRoot(t *testing.T) string {
	t.Helper()

	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("os.Getwd() error = %v", err)
	}
	return filepath.Dir(filepath.Dir(wd))
}

func writeWorkspaceTestConfig(t *testing.T, workspacePath string) string {
	t.Helper()

	configPath := filepath.Join(t.TempDir(), "config.json")
	cfg := config.CreateDefaultConfig()
	cfg.Agents.Profiles["default"] = config.ProfileConfig{
		Workspace:         workspacePath,
		Provider:          "codex",
		Model:             "gpt-5.4",
		MaxTokens:         512,
		Temperature:       0.1,
		MaxToolIterations: 4,
		MemoryWindow:      10,
		MaxRetryTimes:     1,
	}
	encoded, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("json.Marshal(config) error = %v", err)
	}
	if err := os.WriteFile(configPath, encoded, 0o644); err != nil {
		t.Fatalf("os.WriteFile(config) error = %v", err)
	}
	return configPath
}

func writeWorkspaceJSON(t *testing.T, path string, payload any) {
	t.Helper()

	encoded, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		t.Fatalf("json.MarshalIndent(%s) error = %v", path, err)
	}
	if err := os.WriteFile(path, encoded, 0o644); err != nil {
		t.Fatalf("os.WriteFile(%s) error = %v", path, err)
	}
}
