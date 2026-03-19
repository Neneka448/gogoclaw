package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveCodexProviderConfigLoadsCustomProviderFromCodexConfig(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.toml")
	t.Setenv("CODEX_CONFIG_PATH", configPath)
	if err := os.WriteFile(configPath, []byte(`model_provider = "custom"

[model_providers.custom]
base_url = "https://codex-proxy.example.com"
`), 0644); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	provider, err := resolveCodexProviderConfig(nil)
	if err != nil {
		t.Fatalf("resolveCodexProviderConfig() error = %v", err)
	}
	if provider.BaseURL != "https://codex-proxy.example.com" {
		t.Fatalf("provider.BaseURL = %q, want custom base url", provider.BaseURL)
	}
	if provider.Path != "v1/responses" {
		t.Fatalf("provider.Path = %q, want v1/responses", provider.Path)
	}
	if provider.Timeout != 60 {
		t.Fatalf("provider.Timeout = %d, want 60", provider.Timeout)
	}
}

func TestResolveCodexProviderConfigAllowsExplicitOverride(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.toml")
	t.Setenv("CODEX_CONFIG_PATH", configPath)
	if err := os.WriteFile(configPath, []byte(`model_provider = "custom"

[model_providers.custom]
base_url = "https://codex-proxy.example.com"
`), 0644); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	provider, err := resolveCodexProviderConfig(&ProviderConfig{Name: "codex", Timeout: 15, BaseURL: "https://override.example.com", Path: "custom/responses"})
	if err != nil {
		t.Fatalf("resolveCodexProviderConfig() error = %v", err)
	}
	if provider.BaseURL != "https://override.example.com" {
		t.Fatalf("provider.BaseURL = %q, want override", provider.BaseURL)
	}
	if provider.Path != "custom/responses" {
		t.Fatalf("provider.Path = %q, want custom/responses", provider.Path)
	}
	if provider.Timeout != 15 {
		t.Fatalf("provider.Timeout = %d, want 15", provider.Timeout)
	}
}

func TestResolveCodexProviderConfigFallsBackWhenSelectedProviderMissing(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.toml")
	t.Setenv("CODEX_CONFIG_PATH", configPath)
	if err := os.WriteFile(configPath, []byte(`model_provider = "custom"
`), 0644); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	provider, err := resolveCodexProviderConfig(nil)
	if err != nil {
		t.Fatalf("resolveCodexProviderConfig() error = %v", err)
	}
	if provider.BaseURL != "" {
		t.Fatalf("provider.BaseURL = %q, want empty", provider.BaseURL)
	}
	if provider.Path != "" {
		t.Fatalf("provider.Path = %q, want empty", provider.Path)
	}
}
