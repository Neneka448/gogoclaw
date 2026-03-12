package bootstrap

import (
	"github.com/Neneka448/gogoclaw/internal/channels"
	"github.com/Neneka448/gogoclaw/internal/config"
	"github.com/Neneka448/gogoclaw/internal/context"
	"github.com/Neneka448/gogoclaw/internal/gateway"
	messagebus "github.com/Neneka448/gogoclaw/internal/message_bus"
	"github.com/Neneka448/gogoclaw/internal/provider"
	"github.com/Neneka448/gogoclaw/internal/session"
	"github.com/Neneka448/gogoclaw/internal/skills"
	"github.com/Neneka448/gogoclaw/internal/tools"
)

func Bootstrap(configPath string) (*gateway.Gateway, error) {
	configManager := config.NewConfigManager(configPath)
	sysConfig, err := configManager.GetConfig()
	if err != nil {
		return nil, err
	}
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
	skillRegistry, err := skills.LoadWorkspaceSkills(profile.Workspace)
	if err != nil {
		return nil, err
	}
	messageBus := messagebus.NewMessageBus()
	channelRegistry := channels.NewRegistry()
	if err := channelRegistry.Register(channels.NewCLIChannel(sysConfig.Channels.CLI, nil)); err != nil {
		return nil, err
	}
	if sysConfig.Channels.Feishu.Enabled {
		if err := channelRegistry.Register(channels.NewFeishuChannel(sysConfig.Channels.Feishu, messageBus)); err != nil {
			return nil, err
		}
	}

	context := context.SystemContext{
		MessageBus:      messageBus,
		Provider:        llmProvider,
		ConfigManager:   configManager,
		ToolRegistry:    tools.NewToolRegistry(),
		Skills:          skillRegistry,
		ChannelRegistry: channelRegistry,
		SessionManager:  session.NewSessionManager(profile.Workspace),
	}
	if err := context.ToolRegistry.RegisterTool("read_file", tools.NewReadFileTool(profile.Workspace)); err != nil {
		return nil, err
	}
	if err := context.ToolRegistry.RegisterTool("list_dir", tools.NewListDirTool(profile.Workspace)); err != nil {
		return nil, err
	}
	if err := context.ToolRegistry.RegisterTool("get_skill", tools.NewGetSkillTool(skillRegistry)); err != nil {
		return nil, err
	}

	gateway := gateway.NewGateway(context)

	return &gateway, nil
}
