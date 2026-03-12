package context

import (
	"github.com/Neneka448/gogoclaw/internal/channels"
	"github.com/Neneka448/gogoclaw/internal/config"
	messagebus "github.com/Neneka448/gogoclaw/internal/message_bus"
	"github.com/Neneka448/gogoclaw/internal/provider"
	"github.com/Neneka448/gogoclaw/internal/session"
	"github.com/Neneka448/gogoclaw/internal/skills"
	"github.com/Neneka448/gogoclaw/internal/tools"
)

type SystemContext struct {
	MessageBus      messagebus.MessageBus
	Provider        provider.LLMProviderOpenaiCompatible
	ConfigManager   config.ConfigManager
	ToolRegistry    tools.ToolRegistry
	Skills          skills.Registry
	ChannelRegistry channels.Registry
	SessionManager  session.SessionManager
}
