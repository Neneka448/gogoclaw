package bootstrap

import (
	"fmt"
	"os"
	"path/filepath"
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
	"github.com/Neneka448/gogoclaw/internal/taskwatch"
	"github.com/Neneka448/gogoclaw/internal/utils"
)

func Bootstrap(configPath string) (*gateway.Gateway, error) {
	bootstrapStart := time.Now()
	utils.Perf("bootstrap: start")

	t0 := time.Now()
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
	utils.Perf("bootstrap: config loading took %s", time.Since(t0))

	t0 = time.Now()
	messageBus := messagebus.NewMessageBus()
	channelRegistry := channels.NewRegistry()
	if err := channelRegistry.Register(channels.NewCLIChannel(channelsConfig.CLI, nil)); err != nil {
		return nil, err
	}
	if channelsConfig.MQ.Enabled {
		if err := channelRegistry.Register(channels.NewMQChannel(channelsConfig.MQ, messageBus, defaultProfile.Workspace)); err != nil {
			return nil, err
		}
	}
	if channelsConfig.Feishu.Enabled {
		if err := channelRegistry.Register(channels.NewFeishuChannel(channelsConfig.Feishu, messageBus, defaultProfile.Workspace)); err != nil {
			return nil, err
		}
	}
	utils.Perf("bootstrap: channel registry took %s", time.Since(t0))

	t0 = time.Now()
	cronLocation, err := time.LoadLocation(strings.TrimSpace(cronConfig.Timezone))
	if err != nil {
		return nil, err
	}
	cronManager := cron.NewCronManager(cronLocation)

	var invoker appcontext.InvocationService
	var taskWatchRef taskwatch.Service
	cronService := cron.NewCronService(resolver, cronManager, func(request cron.ExecutionRequest) error {
		return executeCronRequest(invoker, taskWatchRef, request)
	}, cronLocation)
	utils.Perf("bootstrap: cron setup took %s", time.Since(t0))

	t0 = time.Now()
	taskWatchService := taskwatch.NewService(taskwatch.Options{
		Workspace:   defaultProfile.Workspace,
		MessageBus:  messageBus,
		CronService: cronService,
	})
	taskWatchRef = taskWatchService
	utils.Perf("bootstrap: taskwatch setup took %s", time.Since(t0))

	t0 = time.Now()
	invoker, err = agent.NewInvocationService(configManager, messageBus, channelRegistry, cronService, cronConfig.Enabled, taskWatchService, codexTokenProvider{})
	if err != nil {
		return nil, err
	}
	utils.Perf("bootstrap: invocation service took %s", time.Since(t0))

	t0 = time.Now()
	sysContext := appcontext.SystemContext{
		MessageBus:       messageBus,
		ConfigManager:    configManager,
		ChannelRegistry:  channelRegistry,
		CronService:      cronService,
		CronEnabled:      cronConfig.Enabled,
		TaskWatchService: taskWatchService,
		TaskWatchEnabled: true,
		Invoker:          invoker,
	}

	gateway, err := gateway.NewGateway(sysContext)
	if err != nil {
		return nil, err
	}
	utils.Perf("bootstrap: gateway creation took %s", time.Since(t0))

	utils.Perf("bootstrap: total took %s", time.Since(bootstrapStart))
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

func executeCronRequest(invoker appcontext.InvocationService, taskWatchService taskwatch.Service, request cron.ExecutionRequest) error {
	if invoker == nil {
		return fmt.Errorf("invoker is not initialized")
	}

	// Auto-register taskwatch for invocation task crons.
	autoRegisterInvocationWatch(taskWatchService, request)

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
		if err := appcontext.ValidateInvocationMode(request.Mode); err != nil {
			return err
		}
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

// autoRegisterInvocationWatch detects invocation task crons (cron_id matching
// "{inv-*}-task") and automatically registers a taskwatch entry so that
// completion/timeout is monitored without the SKILL needing to call
// register_task_watch explicitly.
func autoRegisterInvocationWatch(taskWatchService taskwatch.Service, request cron.ExecutionRequest) {
	if taskWatchService == nil {
		return
	}
	invocationID := taskwatch.ParseInvocationCronID(request.CronID)
	if invocationID == "" {
		return
	}

	// Derive workspace from ExecutionDir: {workspace}/crons/{cron_id}/{exec_id}/
	workspace := filepath.Dir(filepath.Dir(filepath.Dir(request.ExecutionDir)))
	invocationDir, err := taskwatch.ResolveInvocationDir(workspace, invocationID)
	if err != nil {
		// Not an invocation-managed cron, or manifest not yet written.
		return
	}

	manifest, err := taskwatch.ReadManifest(invocationDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[taskwatch] auto-register: read manifest %s: %v\n", invocationID, err)
		return
	}

	entry := taskwatch.WatchEntryFromManifest(manifest, invocationDir)
	if err := taskWatchService.Register(entry); err != nil {
		fmt.Fprintf(os.Stderr, "[taskwatch] auto-register %s: %v\n", invocationID, err)
	} else {
		fmt.Fprintf(os.Stderr, "[taskwatch] auto-registered watch for invocation %s (caller=%s)\n", invocationID, manifest.CallerProfile)
	}
}
