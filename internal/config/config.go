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

	getProviderConfig(providerName string) (*ProviderConfig, error)
}

type configManager struct {
	configPath  string
	configCache *SysConfig
}

func NewConfigManager(configPath string) ConfigManager {
	return &configManager{
		configPath: configPath,
	}
}

func (cm *configManager) GetConfig() (*SysConfig, error) {
	if cm.configCache == nil {
		if err := cm.readConfig(); err != nil {
			return nil, err
		}
	}
	return cm.configCache, nil
}

func (cm *configManager) readConfig() error {
	if _, err := os.Stat(cm.configPath); os.IsNotExist(err) {
		return fmt.Errorf("config file not exists: %s", cm.configPath)
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

	return nil
}

func (cm *configManager) writeConfig() error {
	return nil
}

func (cm *configManager) getProviderConfig(providerName string) (*ProviderConfig, error) {
	for i := range cm.configCache.Providers {
		if cm.configCache.Providers[i].Name == providerName {
			return &cm.configCache.Providers[i], nil
		}
	}
	return nil, fmt.Errorf("provider not found: %s", providerName)
}
