package bootstrap

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

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
}

func TestResolveToolTimeoutUsesConfiguredValue(t *testing.T) {
	timeout := resolveToolTimeout([]config.ToolConfig{{Name: "terminal", Timeout: 12}}, "terminal", 30*time.Second)
	if timeout != 12*time.Second {
		t.Fatalf("resolveToolTimeout() = %v, want 12s", timeout)
	}
}

func TestResolveToolTimeoutFallsBackToDefault(t *testing.T) {
	timeout := resolveToolTimeout([]config.ToolConfig{{Name: "read_file", Timeout: 5}}, "terminal", 30*time.Second)
	if timeout != 30*time.Second {
		t.Fatalf("resolveToolTimeout() = %v, want 30s", timeout)
	}
}

func TestBuildEmbeddingProvidersReusesSameProviderInstance(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.json")

	defaultConfig := config.CreateDefaultConfig()
	defaultConfig.Agents.Profiles["default"] = config.ProfileConfig{
		Workspace:         tempDir,
		Provider:          "codex",
		Model:             "gpt-5.4",
		MaxTokens:         512,
		Temperature:       0.1,
		MaxToolIterations: 4,
		MemoryWindow:      10,
		MaxRetryTimes:     1,
	}
	defaultConfig.Embedding.Profiles["default"] = config.EmbeddingProfileConfig{
		Text: config.EmbeddingModelConfig{
			Provider: "voyageai",
			Model:    "voyage-4",
		},
		Modal: config.EmbeddingModelConfig{
			Provider: "voyageai",
			Model:    "voyage-multimodal-3.5",
		},
	}

	encoded, err := json.Marshal(defaultConfig)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	if err := os.WriteFile(configPath, encoded, 0644); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	manager := config.NewConfigManager(configPath)
	embeddingProfile, err := manager.GetEmbeddingProfileConfig("default")
	if err != nil {
		t.Fatalf("GetEmbeddingProfileConfig() error = %v", err)
	}

	textProvider, modalProvider, err := buildEmbeddingProviders(manager, embeddingProfile)
	if err != nil {
		t.Fatalf("buildEmbeddingProviders() error = %v", err)
	}
	if textProvider == nil || modalProvider == nil {
		t.Fatalf("providers = (%v, %v), want non-nil", textProvider, modalProvider)
	}
	if textProvider != modalProvider {
		t.Fatal("expected textProvider and modalProvider to reuse the same provider instance")
	}
}
