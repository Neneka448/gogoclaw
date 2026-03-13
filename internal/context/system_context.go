package context

import (
	"github.com/Neneka448/gogoclaw/internal/channels"
	"github.com/Neneka448/gogoclaw/internal/config"
	"github.com/Neneka448/gogoclaw/internal/cron"
	messagebus "github.com/Neneka448/gogoclaw/internal/message_bus"
	"github.com/Neneka448/gogoclaw/internal/provider"
	"github.com/Neneka448/gogoclaw/internal/session"
	"github.com/Neneka448/gogoclaw/internal/skills"
	"github.com/Neneka448/gogoclaw/internal/systemprompt"
	"github.com/Neneka448/gogoclaw/internal/tools"
	"github.com/Neneka448/gogoclaw/internal/vectorstore"
)

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
	VectorStore     vectorstore.Service
	CronService     cron.Service
	CronEnabled     bool
}
