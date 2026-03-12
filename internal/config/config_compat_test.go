package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestConfigManagerMergesChannelDefaultsForLegacyConfig(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	legacy := `{
		"agents":{"profiles":{"default":{"workspace":"/tmp/workspace","provider":"codex","model":"gpt-5.4","maxTokens":512,"temperature":0.1,"maxToolIterations":4,"memoryWindow":8,"maxRetryTimes":1}}},
		"providers":[{"name":"codex","timeout":60,"auth":{"token":"x"}}],
		"channels":{},
		"gateway":{"port":8080,"host":"127.0.0.1","heartbeat":{"interval":1800,"enable":true}},
		"tools":[]
	}`
	if err := os.WriteFile(configPath, []byte(legacy), 0644); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	manager := NewConfigManager(configPath)
	config, err := manager.GetConfig()
	if err != nil {
		t.Fatalf("GetConfig() error = %v", err)
	}
	if !config.Channels.CLI.Enabled {
		t.Fatal("config.Channels.CLI.Enabled = false, want true")
	}
	if len(config.Channels.Feishu.AllowFrom) != 1 || config.Channels.Feishu.AllowFrom[0] != "*" {
		t.Fatalf("config.Channels.Feishu.AllowFrom = %#v, want [*]", config.Channels.Feishu.AllowFrom)
	}
	if config.Channels.Feishu.ReactEmoji != "THUMBSUP" {
		t.Fatalf("config.Channels.Feishu.ReactEmoji = %q, want THUMBSUP", config.Channels.Feishu.ReactEmoji)
	}
	if !config.Channels.SendProgress || !config.Channels.SendToolHints {
		t.Fatalf("progress flags = (%v, %v), want true,true", config.Channels.SendProgress, config.Channels.SendToolHints)
	}
}
