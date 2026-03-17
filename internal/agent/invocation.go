package agent

import (
	"fmt"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"github.com/Neneka448/gogoclaw/internal/channels"
	"github.com/Neneka448/gogoclaw/internal/config"
	appcontext "github.com/Neneka448/gogoclaw/internal/context"
	"github.com/Neneka448/gogoclaw/internal/cron"
	mcppkg "github.com/Neneka448/gogoclaw/internal/mcp"
	"github.com/Neneka448/gogoclaw/internal/memory"
	messagebus "github.com/Neneka448/gogoclaw/internal/message_bus"
	"github.com/Neneka448/gogoclaw/internal/provider"
	"github.com/Neneka448/gogoclaw/internal/session"
	"github.com/Neneka448/gogoclaw/internal/skills"
	"github.com/Neneka448/gogoclaw/internal/systemprompt"
	"github.com/Neneka448/gogoclaw/internal/tools"
	"github.com/Neneka448/gogoclaw/internal/vectorstore"
	workspacepkg "github.com/Neneka448/gogoclaw/internal/workspace"
)

const defaultAgentProfileName = "default"

type invocationService struct {
	configManager           config.ConfigManager
	sysConfig               *config.SysConfig
	defaultMessageBus       messagebus.MessageBus
	defaultChannelRegistry  channels.Registry
	cronService             cron.Service
	cronEnabled             bool
	mu                      sync.Mutex
	runtimes                map[string]*profileRuntime
}

type profileRuntime struct {
	context               appcontext.SystemContext
	profileName           string
	profile               config.ProfileConfig
	embeddingProfileName  string
	embeddingProfile      config.EmbeddingProfileConfig
	workspace             string
	skillRegistry         skills.Registry
	sysConfig             *config.SysConfig
	configManager         config.ConfigManager
	defaultMessageBus     messagebus.MessageBus
	defaultChannelRegistry channels.Registry
	cronService           cron.Service
	cronEnabled           bool
	startOnce             sync.Once
	startErr              error
}

func NewInvocationService(configManager config.ConfigManager, sysConfig *config.SysConfig, defaultMessageBus messagebus.MessageBus, defaultChannelRegistry channels.Registry, cronService cron.Service, cronEnabled bool) (appcontext.InvocationService, error) {
	if configManager == nil {
		return nil, fmt.Errorf("config manager is required")
	}
	if sysConfig == nil {
		return nil, fmt.Errorf("system config is required")
	}
	return &invocationService{
		configManager:          configManager,
		sysConfig:              sysConfig,
		defaultMessageBus:      defaultMessageBus,
		defaultChannelRegistry: defaultChannelRegistry,
		cronService:            cronService,
		cronEnabled:            cronEnabled,
		runtimes:               make(map[string]*profileRuntime),
	}, nil
}

func (service *invocationService) EnsureProfile(profileName string) error {
	runtime, err := service.getProfileRuntime(profileName)
	if err != nil {
		return err
	}
	return runtime.ensureReady()
}

func (service *invocationService) Invoke(request appcontext.InvocationRequest) error {
	executionContext, err := service.buildExecutionContext(request)
	if err != nil {
		return err
	}
	if err := ensureInvocationSession(executionContext, request.Message); err != nil {
		return err
	}
	if strings.TrimSpace(request.Message.Message) == "" {
		return nil
	}
	return NewAgentLoop(executionContext).ProcessMessage(request.Message)
}

func (service *invocationService) InvokeAsync(request appcontext.InvocationRequest) (<-chan error, error) {
	executionContext, err := service.buildExecutionContext(request)
	if err != nil {
		return nil, err
	}
	errCh := make(chan error, 1)
	if err := ensureInvocationSession(executionContext, request.Message); err != nil {
		errCh <- err
		return errCh, nil
	}
	if strings.TrimSpace(request.Message.Message) == "" {
		errCh <- nil
		return errCh, nil
	}
	go func() {
		defer func() {
			if r := recover(); r != nil {
				errCh <- fmt.Errorf("agent loop panic: %v\n%s", r, debug.Stack())
			}
		}()
		errCh <- NewAgentLoop(executionContext).ProcessMessage(request.Message)
	}()
	return errCh, nil
}

func (service *invocationService) Close() error {
	service.mu.Lock()
	runtimes := make([]*profileRuntime, 0, len(service.runtimes))
	for _, runtime := range service.runtimes {
		runtimes = append(runtimes, runtime)
	}
	service.runtimes = make(map[string]*profileRuntime)
	service.mu.Unlock()

	var firstErr error
	for _, runtime := range runtimes {
		if err := runtime.close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func (service *invocationService) buildExecutionContext(request appcontext.InvocationRequest) (appcontext.SystemContext, error) {
	runtime, err := service.getProfileRuntime(request.ProfileName)
	if err != nil {
		return appcontext.SystemContext{}, err
	}
	if err := runtime.ensureReady(); err != nil {
		return appcontext.SystemContext{}, err
	}
	messageBus := runtime.defaultMessageBus
	if request.Overrides.ReplaceMessageBus {
		messageBus = request.Overrides.MessageBus
	}
	channelRegistry := runtime.defaultChannelRegistry
	if request.Overrides.ReplaceChannelRegistry {
		channelRegistry = request.Overrides.ChannelRegistry
	}
	toolRegistry, err := buildInvocationToolRegistry(runtime.workspace, runtime.sysConfig, runtime.skillRegistry, messageBus, runtime.cronService, runtime.context.MCPService, runtime.context.MemoryService)
	if err != nil {
		return appcontext.SystemContext{}, err
	}

	executionContext := runtime.context
	executionContext.MessageBus = messageBus
	executionContext.ChannelRegistry = channelRegistry
	executionContext.ToolRegistry = toolRegistry
	executionContext.Runtime = appcontext.RuntimeContext{
		ProfileName:          runtime.profileName,
		Profile:              runtime.profile,
		EmbeddingProfileName: runtime.embeddingProfileName,
		EmbeddingProfile:     runtime.embeddingProfile,
		Workspace:            runtime.workspace,
		InvocationMode:       normalizeInvocationMode(request.Mode),
	}
	return executionContext, nil
}

func (service *invocationService) getProfileRuntime(profileName string) (*profileRuntime, error) {
	resolvedProfileName := normalizeProfileName(profileName)
	service.mu.Lock()
	if runtime, ok := service.runtimes[resolvedProfileName]; ok {
		service.mu.Unlock()
		return runtime, nil
	}
	service.mu.Unlock()

	runtime, err := service.buildProfileRuntime(resolvedProfileName)
	if err != nil {
		return nil, err
	}

	service.mu.Lock()
	defer service.mu.Unlock()
	if existing, ok := service.runtimes[resolvedProfileName]; ok {
		_ = runtime.close()
		return existing, nil
	}
	service.runtimes[resolvedProfileName] = runtime
	return runtime, nil
}

func (service *invocationService) buildProfileRuntime(profileName string) (*profileRuntime, error) {
	profile, err := service.configManager.GetAgentProfileConfig(profileName)
	if err != nil {
		return nil, err
	}
	workspace := strings.TrimSpace(profile.Workspace)
	if err := workspacepkg.EnsureMemorySkill(workspace); err != nil {
		return nil, err
	}
	if err := workspacepkg.EnsureDefaultSkills(workspace); err != nil {
		return nil, err
	}
	skillRegistry, err := skills.LoadWorkspaceSkills(workspace)
	if err != nil {
		return nil, err
	}
	embeddingProfileName, embeddingProfile, err := resolveEmbeddingProfile(service.configManager, profileName, profile)
	if err != nil {
		return nil, err
	}
	providerConfig, err := service.configManager.GetProviderConfig(profile.Provider)
	if err != nil {
		return nil, err
	}
	llmProvider, err := provider.NewOpenAICompatibleProvider(providerConfig)
	if err != nil {
		return nil, err
	}
	textEmbeddingProvider, modalEmbeddingProvider, err := buildInvocationEmbeddingProviders(service.configManager, embeddingProfile)
	if err != nil {
		return nil, err
	}
	mcpService, err := mcppkg.NewService(workspace, service.sysConfig.MCP, mcppkg.Options{FailFast: true})
	if err != nil {
		return nil, err
	}
	vectorStore := vectorstore.NewSQLiteVecService(workspace, profileName, *embeddingProfile)
	memoryEnabled := service.sysConfig.Memory.Enabled && textEmbeddingProvider != nil
	var memoryService memory.Service
	if memoryEnabled {
		if err := config.ValidateMemoryConfig(service.sysConfig.Memory); err != nil {
			_ = mcpService.Close()
			return nil, fmt.Errorf("invalid memory config: %w", err)
		}
		memoryService = memory.NewService(
			vectorStore,
			llmProvider,
			profile.Model,
			textEmbeddingProvider,
			embeddingProfile.Text,
			service.sysConfig.Memory,
		)
	}

	return &profileRuntime{
		context: appcontext.SystemContext{
			Provider:       llmProvider,
			TextEmbedding:  textEmbeddingProvider,
			ModalEmbedding: modalEmbeddingProvider,
			ConfigManager:  service.configManager,
			Skills:         skillRegistry,
			SystemPrompt:   systemprompt.NewService(workspace),
			SessionManager: session.NewSessionManager(workspace),
			VectorStore:    vectorStore,
			CronService:    service.cronService,
			CronEnabled:    service.cronEnabled,
			MCPService:     mcpService,
			MemoryService:  memoryService,
			MemoryEnabled:  memoryEnabled,
		},
		profileName:            profileName,
		profile:                *profile,
		embeddingProfileName:   embeddingProfileName,
		embeddingProfile:       *embeddingProfile,
		workspace:              workspace,
		skillRegistry:          skillRegistry,
		sysConfig:              service.sysConfig,
		configManager:          service.configManager,
		defaultMessageBus:      service.defaultMessageBus,
		defaultChannelRegistry: service.defaultChannelRegistry,
		cronService:            service.cronService,
		cronEnabled:            service.cronEnabled,
	}, nil
}

func (runtime *profileRuntime) ensureReady() error {
	runtime.startOnce.Do(func() {
		if runtime.context.VectorStore != nil {
			if err := runtime.context.VectorStore.Start(); err != nil {
				runtime.startErr = err
				return
			}
		}
		if runtime.context.MemoryService != nil && runtime.context.MemoryEnabled {
			if err := runtime.context.MemoryService.Initialize(); err != nil {
				if runtime.context.VectorStore != nil {
					_ = runtime.context.VectorStore.Stop()
				}
				runtime.startErr = err
			}
		}
	})
	return runtime.startErr
}

func (runtime *profileRuntime) close() error {
	var firstErr error
	if runtime.context.MCPService != nil {
		if err := runtime.context.MCPService.Close(); err != nil {
			recordFirstError(&firstErr, err)
		}
		runtime.context.MCPService = nil
	}
	if runtime.context.VectorStore != nil {
		if err := runtime.context.VectorStore.Stop(); err != nil {
			recordFirstError(&firstErr, err)
		}
		runtime.context.VectorStore = nil
	}
	if runtime.context.SessionManager != nil {
		if err := runtime.context.SessionManager.Close(); err != nil {
			recordFirstError(&firstErr, err)
		}
		runtime.context.SessionManager = nil
	}
	return firstErr
}

func ensureInvocationSession(executionContext appcontext.SystemContext, message messagebus.Message) error {
	if executionContext.SessionManager == nil {
		return nil
	}
	_, err := executionContext.SessionManager.GetOrCreateSession(session.MakeSessionID(message.ChannelID, message.ChatID), message.SenderID)
	return err
}

func recordFirstError(target *error, err error) {
	if err == nil {
		return
	}
	if *target == nil {
		*target = err
	}
}

func normalizeProfileName(profileName string) string {
	trimmed := strings.TrimSpace(profileName)
	if trimmed == "" {
		return defaultAgentProfileName
	}
	return trimmed
}

func normalizeInvocationMode(mode appcontext.InvocationMode) appcontext.InvocationMode {
	if strings.TrimSpace(string(mode)) == "" {
		return appcontext.InvocationModeForeground
	}
	return mode
}

func resolveEmbeddingProfile(configManager config.ConfigManager, profileName string, profile *config.ProfileConfig) (string, *config.EmbeddingProfileConfig, error) {
	candidates := []string{}
	if profile != nil {
		if explicit := strings.TrimSpace(profile.EmbeddingProfile); explicit != "" {
			candidates = append(candidates, explicit)
		}
	}
	if trimmedProfileName := strings.TrimSpace(profileName); trimmedProfileName != "" {
		candidates = append(candidates, trimmedProfileName)
	}
	candidates = append(candidates, defaultAgentProfileName)

	seen := map[string]struct{}{}
	for _, candidate := range candidates {
		if _, ok := seen[candidate]; ok {
			continue
		}
		seen[candidate] = struct{}{}
		embeddingProfile, err := configManager.GetEmbeddingProfileConfig(candidate)
		if err == nil {
			return candidate, embeddingProfile, nil
		}
	}
	return "", nil, fmt.Errorf("embedding profile not found for agent profile %s", normalizeProfileName(profileName))
}

func buildInvocationEmbeddingProviders(configManager config.ConfigManager, profile *config.EmbeddingProfileConfig) (provider.EmbeddingProvider, provider.EmbeddingProvider, error) {
	if profile == nil {
		return nil, nil, nil
	}
	cache := map[string]provider.EmbeddingProvider{}
	textProvider, err := resolveInvocationEmbeddingProvider(configManager, cache, profile.Text.Provider)
	if err != nil {
		return nil, nil, err
	}
	modalProvider, err := resolveInvocationEmbeddingProvider(configManager, cache, profile.Modal.Provider)
	if err != nil {
		return nil, nil, err
	}
	return textProvider, modalProvider, nil
}

func resolveInvocationEmbeddingProvider(configManager config.ConfigManager, cache map[string]provider.EmbeddingProvider, providerName string) (provider.EmbeddingProvider, error) {
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

func buildInvocationToolRegistry(workspace string, sysConfig *config.SysConfig, skillRegistry skills.Registry, bus messagebus.MessageBus, cronService cron.Service, mcpService mcppkg.Service, memoryService memory.Service) (tools.ToolRegistry, error) {
	registry := tools.NewToolRegistry()
	if err := registry.RegisterTool("read_file", tools.NewReadFileTool(workspace)); err != nil {
		return nil, err
	}
	if err := registry.RegisterTool("list_dir", tools.NewListDirTool(workspace)); err != nil {
		return nil, err
	}
	if err := registry.RegisterTool("terminal", tools.NewTerminalTool(workspace, resolveInvocationToolTimeout(sysConfig.Tools, "terminal", tools.DefaultTerminalTimeout()))); err != nil {
		return nil, err
	}
	if err := registry.RegisterTool("message", tools.NewMessageTool(bus)); err != nil {
		return nil, err
	}
	if err := registry.RegisterTool("get_skill", tools.NewGetSkillTool(skillRegistry)); err != nil {
		return nil, err
	}
	if err := registry.RegisterTool("create_cron", tools.NewCreateCronTool(cronService)); err != nil {
		return nil, err
	}
	if memoryService != nil {
		if err := registry.RegisterTool("recall_memory", tools.NewRecallMemoryTool(memoryService)); err != nil {
			return nil, err
		}
	}
	if mcpService != nil {
		for _, descriptor := range mcpService.ToolDescriptors() {
			if err := registry.RegisterTool(descriptor.Name, descriptor); err != nil {
				return nil, err
			}
		}
	}
	return registry, nil
}

func resolveInvocationToolTimeout(configs []config.ToolConfig, name string, defaultTimeout time.Duration) time.Duration {
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
