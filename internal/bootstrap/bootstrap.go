package bootstrap

import (
	"fmt"
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
)

func Bootstrap(configPath string) (*gateway.Gateway, error) {
	configManager := config.NewConfigManager(configPath)
	sysConfig, err := configManager.GetConfig()
	if err != nil {
		return nil, err
	}
	defaultProfile, err := configManager.GetAgentProfileConfig("default")
	if err != nil {
		return nil, err
	}
	profileWorkspaces := make(map[string]string, len(sysConfig.Agents.Profiles))
	for profileName, profile := range sysConfig.Agents.Profiles {
		profileWorkspaces[profileName] = profile.Workspace
	}
	messageBus := messagebus.NewMessageBus()
	channelRegistry := channels.NewRegistry()
	if err := channelRegistry.Register(channels.NewCLIChannel(sysConfig.Channels.CLI, nil)); err != nil {
		return nil, err
	}
	if sysConfig.Channels.Feishu.Enabled {
		if err := channelRegistry.Register(channels.NewFeishuChannel(sysConfig.Channels.Feishu, messageBus, defaultProfile.Workspace)); err != nil {
			return nil, err
		}
	}
	cronLocation, err := time.LoadLocation(strings.TrimSpace(sysConfig.Cron.Timezone))
	if err != nil {
		return nil, err
	}
	cronManager := cron.NewCronManager(cronLocation)

	var invoker appcontext.InvocationService
	cronService := cron.NewMultiProfileService(profileWorkspaces, "default", cronManager, func(request cron.ExecutionRequest) error {
		return executeCronRequest(invoker, request)
	}, cronLocation)
	invoker, err = agent.NewInvocationService(configManager, sysConfig, messageBus, channelRegistry, cronService, sysConfig.Cron.Enabled)
	if err != nil {
		return nil, err
	}

	sysContext := appcontext.SystemContext{
		MessageBus:      messageBus,
		ConfigManager:   configManager,
		ChannelRegistry: channelRegistry,
		CronService:     cronService,
		CronEnabled:     sysConfig.Cron.Enabled,
		Invoker:         invoker,
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
