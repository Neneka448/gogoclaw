package bootstrap

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/Neneka448/gogoclaw/internal/config"
	messagebus "github.com/Neneka448/gogoclaw/internal/message_bus"
)

func TestBootstrapInitializesSessionManagerFromWorkspace(t *testing.T) {
	tempDir := t.TempDir()
	workspaceDir := filepath.Join(tempDir, "workspace")
	configPath := filepath.Join(tempDir, "config.json")

	defaultConfig := config.CreateDefaultConfig()
	defaultConfig.Agents.Profiles["default"] = config.ProfileConfig{
		Workspace:         workspaceDir,
		Provider:          "codex",
		Model:             "gpt-5.4",
		MaxTokens:         512,
		Temperature:       0.1,
		MaxToolIterations: 4,
		MemoryWindow:      10,
		MaxRetryTimes:     1,
	}

	encoded, err := json.Marshal(defaultConfig)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	if err := os.WriteFile(configPath, encoded, 0644); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	gatewayRef, err := Bootstrap(configPath)
	if err != nil {
		t.Fatalf("Bootstrap() error = %v", err)
	}

	responses, err := (*gatewayRef).DirectProcessAndReturn(messagebus.Message{
		ChannelID: "telegram",
		ChatID:    "chat-1",
		SenderID:  "user-1",
	})
	if err != nil {
		t.Fatalf("DirectProcessAndReturn() error = %v", err)
	}
	if len(responses) != 0 {
		t.Fatalf("len(responses) = %d, want 0", len(responses))
	}

	if _, err := os.Stat(filepath.Join(workspaceDir, "sessions", "telegram:chat-1.json")); err != nil {
		t.Fatalf("session file not created: %v", err)
	}
	if _, err := os.Stat(filepath.Join(workspaceDir, "AGENTS.md")); err == nil {
		t.Fatalf("Bootstrap() should not create workspace bootstrap files automatically")
	}
	if err := (*gatewayRef).Start(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(workspaceDir, "sqlite-vec", "store.db")); err != nil {
		t.Fatalf("sqlite-vec database not created: %v", err)
	}
	if err := (*gatewayRef).Stop(); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
}

func TestBootstrapRoutesInvocationToNamedProfileWorkspace(t *testing.T) {
	tempDir := t.TempDir()
	defaultWorkspace := filepath.Join(tempDir, "default-workspace")
	workerWorkspace := filepath.Join(tempDir, "worker-workspace")
	configPath := filepath.Join(tempDir, "config.json")

	defaultConfig := config.CreateDefaultConfig()
	defaultConfig.Agents.Profiles["default"] = config.ProfileConfig{
		Workspace:         defaultWorkspace,
		Provider:          "codex",
		Model:             "gpt-5.4",
		MaxTokens:         512,
		Temperature:       0.1,
		MaxToolIterations: 4,
		MemoryWindow:      10,
		MaxRetryTimes:     1,
	}
	defaultConfig.Agents.Profiles["worker"] = config.ProfileConfig{
		Workspace:         workerWorkspace,
		Provider:          "codex",
		EmbeddingProfile:  "default",
		Model:             "gpt-5.4",
		MaxTokens:         512,
		Temperature:       0.1,
		MaxToolIterations: 4,
		MemoryWindow:      10,
		MaxRetryTimes:     1,
	}

	encoded, err := json.Marshal(defaultConfig)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	if err := os.WriteFile(configPath, encoded, 0644); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	gatewayRef, err := Bootstrap(configPath)
	if err != nil {
		t.Fatalf("Bootstrap() error = %v", err)
	}
	defer func() {
		if err := (*gatewayRef).Stop(); err != nil {
			t.Fatalf("Stop() error = %v", err)
		}
	}()

	responses, err := (*gatewayRef).DirectProcessAndReturn(messagebus.Message{
		ChannelID: "cli",
		ChatID:    "chat-worker",
		SenderID:  "user-1",
		Metadata:  map[string]string{"agent_profile": "worker"},
	})
	if err != nil {
		t.Fatalf("DirectProcessAndReturn() error = %v", err)
	}
	if len(responses) != 0 {
		t.Fatalf("len(responses) = %d, want 0", len(responses))
	}
	if _, err := os.Stat(filepath.Join(workerWorkspace, "sessions", "cli:chat-worker.json")); err != nil {
		t.Fatalf("worker session file not created: %v", err)
	}
	if _, err := os.Stat(filepath.Join(defaultWorkspace, "sessions", "cli:chat-worker.json")); err == nil {
		t.Fatal("default workspace unexpectedly received worker session file")
	}
}
