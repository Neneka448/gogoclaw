package config

import (
	"encoding/json"
	"fmt"
	"os"
)

type ConfigManager interface {
	GetConfig() (*SysConfig, error)
	readConfig() error
	writeConfig() error

	GetProviderConfig(providerName string) (*ProviderConfig, error)
	GetAgentProfileConfig(profileName string) (*ProfileConfig, error)
}

type configManager struct {
	configPath  string
	configCache *SysConfig
	loaded      bool
}

func NewConfigManager(configPath string) ConfigManager {
	return &configManager{
		configPath: configPath,
	}
}

func (cm *configManager) GetConfig() (*SysConfig, error) {
	if !cm.loaded {
		if err := cm.readConfig(); err != nil {
			return nil, err
		}
	}
	return cm.configCache, nil
}

func (cm *configManager) GetAgentProfileConfig(profileName string) (*ProfileConfig, error) {
	config, err := cm.GetConfig()
	if err != nil {
		return nil, err
	}
	profile, ok := config.Agents.Profiles[profileName]
	if !ok {
		return nil, fmt.Errorf("profile not found: %s", profileName)
	}
	return &profile, nil
}

func (cm *configManager) readConfig() error {
	if _, err := os.Stat(cm.configPath); os.IsNotExist(err) {
		return fmt.Errorf("config file not exists: %s", cm.configPath)
	}
	if cm.configCache == nil {
		cm.configCache = &SysConfig{}
	}
	configFile, err := os.Open(cm.configPath)
	if err != nil {
		return err
	}
	defer configFile.Close()

	decoder := json.NewDecoder(configFile)
	if err := decoder.Decode(cm.configCache); err != nil {
		return err
	}
	cm.loaded = true

	return nil
}

func (cm *configManager) writeConfig() error {
	return nil
}

func (cm *configManager) GetProviderConfig(providerName string) (*ProviderConfig, error) {
	config, err := cm.GetConfig()
	if err != nil {
		return nil, err
	}
	for i := range config.Providers {
		if config.Providers[i].Name == providerName {
			return &config.Providers[i], nil
		}
	}
	return nil, fmt.Errorf("provider not found: %s", providerName)
}
