package bootstrap

import (
	"fmt"
	"strings"
	"time"

	"github.com/Neneka448/gogoclaw/internal/agent"
	"github.com/Neneka448/gogoclaw/internal/channels"
	cliauth "github.com/Neneka448/gogoclaw/internal/cli/auth"
	"github.com/Neneka448/gogoclaw/internal/config"
	appcontext "github.com/Neneka448/gogoclaw/internal/context"
	"github.com/Neneka448/gogoclaw/internal/cron"
	"github.com/Neneka448/gogoclaw/internal/gateway"
	mcppkg "github.com/Neneka448/gogoclaw/internal/mcp"
	messagebus "github.com/Neneka448/gogoclaw/internal/message_bus"
)

func Bootstrap(configPath string) (*gateway.Gateway, error) {
	configManager := config.NewConfigManager(configPath)
	channelsConfig, err := configManager.GetChannelsConfig()
	if err != nil {
		return nil, err
	}
	cronConfig, err := configManager.GetCronConfig()
	if err != nil {
		return nil, err
	}
	defaultProfile, err := configManager.GetAgentProfileConfig("default")
	if err != nil {
		return nil, err
	}
	resolver, err := configManager.NewProfileResolver()
	if err != nil {
		return nil, err
	}
	messageBus := messagebus.NewMessageBus()
	channelRegistry := channels.NewRegistry()
	if err := channelRegistry.Register(channels.NewCLIChannel(channelsConfig.CLI, nil)); err != nil {
		return nil, err
	}
	if channelsConfig.Feishu.Enabled {
		if err := channelRegistry.Register(channels.NewFeishuChannel(channelsConfig.Feishu, messageBus, defaultProfile.Workspace)); err != nil {
			return nil, err
		}
	}
	cronLocation, err := time.LoadLocation(strings.TrimSpace(cronConfig.Timezone))
	if err != nil {
		return nil, err
	}
	cronManager := cron.NewCronManager(cronLocation)

	var invoker appcontext.InvocationService
	cronService := cron.NewCronService(resolver, cronManager, func(request cron.ExecutionRequest) error {
		return executeCronRequest(invoker, request)
	}, cronLocation)
	invoker, err = agent.NewInvocationService(configManager, messageBus, channelRegistry, cronService, cronConfig.Enabled, codexTokenProvider{})
	if err != nil {
		return nil, err
	}

	sysContext := appcontext.SystemContext{
		MessageBus:      messageBus,
		ConfigManager:   configManager,
		ChannelRegistry: channelRegistry,
		CronService:     cronService,
		CronEnabled:     cronConfig.Enabled,
		Invoker:         invoker,
	}

	gateway := gateway.NewGateway(sysContext)

	return &gateway, nil
}

type codexTokenProvider struct{}

func (codexTokenProvider) GetToken() (string, string, error) {
	token, err := cliauth.GetCodexToken()
	if err != nil {
		return "", "", err
	}
	return token.Access, token.AccountID, nil
}

func BootstrapMCPService(configPath string, failFast bool) (mcppkg.Service, error) {
	configManager := config.NewConfigManager(configPath)
	mcpConfig, err := configManager.GetMCPConfig()
	if err != nil {
		return nil, err
	}
	profile, err := configManager.GetAgentProfileConfig("default")
	if err != nil {
		return nil, err
	}
	return mcppkg.NewService(profile.Workspace, mcpConfig, mcppkg.Options{FailFast: failFast})
}

func executeCronRequest(invoker appcontext.InvocationService, request cron.ExecutionRequest) error {
	if invoker == nil {
		return fmt.Errorf("invoker is not initialized")
	}
	tempBus := messagebus.NewMessageBus()
	defer tempBus.Close()
	metadata := request.Metadata
	if metadata == nil {
		metadata = make(map[string]string, 2)
	}
	if strings.TrimSpace(request.ProfileName) != "" {
		metadata["agent_profile"] = strings.TrimSpace(request.ProfileName)
	}
	if strings.TrimSpace(request.Mode) != "" {
		metadata["invocation_mode"] = strings.TrimSpace(request.Mode)
	}

	message := messagebus.Message{
		ChannelID:   "cron",
		ChatID:      strings.TrimPrefix(request.SessionID, "cron:"),
		SenderID:    request.CronID,
		MessageType: "cron",
		Message:     request.Prompt,
		Metadata:    metadata,
	}
	mode := appcontext.InvocationModeCron
	if strings.TrimSpace(request.Mode) != "" {
		mode = appcontext.InvocationMode(request.Mode)
	}
	return invoker.Invoke(appcontext.InvocationRequest{
		ProfileName: request.ProfileName,
		Message:     message,
		Mode:        mode,
		Overrides: appcontext.InvocationOverrides{
			MessageBus:             tempBus,
			ReplaceMessageBus:      true,
			ChannelRegistry:        nil,
			ReplaceChannelRegistry: true,
		},
	})
}
