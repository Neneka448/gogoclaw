package bootstrap

import (
	"github.com/Neneka448/gogoclaw/internal/config"
	"github.com/Neneka448/gogoclaw/internal/context"
	"github.com/Neneka448/gogoclaw/internal/gateway"
	messagebus "github.com/Neneka448/gogoclaw/internal/message_bus"
	"github.com/Neneka448/gogoclaw/internal/provider"
	"github.com/Neneka448/gogoclaw/internal/session"
	"github.com/Neneka448/gogoclaw/internal/tools"
)

func Bootstrap(configPath string) (*gateway.Gateway, error) {
	configManager := config.NewConfigManager(configPath)
	profile, err := configManager.GetAgentProfileConfig("default")
	if err != nil {
		return nil, err
	}
	providerConfig, err := configManager.GetProviderConfig(profile.Provider)
	if err != nil {
		return nil, err
	}
	llmProvider, err := provider.NewOpenAICompatibleProvider(providerConfig)
	if err != nil {
		return nil, err
	}

	context := context.SystemContext{
		MessageBus:     messagebus.NewMessageBus(),
		Provider:       llmProvider,
		ConfigManager:  configManager,
		ToolRegistry:   tools.NewToolRegistry(),
		SessionManager: session.NewSessionManager(profile.Workspace),
	}

	gateway := gateway.NewGateway(context)

	return &gateway, nil
}
