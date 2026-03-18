package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type ConfigManager interface {
	GetConfig() (SysConfig, error)
	PreviewOnboardUpdate(update OnboardUpdate) (SysConfig, error)
	ApplyOnboardUpdate(update OnboardUpdate) (SysConfig, error)

	GetProviderConfig(providerName string) (*ProviderConfig, error)
	GetAgentProfileConfig(profileName string) (*ProfileConfig, error)
	GetEmbeddingProviderConfig(providerName string) (*ProviderConfig, error)
	GetEmbeddingProfileConfig(profileName string) (*EmbeddingProfileConfig, error)
	ResolveEmbeddingProfile(profileName string) (string, *EmbeddingProfileConfig, error)
	GetChannelsConfig() (ChannelsConfig, error)
	GetGatewayConfig() (GatewayConfig, error)
	GetToolsConfig() ([]ToolConfig, error)
	GetMCPConfig() (MCPConfig, error)
	GetCronConfig() (CronConfig, error)
	GetMemoryConfig() (MemoryConfig, error)
	NewProfileResolver() (*ProfileResolver, error)
}

type configManager struct {
	configPath  string
	mu          sync.RWMutex
	configCache SysConfig
	loaded      bool
}

func NewConfigManager(configPath string) ConfigManager {
	return &configManager{
		configPath: configPath,
	}
}

func (cm *configManager) GetConfig() (SysConfig, error) {
	cm.mu.RLock()
	if cm.loaded {
		cfg := cloneSysConfig(cm.configCache)
		cm.mu.RUnlock()
		return cfg, nil
	}
	cm.mu.RUnlock()

	cm.mu.Lock()
	defer cm.mu.Unlock()
	if err := cm.ensureLoadedLocked(false); err != nil {
		return SysConfig{}, err
	}
	return cloneSysConfig(cm.configCache), nil
}

func (cm *configManager) applyConfigUpdate(fn func(*SysConfig) error) (SysConfig, error) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	if err := cm.ensureLoadedLocked(true); err != nil {
		return SysConfig{}, err
	}

	next := cloneSysConfig(cm.configCache)
	if err := fn(&next); err != nil {
		return SysConfig{}, err
	}
	if err := cm.persistLocked(next); err != nil {
		return SysConfig{}, err
	}
	return cloneSysConfig(cm.configCache), nil
}

func (cm *configManager) GetAgentProfileConfig(profileName string) (*ProfileConfig, error) {
	cfg, err := cm.GetConfig()
	if err != nil {
		return nil, err
	}
	name := normalizeProfileName(profileName)
	profile, ok := cfg.Agents.Profiles[name]
	if !ok {
		return nil, fmt.Errorf("profile not found: %s", name)
	}
	return &profile, nil
}

func (cm *configManager) GetEmbeddingProfileConfig(profileName string) (*EmbeddingProfileConfig, error) {
	cfg, err := cm.GetConfig()
	if err != nil {
		return nil, err
	}
	name := normalizeProfileName(profileName)
	profile, ok := cfg.Embedding.Profiles[name]
	if !ok {
		return nil, fmt.Errorf("embedding profile not found: %s", name)
	}
	return &profile, nil
}

func (cm *configManager) GetProviderConfig(providerName string) (*ProviderConfig, error) {
	cfg, err := cm.GetConfig()
	if err != nil {
		return nil, err
	}
	return findProviderConfig(cfg.Providers, providerName)
}

func (cm *configManager) GetEmbeddingProviderConfig(providerName string) (*ProviderConfig, error) {
	cfg, err := cm.GetConfig()
	if err != nil {
		return nil, err
	}
	return findProviderConfig(cfg.Embedding.Providers, providerName)
}

func (cm *configManager) ResolveEmbeddingProfile(profileName string) (string, *EmbeddingProfileConfig, error) {
	cfg, err := cm.GetConfig()
	if err != nil {
		return "", nil, err
	}
	profileName = normalizeProfileName(profileName)
	profile, ok := cfg.Agents.Profiles[profileName]
	if !ok {
		return "", nil, fmt.Errorf("profile not found: %s", profileName)
	}

	candidates := []string{}
	if explicit := strings.TrimSpace(profile.EmbeddingProfile); explicit != "" {
		candidates = append(candidates, explicit)
	}
	candidates = append(candidates, profileName, "default")

	seen := map[string]struct{}{}
	for _, candidate := range candidates {
		if _, ok := seen[candidate]; ok {
			continue
		}
		seen[candidate] = struct{}{}
		embeddingProfile, ok := cfg.Embedding.Profiles[candidate]
		if !ok {
			continue
		}
		profileCopy := embeddingProfile
		return candidate, &profileCopy, nil
	}
	return "", nil, fmt.Errorf("embedding profile not found for agent profile %s", profileName)
}

func (cm *configManager) GetChannelsConfig() (ChannelsConfig, error) {
	cfg, err := cm.GetConfig()
	if err != nil {
		return ChannelsConfig{}, err
	}
	return cloneChannelsConfig(cfg.Channels), nil
}

func (cm *configManager) GetGatewayConfig() (GatewayConfig, error) {
	cfg, err := cm.GetConfig()
	if err != nil {
		return GatewayConfig{}, err
	}
	return cfg.Gateway, nil
}

func (cm *configManager) GetToolsConfig() ([]ToolConfig, error) {
	cfg, err := cm.GetConfig()
	if err != nil {
		return nil, err
	}
	return cloneToolConfigs(cfg.Tools), nil
}

func (cm *configManager) GetMCPConfig() (MCPConfig, error) {
	cfg, err := cm.GetConfig()
	if err != nil {
		return MCPConfig{}, err
	}
	return cloneMCPConfig(cfg.MCP), nil
}

func (cm *configManager) GetCronConfig() (CronConfig, error) {
	cfg, err := cm.GetConfig()
	if err != nil {
		return CronConfig{}, err
	}
	return cfg.Cron, nil
}

func (cm *configManager) GetMemoryConfig() (MemoryConfig, error) {
	cfg, err := cm.GetConfig()
	if err != nil {
		return MemoryConfig{}, err
	}
	return cfg.Memory, nil
}

func (cm *configManager) NewProfileResolver() (*ProfileResolver, error) {
	cfg, err := cm.GetConfig()
	if err != nil {
		return nil, err
	}
	return NewProfileResolver(cfg.Agents.Profiles, "default"), nil
}

func (cm *configManager) ensureLoadedLocked(allowMissing bool) error {
	if cm.loaded {
		return nil
	}

	cfg, err := loadConfigFromDisk(cm.configPath, allowMissing)
	if err != nil {
		return err
	}
	cm.configCache = cfg
	cm.loaded = true
	return nil
}

func (cm *configManager) persistLocked(cfg SysConfig) error {
	normalizeSysConfig(&cfg)
	if err := validateSysConfig(cfg); err != nil {
		return err
	}
	if err := writeConfigAtomically(cm.configPath, cfg); err != nil {
		return err
	}
	cm.configCache = cloneSysConfig(cfg)
	cm.loaded = true
	return nil
}

func loadConfigFromDisk(configPath string, allowMissing bool) (SysConfig, error) {
	defaultConfig := CreateDefaultConfig()
	content, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) && allowMissing {
			normalizeSysConfig(&defaultConfig)
			if err := validateSysConfig(defaultConfig); err != nil {
				return SysConfig{}, err
			}
			return defaultConfig, nil
		}
		if os.IsNotExist(err) {
			return SysConfig{}, fmt.Errorf("config file not exists: %s", configPath)
		}
		return SysConfig{}, err
	}

	if err := json.Unmarshal(content, &defaultConfig); err != nil {
		return SysConfig{}, err
	}
	normalizeSysConfig(&defaultConfig)
	if err := validateSysConfig(defaultConfig); err != nil {
		return SysConfig{}, err
	}
	return defaultConfig, nil
}

func writeConfigAtomically(configPath string, cfg SysConfig) error {
	if err := os.MkdirAll(filepath.Dir(configPath), 0755); err != nil {
		return err
	}

	encoded, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	encoded = append(encoded, '\n')

	tempFile, err := os.CreateTemp(filepath.Dir(configPath), ".config-*.json")
	if err != nil {
		return err
	}
	tempPath := tempFile.Name()
	if _, err := tempFile.Write(encoded); err != nil {
		_ = tempFile.Close()
		_ = os.Remove(tempPath)
		return err
	}
	if err := tempFile.Chmod(0644); err != nil {
		_ = tempFile.Close()
		_ = os.Remove(tempPath)
		return err
	}
	if err := tempFile.Close(); err != nil {
		_ = os.Remove(tempPath)
		return err
	}
	if err := os.Rename(tempPath, configPath); err != nil {
		_ = os.Remove(tempPath)
		return err
	}
	return nil
}

func validateSysConfig(cfg SysConfig) error {
	if err := ValidateMemoryConfig(cfg.Memory); err != nil {
		return fmt.Errorf("invalid memory config: %w", err)
	}
	if _, err := time.LoadLocation(strings.TrimSpace(cfg.Cron.Timezone)); err != nil {
		return fmt.Errorf("invalid cron timezone %q: %w", cfg.Cron.Timezone, err)
	}

	seenProfiles := map[string]struct{}{}
	for name := range cfg.Agents.Profiles {
		trimmed := strings.TrimSpace(name)
		if trimmed == "" {
			return fmt.Errorf("profile name is required")
		}
		if _, exists := seenProfiles[trimmed]; exists {
			return fmt.Errorf("duplicate profile name: %s", trimmed)
		}
		seenProfiles[trimmed] = struct{}{}
	}

	if err := validateProviderNames(cfg.Providers, "provider"); err != nil {
		return err
	}
	if err := validateProviderNames(cfg.Embedding.Providers, "embedding provider"); err != nil {
		return err
	}
	if err := validateEmbeddingProfileReferences(cfg); err != nil {
		return err
	}
	if err := validateToolNames(cfg.Tools); err != nil {
		return err
	}
	return nil
}

func validateProviderNames(providers []ProviderConfig, label string) error {
	seen := map[string]struct{}{}
	for _, provider := range providers {
		name := strings.TrimSpace(provider.Name)
		if name == "" {
			return fmt.Errorf("%s name is required", label)
		}
		if _, exists := seen[name]; exists {
			return fmt.Errorf("duplicate %s name: %s", label, name)
		}
		seen[name] = struct{}{}
	}
	return nil
}

func validateEmbeddingProfileReferences(cfg SysConfig) error {
	for profileName, profile := range cfg.Agents.Profiles {
		explicit := strings.TrimSpace(profile.EmbeddingProfile)
		if explicit == "" {
			continue
		}
		if _, ok := cfg.Embedding.Profiles[explicit]; !ok {
			return fmt.Errorf("embedding profile %q referenced by agent profile %q not found", explicit, profileName)
		}
	}
	return nil
}

func validateToolNames(tools []ToolConfig) error {
	seen := map[string]struct{}{}
	for _, tool := range tools {
		name := strings.ToLower(strings.TrimSpace(tool.Name))
		if name == "" {
			return fmt.Errorf("tool name is required")
		}
		if _, exists := seen[name]; exists {
			return fmt.Errorf("duplicate tool name: %s", tool.Name)
		}
		seen[name] = struct{}{}
	}
	return nil
}

func normalizeSysConfig(cfg *SysConfig) {
	if cfg == nil {
		return
	}

	defaultConfig := CreateDefaultConfig()
	if cfg.Agents.Profiles == nil {
		cfg.Agents.Profiles = make(map[string]ProfileConfig)
	}
	if cfg.Embedding.Profiles == nil {
		cfg.Embedding.Profiles = make(map[string]EmbeddingProfileConfig)
	}
	if cfg.MCP.MCPServers == nil {
		cfg.MCP.MCPServers = make(map[string]MCPServerConfig)
	}
	if len(cfg.Embedding.Providers) == 0 {
		cfg.Embedding.Providers = cloneProviderConfigs(defaultConfig.Embedding.Providers)
	}
	if len(cfg.Providers) == 0 {
		cfg.Providers = cloneProviderConfigs(defaultConfig.Providers)
	}
	if len(cfg.Tools) == 0 && cfg.Tools == nil {
		cfg.Tools = []ToolConfig{}
	}
	if cfg.Channels.Feishu.AllowFrom == nil {
		cfg.Channels.Feishu.AllowFrom = append([]string(nil), defaultConfig.Channels.Feishu.AllowFrom...)
	}
	if strings.TrimSpace(cfg.Channels.Feishu.ReactEmoji) == "" {
		cfg.Channels.Feishu.ReactEmoji = defaultConfig.Channels.Feishu.ReactEmoji
	}
	if !cfg.Channels.CLI.Enabled {
		cfg.Channels.CLI.Enabled = defaultConfig.Channels.CLI.Enabled
	}
	if !cfg.Channels.SendProgress {
		cfg.Channels.SendProgress = defaultConfig.Channels.SendProgress
	}
	if !cfg.Channels.SendToolHints {
		cfg.Channels.SendToolHints = defaultConfig.Channels.SendToolHints
	}
	if cfg.Gateway.Port == 0 {
		cfg.Gateway.Port = defaultConfig.Gateway.Port
	}
	if strings.TrimSpace(cfg.Gateway.Host) == "" {
		cfg.Gateway.Host = defaultConfig.Gateway.Host
	}
	if cfg.Gateway.Heartbeat.Interval == 0 {
		cfg.Gateway.Heartbeat.Interval = defaultConfig.Gateway.Heartbeat.Interval
	}
	if !cfg.Gateway.Heartbeat.Enable {
		cfg.Gateway.Heartbeat.Enable = defaultConfig.Gateway.Heartbeat.Enable
	}
	if strings.TrimSpace(cfg.Cron.Timezone) == "" {
		cfg.Cron.Timezone = defaultConfig.Cron.Timezone
	}
	if !cfg.Cron.Enabled {
		cfg.Cron.Enabled = defaultConfig.Cron.Enabled
	}
	if cfg.Memory == (MemoryConfig{}) {
		cfg.Memory = defaultConfig.Memory
	}
	if _, ok := cfg.Embedding.Profiles["default"]; !ok {
		cfg.Embedding.Profiles["default"] = defaultConfig.Embedding.Profiles["default"]
	}
}

func findProviderConfig(providers []ProviderConfig, providerName string) (*ProviderConfig, error) {
	providerName = strings.TrimSpace(providerName)
	for i := range providers {
		if strings.TrimSpace(providers[i].Name) == providerName {
			provider := providers[i]
			return &provider, nil
		}
	}
	return nil, fmt.Errorf("provider not found: %s", providerName)
}

func normalizeProfileName(profileName string) string {
	profileName = strings.TrimSpace(profileName)
	if profileName == "" {
		return "default"
	}
	return profileName
}

func cloneSysConfig(cfg SysConfig) SysConfig {
	return SysConfig{
		Agents: AgentConfig{
			Profiles: cloneProfileConfigs(cfg.Agents.Profiles),
		},
		Embedding: EmbeddingConfig{
			Profiles:  cloneEmbeddingProfileConfigs(cfg.Embedding.Profiles),
			Providers: cloneProviderConfigs(cfg.Embedding.Providers),
		},
		Providers: cloneProviderConfigs(cfg.Providers),
		Channels:  cloneChannelsConfig(cfg.Channels),
		Gateway:   cfg.Gateway,
		Tools:     cloneToolConfigs(cfg.Tools),
		MCP:       cloneMCPConfig(cfg.MCP),
		Cron:      cfg.Cron,
		Memory:    cfg.Memory,
	}
}

func cloneProfileConfigs(src map[string]ProfileConfig) map[string]ProfileConfig {
	if src == nil {
		return nil
	}
	dst := make(map[string]ProfileConfig, len(src))
	for key, value := range src {
		dst[key] = value
	}
	return dst
}

func cloneEmbeddingProfileConfigs(src map[string]EmbeddingProfileConfig) map[string]EmbeddingProfileConfig {
	if src == nil {
		return nil
	}
	dst := make(map[string]EmbeddingProfileConfig, len(src))
	for key, value := range src {
		dst[key] = value
	}
	return dst
}

func cloneProviderConfigs(src []ProviderConfig) []ProviderConfig {
	if src == nil {
		return nil
	}
	dst := make([]ProviderConfig, len(src))
	for i, p := range src {
		dst[i] = p
		if p.Headers != nil {
			dst[i].Headers = make(map[string]string, len(p.Headers))
			for k, v := range p.Headers {
				dst[i].Headers[k] = v
			}
		}
		if p.ExtraBody != nil {
			dst[i].ExtraBody = cloneExtraBody(p.ExtraBody)
		}
	}
	return dst
}

func cloneExtraBody(src map[string]any) map[string]any {
	encoded, err := json.Marshal(src)
	if err != nil {
		dst := make(map[string]any, len(src))
		for k, v := range src {
			dst[k] = v
		}
		return dst
	}
	var dst map[string]any
	_ = json.Unmarshal(encoded, &dst)
	return dst
}

func cloneChannelsConfig(cfg ChannelsConfig) ChannelsConfig {
	cloned := cfg
	cloned.Feishu.AllowFrom = append([]string(nil), cfg.Feishu.AllowFrom...)
	return cloned
}

func cloneToolConfigs(src []ToolConfig) []ToolConfig {
	if src == nil {
		return nil
	}
	dst := make([]ToolConfig, len(src))
	copy(dst, src)
	return dst
}

func cloneMCPConfig(cfg MCPConfig) MCPConfig {
	cloned := MCPConfig{
		MCPServers: make(map[string]MCPServerConfig, len(cfg.MCPServers)),
	}
	for name, server := range cfg.MCPServers {
		serverCopy := server
		serverCopy.Args = append([]string(nil), server.Args...)
		if server.Env != nil {
			serverCopy.Env = make(map[string]string, len(server.Env))
			for key, value := range server.Env {
				serverCopy.Env[key] = value
			}
		}
		if server.Headers != nil {
			serverCopy.Headers = make(map[string]string, len(server.Headers))
			for key, value := range server.Headers {
				serverCopy.Headers[key] = value
			}
		}
		cloned.MCPServers[name] = serverCopy
	}
	return cloned
}
