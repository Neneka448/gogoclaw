package bootstrap

import (
	"strings"
	"time"

	"github.com/Neneka448/gogoclaw/internal/channels"
	"github.com/Neneka448/gogoclaw/internal/config"
	"github.com/Neneka448/gogoclaw/internal/context"
	"github.com/Neneka448/gogoclaw/internal/gateway"
	messagebus "github.com/Neneka448/gogoclaw/internal/message_bus"
	"github.com/Neneka448/gogoclaw/internal/provider"
	"github.com/Neneka448/gogoclaw/internal/session"
	"github.com/Neneka448/gogoclaw/internal/skills"
	"github.com/Neneka448/gogoclaw/internal/systemprompt"
	"github.com/Neneka448/gogoclaw/internal/tools"
	"github.com/Neneka448/gogoclaw/internal/vectorstore"
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
	embeddingProfile, err := configManager.GetEmbeddingProfileConfig("default")
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
	textEmbeddingProvider, modalEmbeddingProvider, err := buildEmbeddingProviders(configManager, embeddingProfile)
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
		if err := channelRegistry.Register(channels.NewFeishuChannel(sysConfig.Channels.Feishu, messageBus, profile.Workspace)); err != nil {
			return nil, err
		}
	}

	context := context.SystemContext{
		MessageBus:      messageBus,
		Provider:        llmProvider,
		TextEmbedding:   textEmbeddingProvider,
		ModalEmbedding:  modalEmbeddingProvider,
		ConfigManager:   configManager,
		ToolRegistry:    tools.NewToolRegistry(),
		Skills:          skillRegistry,
		SystemPrompt:    systemprompt.NewService(profile.Workspace),
		ChannelRegistry: channelRegistry,
		SessionManager:  session.NewSessionManager(profile.Workspace),
		VectorStore:     vectorstore.NewSQLiteVecService(profile.Workspace, "default", *embeddingProfile),
	}
	if err := context.ToolRegistry.RegisterTool("read_file", tools.NewReadFileTool(profile.Workspace)); err != nil {
		return nil, err
	}
	if err := context.ToolRegistry.RegisterTool("list_dir", tools.NewListDirTool(profile.Workspace)); err != nil {
		return nil, err
	}
	if err := context.ToolRegistry.RegisterTool("terminal", tools.NewTerminalTool(profile.Workspace, resolveToolTimeout(sysConfig.Tools, "terminal", tools.DefaultTerminalTimeout()))); err != nil {
		return nil, err
	}
	if err := context.ToolRegistry.RegisterTool("message", tools.NewMessageTool(messageBus)); err != nil {
		return nil, err
	}
	if err := context.ToolRegistry.RegisterTool("get_skill", tools.NewGetSkillTool(skillRegistry)); err != nil {
		return nil, err
	}

	gateway := gateway.NewGateway(context)

	return &gateway, nil
}

func resolveToolTimeout(configs []config.ToolConfig, name string, defaultTimeout time.Duration) time.Duration {
	for _, toolConfig := range configs {
		if !strings.EqualFold(strings.TrimSpace(toolConfig.Name), name) {
			continue
		}
		if toolConfig.Timeout <= 0 {
			return defaultTimeout
		}
		return time.Duration(toolConfig.Timeout) * time.Second
	}

	return defaultTimeout
}

func buildEmbeddingProviders(configManager config.ConfigManager, profile *config.EmbeddingProfileConfig) (provider.EmbeddingProvider, provider.EmbeddingProvider, error) {
	if profile == nil {
		return nil, nil, nil
	}

	cache := map[string]provider.EmbeddingProvider{}
	textProvider, err := resolveEmbeddingProvider(configManager, cache, profile.Text.Provider)
	if err != nil {
		return nil, nil, err
	}
	modalProvider, err := resolveEmbeddingProvider(configManager, cache, profile.Modal.Provider)
	if err != nil {
		return nil, nil, err
	}

	return textProvider, modalProvider, nil
}

func resolveEmbeddingProvider(configManager config.ConfigManager, cache map[string]provider.EmbeddingProvider, providerName string) (provider.EmbeddingProvider, error) {
	providerName = strings.TrimSpace(providerName)
	if providerName == "" {
		return nil, nil
	}
	if embeddingProvider, ok := cache[providerName]; ok {
		return embeddingProvider, nil
	}

	providerConfig, err := configManager.GetEmbeddingProviderConfig(providerName)
	if err != nil {
		return nil, err
	}
	embeddingProvider, err := provider.NewEmbeddingProvider(providerConfig)
	if err != nil {
		return nil, err
	}
	cache[providerName] = embeddingProvider

	return embeddingProvider, nil
}
