package config

import (
	"fmt"
	"path/filepath"
	"strings"
)

type OnboardUpdate struct {
	ProfileName string
	Workspace   string
	Provider    string
	Model       string
	APIKey      string
}

func (cm *configManager) PreviewOnboardUpdate(update OnboardUpdate) (SysConfig, error) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	if err := cm.ensureLoadedLocked(true); err != nil {
		return SysConfig{}, err
	}

	next := cloneSysConfig(cm.configCache)
	if err := applyOnboardUpdate(&next, update); err != nil {
		return SysConfig{}, err
	}
	normalizeSysConfig(&next)
	if err := validateSysConfig(next); err != nil {
		return SysConfig{}, err
	}
	return cloneSysConfig(next), nil
}

func (cm *configManager) ApplyOnboardUpdate(update OnboardUpdate) (SysConfig, error) {
	return cm.applyConfigUpdate(func(cfg *SysConfig) error {
		return applyOnboardUpdate(cfg, update)
	})
}

func applyOnboardUpdate(cfg *SysConfig, update OnboardUpdate) error {
	if cfg == nil {
		return fmt.Errorf("config is required")
	}
	normalizeSysConfig(cfg)

	profileName := normalizeProfileName(update.ProfileName)
	if err := ensureWorkspaceConflict(*cfg, profileName, update.Workspace); err != nil {
		return err
	}
	if cfg.Agents.Profiles == nil {
		cfg.Agents.Profiles = make(map[string]ProfileConfig)
	}

	profile, ok := cfg.Agents.Profiles[profileName]
	if !ok {
		profile = CreateDefaultConfig().Agents.Profiles["default"]
	}
	profile.Workspace = update.Workspace
	profile.Provider = update.Provider
	profile.Model = update.Model
	cfg.Agents.Profiles[profileName] = profile

	provider := strings.TrimSpace(update.Provider)
	if provider == "" {
		return nil
	}
	// Codex uses OAuth from ~/.codex/auth.json; no provider config entry needed.
	if provider == "codex" {
		return nil
	}
	if strings.TrimSpace(update.APIKey) == "" {
		return nil
	}
	for i := range cfg.Providers {
		if strings.TrimSpace(cfg.Providers[i].Name) != provider {
			continue
		}
		cfg.Providers[i].Auth.Token = update.APIKey
		return nil
	}
	return fmt.Errorf("provider not found: %s", provider)
}

func ensureWorkspaceConflict(cfg SysConfig, targetProfile string, workspace string) error {
	targetWorkspace := canonicalOnboardWorkspacePath(workspace)
	for profileName, profile := range cfg.Agents.Profiles {
		if normalizeProfileName(profileName) == normalizeProfileName(targetProfile) {
			continue
		}
		if canonicalOnboardWorkspacePath(profile.Workspace) == targetWorkspace {
			return fmt.Errorf("workspace already used by profile %s: %s", profileName, workspace)
		}
	}
	return nil
}

func canonicalOnboardWorkspacePath(path string) string {
	if abs, err := filepath.Abs(path); err == nil {
		return filepath.Clean(abs)
	}
	return filepath.Clean(path)
}
