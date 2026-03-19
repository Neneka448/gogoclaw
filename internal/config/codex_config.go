package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

type codexCLIConfig struct {
	ModelProvider  string                           `toml:"model_provider"`
	ModelProviders map[string]codexCLIModelProvider `toml:"model_providers"`
}

type codexCLIModelProvider struct {
	BaseURL string `toml:"base_url"`
}

func resolveCodexProviderConfig(explicit *ProviderConfig) (*ProviderConfig, error) {
	resolved := ProviderConfig{Name: "codex", Timeout: 60}
	if defaults, err := loadCodexProviderDefaults(); err != nil {
		return nil, err
	} else if defaults != nil {
		resolved = mergeProviderConfig(resolved, *defaults)
	}
	if explicit != nil {
		resolved = mergeProviderConfig(resolved, *explicit)
	}
	if strings.TrimSpace(resolved.Name) == "" {
		resolved.Name = "codex"
	}
	if resolved.Timeout <= 0 {
		resolved.Timeout = 60
	}
	return &resolved, nil
}

func loadCodexProviderDefaults() (*ProviderConfig, error) {
	configPath, err := codexConfigPath()
	if err != nil {
		return nil, err
	}
	raw, err := os.ReadFile(configPath)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read codex config %q: %w", configPath, err)
	}

	var cfg codexCLIConfig
	if _, err := toml.Decode(string(raw), &cfg); err != nil {
		return nil, fmt.Errorf("parse codex config %q: %w", configPath, err)
	}

	modelProvider := strings.TrimSpace(cfg.ModelProvider)
	if modelProvider == "" {
		return nil, nil
	}
	provider, ok := cfg.ModelProviders[modelProvider]
	if !ok {
		return nil, nil
	}

	resolved := &ProviderConfig{Name: "codex", Timeout: 60}
	resolved.BaseURL = strings.TrimSpace(provider.BaseURL)
	if resolved.BaseURL == "" {
		return nil, nil
	}
	resolved.Path = "v1/responses"

	return resolved, nil
}

func codexConfigPath() (string, error) {
	if override := strings.TrimSpace(os.Getenv("CODEX_CONFIG_PATH")); override != "" {
		return override, nil
	}
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory: %w", err)
	}
	return filepath.Join(homeDir, ".codex", "config.toml"), nil
}

func mergeProviderConfig(base ProviderConfig, override ProviderConfig) ProviderConfig {
	merged := base
	if strings.TrimSpace(override.Name) != "" {
		merged.Name = strings.TrimSpace(override.Name)
	}
	if override.Timeout > 0 {
		merged.Timeout = override.Timeout
	}
	if strings.TrimSpace(override.BaseURL) != "" {
		merged.BaseURL = strings.TrimSpace(override.BaseURL)
	}
	if strings.TrimSpace(override.Path) != "" {
		merged.Path = strings.TrimSpace(override.Path)
	}
	if strings.TrimSpace(override.Auth.Token) != "" {
		merged.Auth = override.Auth
	}
	if len(override.Headers) > 0 {
		merged.Headers = override.Headers
	}
	if len(override.ExtraBody) > 0 {
		merged.ExtraBody = override.ExtraBody
	}
	return merged
}
