package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestNewConfigManagerLoadsConfig(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.json")
	defaultConfig := CreateDefaultConfig()
	defaultConfig.Agents.Profiles["default"] = ProfileConfig{
		Workspace:         tempDir,
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

	manager := NewConfigManager(configPath)
	profile, err := manager.GetAgentProfileConfig("default")
	if err != nil {
		t.Fatalf("GetAgentProfileConfig() error = %v", err)
	}
	if profile.Model != "gpt-5.4" {
		t.Fatalf("profile.Model = %q, want gpt-5.4", profile.Model)
	}

	embeddingProfile, err := manager.GetEmbeddingProfileConfig("default")
	if err != nil {
		t.Fatalf("GetEmbeddingProfileConfig() error = %v", err)
	}
	if embeddingProfile.Text.Provider != "" || embeddingProfile.Modal.Provider != "" {
		t.Fatalf("embedding profile = %#v, want empty default provider selections", embeddingProfile)
	}
	if embeddingProfile.Text.OutputDimension != 0 || embeddingProfile.Modal.OutputDimension != 0 {
		t.Fatalf("embedding output dimensions = (%d, %d), want 0,0", embeddingProfile.Text.OutputDimension, embeddingProfile.Modal.OutputDimension)
	}

	embeddingProvider, err := manager.GetEmbeddingProviderConfig("voyageai")
	if err != nil {
		t.Fatalf("GetEmbeddingProviderConfig() error = %v", err)
	}
	if embeddingProvider.Name != "voyageai" {
		t.Fatalf("embeddingProvider.Name = %q, want voyageai", embeddingProvider.Name)
	}
	loadedConfig, err := manager.GetConfig()
	if err != nil {
		t.Fatalf("GetConfig() error = %v", err)
	}
	if loadedConfig.Cron.Timezone != "Europe/London" {
		t.Fatalf("Cron.Timezone = %q, want Europe/London", loadedConfig.Cron.Timezone)
	}
	if loadedConfig.MCP.MCPServers == nil {
		t.Fatal("MCP.MCPServers = nil, want empty map")
	}
}

func TestNewConfigManagerLoadsMCPConfig(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.json")
	defaultConfig := CreateDefaultConfig()
	defaultConfig.Agents.Profiles["default"] = ProfileConfig{
		Workspace:         tempDir,
		Provider:          "codex",
		Model:             "gpt-5.4",
		MaxTokens:         512,
		Temperature:       0.1,
		MaxToolIterations: 4,
		MemoryWindow:      10,
		MaxRetryTimes:     1,
	}
	defaultConfig.MCP.MCPServers["docs"] = MCPServerConfig{
		Enabled: true,
		Command: "npx",
		Args:    []string{"-y", "@modelcontextprotocol/server-filesystem"},
		Env: map[string]string{
			"TOKEN": "abc",
		},
	}

	encoded, err := json.Marshal(defaultConfig)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	if err := os.WriteFile(configPath, encoded, 0644); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	manager := NewConfigManager(configPath)
	loadedConfig, err := manager.GetConfig()
	if err != nil {
		t.Fatalf("GetConfig() error = %v", err)
	}
	server, ok := loadedConfig.MCP.MCPServers["docs"]
	if !ok {
		t.Fatal("MCP.MCPServers[docs] missing")
	}
	if !server.Enabled {
		t.Fatal("server.Enabled = false, want true")
	}
	if server.Command != "npx" {
		t.Fatalf("server.Command = %q, want npx", server.Command)
	}
	if len(server.Args) != 2 {
		t.Fatalf("len(server.Args) = %d, want 2", len(server.Args))
	}
	if server.Env["TOKEN"] != "abc" {
		t.Fatalf("server.Env[TOKEN] = %q, want abc", server.Env["TOKEN"])
	}
}

func TestGetConfigReturnsSnapshot(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.json")
	defaultConfig := CreateDefaultConfig()
	defaultConfig.Agents.Profiles["default"] = ProfileConfig{
		Workspace:         tempDir,
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

	manager := NewConfigManager(configPath)
	cfg, err := manager.GetConfig()
	if err != nil {
		t.Fatalf("GetConfig() error = %v", err)
	}

	cfg.Agents.Profiles["default"] = ProfileConfig{Workspace: "changed"}
	cfg.MCP.MCPServers["docs"] = MCPServerConfig{Enabled: true}

	loadedAgain, err := manager.GetConfig()
	if err != nil {
		t.Fatalf("GetConfig() second error = %v", err)
	}
	if loadedAgain.Agents.Profiles["default"].Workspace != tempDir {
		t.Fatalf("default workspace = %q, want %q", loadedAgain.Agents.Profiles["default"].Workspace, tempDir)
	}
	if len(loadedAgain.MCP.MCPServers) != 0 {
		t.Fatalf("MCP server count = %d, want 0", len(loadedAgain.MCP.MCPServers))
	}
}

func TestConfigManagerApplyOnboardUpdatePersistsNamedProfile(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.json")
	manager := NewConfigManager(configPath)

	updated, err := manager.ApplyOnboardUpdate(OnboardUpdate{
		ProfileName: "worker",
		Workspace:   filepath.Join(tempDir, "worker"),
		Provider:    "openrouter",
		Model:       "openai/gpt-4.1",
		APIKey:      "worker-token",
	})
	if err != nil {
		t.Fatalf("ApplyOnboardUpdate() error = %v", err)
	}

	worker, ok := updated.Agents.Profiles["worker"]
	if !ok {
		t.Fatal("worker profile missing after update")
	}
	if worker.Workspace != filepath.Join(tempDir, "worker") {
		t.Fatalf("worker workspace = %q, want %q", worker.Workspace, filepath.Join(tempDir, "worker"))
	}

	reloaded := NewConfigManager(configPath)
	cfg, err := reloaded.GetConfig()
	if err != nil {
		t.Fatalf("GetConfig() error = %v", err)
	}
	if cfg.Agents.Profiles["worker"].Provider != "openrouter" {
		t.Fatalf("worker provider = %q, want openrouter", cfg.Agents.Profiles["worker"].Provider)
	}
	for _, provider := range cfg.Providers {
		if provider.Name == "openrouter" && provider.Auth.Token != "worker-token" {
			t.Fatalf("openrouter token = %q, want worker-token", provider.Auth.Token)
		}
	}
}
