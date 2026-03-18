package context

import (
	"github.com/Neneka448/gogoclaw/internal/channels"
	"github.com/Neneka448/gogoclaw/internal/config"
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
)

type InvocationMode string

const (
	InvocationModeForeground InvocationMode = "foreground"
	InvocationModeBackground InvocationMode = "background"
	InvocationModeCron       InvocationMode = "cron"
)

type RuntimeContext struct {
	ProfileName          string
	Profile              config.ProfileConfig
	EmbeddingProfileName string
	EmbeddingProfile     config.EmbeddingProfileConfig
	Workspace            string
	InvocationMode       InvocationMode
}

type InvocationOverrides struct {
	MessageBus             messagebus.MessageBus
	ReplaceMessageBus      bool
	ChannelRegistry        channels.Registry
	ReplaceChannelRegistry bool
}

type InvocationRequest struct {
	ProfileName string
	Message     messagebus.Message
	Mode        InvocationMode
	Overrides   InvocationOverrides
}

type InvocationService interface {
	Invoke(request InvocationRequest) error
	InvokeAsync(request InvocationRequest) (<-chan error, error)
	EnsureProfile(profileName string) error
	Close() error
}

type SystemContext struct {
	MessageBus      messagebus.MessageBus
	Provider        provider.LLMProviderOpenaiCompatible
	TextEmbedding   provider.EmbeddingProvider
	ModalEmbedding  provider.EmbeddingProvider
	ConfigManager   config.ConfigManager
	ToolRegistry    tools.ToolRegistry
	Skills          skills.Registry
	SystemPrompt    systemprompt.Service
	ChannelRegistry channels.Registry
	SessionManager  session.SessionManager
	CurrentSession  session.Session
	VectorStore     vectorstore.Service
	CronService     cron.Service
	CronEnabled     bool
	MCPService      mcppkg.Service
	MemoryService   memory.Service
	MemoryEnabled   bool
	Runtime         RuntimeContext
	Invoker         InvocationService
}
