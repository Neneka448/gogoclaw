package bootstrap

import (
	"strings"
	"time"

	"github.com/Neneka448/gogoclaw/internal/agent"
	"github.com/Neneka448/gogoclaw/internal/channels"
	"github.com/Neneka448/gogoclaw/internal/config"
	appcontext "github.com/Neneka448/gogoclaw/internal/context"
	"github.com/Neneka448/gogoclaw/internal/cron"
	"github.com/Neneka448/gogoclaw/internal/gateway"
	mcppkg "github.com/Neneka448/gogoclaw/internal/mcp"
	messagebus "github.com/Neneka448/gogoclaw/internal/message_bus"
	"github.com/Neneka448/gogoclaw/internal/provider"
	"github.com/Neneka448/gogoclaw/internal/session"
	"github.com/Neneka448/gogoclaw/internal/skills"
	"github.com/Neneka448/gogoclaw/internal/systemprompt"
	"github.com/Neneka448/gogoclaw/internal/tools"
	"github.com/Neneka448/gogoclaw/internal/vectorstore"
	workspacepkg "github.com/Neneka448/gogoclaw/internal/workspace"
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
	if err := workspacepkg.EnsureDefaultSkills(profile.Workspace); err != nil {
		return nil, err
	}
	skillRegistry, err := skills.LoadWorkspaceSkills(profile.Workspace)
	if err != nil {
		return nil, err
	}
	mcpService, err := mcppkg.NewService(profile.Workspace, sysConfig.MCP, mcppkg.Options{FailFast: true})
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
	cronLocation, err := time.LoadLocation(strings.TrimSpace(sysConfig.Cron.Timezone))
	if err != nil {
		return nil, err
	}
	cronManager := cron.NewCronManager(cronLocation)

	var sysContext appcontext.SystemContext
	cronService := cron.NewCronService(profile.Workspace, cronManager, func(request cron.ExecutionRequest) error {
		return executeCronRequest(sysContext, sysConfig, profile.Workspace, skillRegistry, request)
	}, cronLocation)

	sysContext = appcontext.SystemContext{
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
		CronService:     cronService,
		CronEnabled:     sysConfig.Cron.Enabled,
		MCPService:      mcpService,
	}
	if err := registerTools(sysContext.ToolRegistry, profile.Workspace, sysConfig, skillRegistry, messageBus, mcpService); err != nil {
		_ = mcpService.Close()
		return nil, err
	}

	gateway := gateway.NewGateway(sysContext)

	return &gateway, nil
}

func BootstrapMCPService(configPath string, failFast bool) (mcppkg.Service, error) {
	configManager := config.NewConfigManager(configPath)
	sysConfig, err := configManager.GetConfig()
	if err != nil {
		return nil, err
	}
	profile, err := configManager.GetAgentProfileConfig("default")
	if err != nil {
		return nil, err
	}
	return mcppkg.NewService(profile.Workspace, sysConfig.MCP, mcppkg.Options{FailFast: failFast})
}

func registerBuiltinTools(registry tools.ToolRegistry, workspace string, sysConfig *config.SysConfig, skillRegistry skills.Registry, bus messagebus.MessageBus) error {
	if err := registry.RegisterTool("read_file", tools.NewReadFileTool(workspace)); err != nil {
		return err
	}
	if err := registry.RegisterTool("list_dir", tools.NewListDirTool(workspace)); err != nil {
		return err
	}
	if err := registry.RegisterTool("terminal", tools.NewTerminalTool(workspace, resolveToolTimeout(sysConfig.Tools, "terminal", tools.DefaultTerminalTimeout()))); err != nil {
		return err
	}
	if err := registry.RegisterTool("message", tools.NewMessageTool(bus)); err != nil {
		return err
	}
	if err := registry.RegisterTool("get_skill", tools.NewGetSkillTool(skillRegistry)); err != nil {
		return err
	}
	return nil
}

func registerTools(registry tools.ToolRegistry, workspace string, sysConfig *config.SysConfig, skillRegistry skills.Registry, bus messagebus.MessageBus, mcpService mcppkg.Service) error {
	if err := registerBuiltinTools(registry, workspace, sysConfig, skillRegistry, bus); err != nil {
		return err
	}
	if mcpService == nil {
		return nil
	}
	for _, descriptor := range mcpService.ToolDescriptors() {
		if err := registry.RegisterTool(descriptor.Name, descriptor); err != nil {
			return err
		}
	}
	return nil
}

func executeCronRequest(baseContext appcontext.SystemContext, sysConfig *config.SysConfig, workspace string, skillRegistry skills.Registry, request cron.ExecutionRequest) error {
	tempBus := messagebus.NewMessageBus()
	defer tempBus.Close()
	tempTools := tools.NewToolRegistry()
	if err := registerTools(tempTools, workspace, sysConfig, skillRegistry, tempBus, baseContext.MCPService); err != nil {
		return err
	}
	cronContext := baseContext
	cronContext.MessageBus = tempBus
	cronContext.ToolRegistry = tempTools
	cronContext.ChannelRegistry = nil

	message := messagebus.Message{
		ChannelID:   "cron",
		ChatID:      strings.TrimPrefix(request.SessionID, "cron:"),
		SenderID:    request.CronID,
		MessageType: "cron",
		Message:     request.Prompt,
		Metadata:    request.Metadata,
	}
	return agent.NewAgentLoop(cronContext).ProcessMessage(message)
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
