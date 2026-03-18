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
	configManager          config.ConfigManager
	defaultMessageBus      messagebus.MessageBus
	defaultChannelRegistry channels.Registry
	cronService            cron.Service
	cronEnabled            bool
	codexTokenProvider     provider.TokenProvider
	mu                     sync.Mutex
	runtimes               map[string]*profileRuntime
}

type profileRuntime struct {
	context                appcontext.SystemContext
	profileName            string
	profile                config.ProfileConfig
	embeddingProfileName   string
	embeddingProfile       config.EmbeddingProfileConfig
	workspace              string
	skillRegistry          skills.Registry
	configManager          config.ConfigManager
	defaultMessageBus      messagebus.MessageBus
	defaultChannelRegistry channels.Registry
	cronService            cron.Service
	cronEnabled            bool
	startOnce              sync.Once
	startErr               error
}

func NewInvocationService(configManager config.ConfigManager, defaultMessageBus messagebus.MessageBus, defaultChannelRegistry channels.Registry, cronService cron.Service, cronEnabled bool, codexTokenProvider provider.TokenProvider) (appcontext.InvocationService, error) {
	if configManager == nil {
		return nil, fmt.Errorf("config manager is required")
	}
	return &invocationService{
		configManager:          configManager,
		defaultMessageBus:      defaultMessageBus,
		defaultChannelRegistry: defaultChannelRegistry,
		cronService:            cronService,
		cronEnabled:            cronEnabled,
		codexTokenProvider:     codexTokenProvider,
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
	if strings.TrimSpace(request.Message.Message) == "" {
		return nil
	}
	loop, err := NewAgentLoop(executionContext)
	if err != nil {
		return err
	}
	return loop.ProcessMessage(request.Message)
}

func (service *invocationService) InvokeAsync(request appcontext.InvocationRequest) (<-chan error, error) {
	executionContext, err := service.buildExecutionContext(request)
	if err != nil {
		return nil, err
	}
	errCh := make(chan error, 1)
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
		loop, err := NewAgentLoop(executionContext)
		if err != nil {
			errCh <- err
			return
		}
		errCh <- loop.ProcessMessage(request.Message)
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
	toolConfigs, err := service.configManager.GetToolsConfig()
	if err != nil {
		return appcontext.SystemContext{}, err
	}
	toolRegistry, err := buildInvocationToolRegistry(runtime.workspace, toolConfigs, runtime.skillRegistry, messageBus, runtime.cronService, runtime.context.MCPService, runtime.context.MemoryService)
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
	currentSession, err := resolveInvocationSession(executionContext, request.Message)
	if err != nil {
		return appcontext.SystemContext{}, err
	}
	executionContext.CurrentSession = currentSession
	return executionContext, nil
}

func (service *invocationService) getProfileRuntime(profileName string) (*profileRuntime, error) {
	resolvedProfileName := normalizeProfileName(profileName)
	service.mu.Lock()
	if runtime, ok := service.runtimes[resolvedProfileName]; ok {
		service.mu.Unlock()
		if err := runtime.ensureReady(); err != nil {
			_ = service.discardRuntime(resolvedProfileName, runtime)
			return nil, err
		}
		return runtime, nil
	}
	service.mu.Unlock()

	runtime, err := service.buildProfileRuntime(resolvedProfileName)
	if err != nil {
		return nil, err
	}
	if err := runtime.ensureReady(); err != nil {
		_ = runtime.close()
		return nil, err
	}

	service.mu.Lock()
	defer service.mu.Unlock()
	if existing, ok := service.runtimes[resolvedProfileName]; ok {
		_ = runtime.close()
		if err := existing.ensureReady(); err != nil {
			delete(service.runtimes, resolvedProfileName)
			_ = existing.close()
			return nil, err
		}
		return existing, nil
	}
	service.runtimes[resolvedProfileName] = runtime
	return runtime, nil
}

func (service *invocationService) discardRuntime(profileName string, target *profileRuntime) error {
	service.mu.Lock()
	current, ok := service.runtimes[profileName]
	if !ok || current != target {
		service.mu.Unlock()
		return nil
	}
	delete(service.runtimes, profileName)
	service.mu.Unlock()
	return target.close()
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
	embeddingProfileName, embeddingProfile, err := resolveEmbeddingProfile(service.configManager, profileName)
	if err != nil {
		return nil, err
	}
	providerConfig, err := service.configManager.GetProviderConfig(profile.Provider)
	if err != nil {
		return nil, err
	}
	llmProvider, err := provider.NewOpenAICompatibleProvider(providerConfig, service.codexTokenProvider)
	if err != nil {
		return nil, err
	}
	textEmbeddingProvider, modalEmbeddingProvider, err := buildInvocationEmbeddingProviders(service.configManager, embeddingProfile)
	if err != nil {
		return nil, err
	}
	mcpConfig, err := service.configManager.GetMCPConfig()
	if err != nil {
		return nil, err
	}
	mcpService, err := mcppkg.NewService(workspace, mcpConfig, mcppkg.Options{FailFast: true})
	if err != nil {
		return nil, err
	}
	vectorStore := vectorstore.NewSQLiteVecService(workspace, profileName, *embeddingProfile)
	memoryConfig, err := service.configManager.GetMemoryConfig()
	if err != nil {
		_ = mcpService.Close()
		return nil, err
	}
	memoryEnabled := memoryConfig.Enabled && textEmbeddingProvider != nil
	var memoryService memory.Service
	if memoryEnabled {
		if err := config.ValidateMemoryConfig(memoryConfig); err != nil {
			_ = mcpService.Close()
			return nil, fmt.Errorf("invalid memory config: %w", err)
		}
		memoryService = memory.NewService(
			memory.NewSQLiteStore(vectorStore.Path()),
			vectorStore,
			llmProvider,
			profile.Model,
			textEmbeddingProvider,
			embeddingProfile.Text,
			memoryConfig,
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
		configManager:          service.configManager,
		defaultMessageBus:      service.defaultMessageBus,
		defaultChannelRegistry: service.defaultChannelRegistry,
		cronService:            service.cronService,
		cronEnabled:            service.cronEnabled,
	}, nil
}

func (runtime *profileRuntime) ensureReady() error {
	runtime.startOnce.Do(func() {
		runtime.startErr = appcontext.NewRuntimeInitializer(runtime.context).EnsureReady()
	})
	return runtime.startErr
}

func (runtime *profileRuntime) close() error {
	var firstErr error
	if runtime.context.MemoryService != nil {
		if err := runtime.context.MemoryService.Close(); err != nil {
			recordFirstError(&firstErr, err)
		}
		runtime.context.MemoryService = nil
	}
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

func resolveInvocationSession(executionContext appcontext.SystemContext, message messagebus.Message) (session.Session, error) {
	if executionContext.SessionManager == nil {
		return nil, nil
	}
	currentSession, err := executionContext.SessionManager.GetOrCreateSession(session.MakeSessionID(message.ChannelID, message.ChatID), message.SenderID)
	if err != nil {
		return nil, err
	}
	if err := currentSession.UpdateMetadata(message.ChannelID, sessionTypeFromMessage(message)); err != nil {
		return nil, err
	}
	return currentSession, nil
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

func resolveEmbeddingProfile(configManager config.ConfigManager, profileName string) (string, *config.EmbeddingProfileConfig, error) {
	return configManager.ResolveEmbeddingProfile(profileName)
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

func buildInvocationToolRegistry(workspace string, toolConfigs []config.ToolConfig, skillRegistry skills.Registry, bus messagebus.MessageBus, cronService cron.Service, mcpService mcppkg.Service, memoryService memory.Service) (tools.ToolRegistry, error) {
	registry := tools.NewToolRegistry()
	toolConfigIndex := buildInvocationToolConfigIndex(toolConfigs)
	if err := registry.RegisterTool("read_file", tools.NewReadFileTool(workspace)); err != nil {
		return nil, err
	}
	if err := registry.RegisterTool("list_dir", tools.NewListDirTool(workspace)); err != nil {
		return nil, err
	}
	if err := registry.RegisterTool("terminal", tools.NewTerminalTool(workspace, resolveInvocationToolTimeout(toolConfigIndex, "terminal", tools.DefaultTerminalTimeout()))); err != nil {
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

func buildInvocationToolConfigIndex(configs []config.ToolConfig) map[string]config.ToolConfig {
	index := make(map[string]config.ToolConfig, len(configs))
	for _, toolConfig := range configs {
		name := normalizeInvocationToolName(toolConfig.Name)
		if name == "" {
			continue
		}
		if _, exists := index[name]; exists {
			continue
		}
		index[name] = toolConfig
	}
	return index
}

func resolveInvocationToolTimeout(configs map[string]config.ToolConfig, name string, defaultTimeout time.Duration) time.Duration {
	toolConfig, ok := configs[normalizeInvocationToolName(name)]
	if !ok {
		return defaultTimeout
	}
	if toolConfig.Timeout <= 0 {
		return defaultTimeout
	}
	return time.Duration(toolConfig.Timeout) * time.Second
}

func normalizeInvocationToolName(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}
