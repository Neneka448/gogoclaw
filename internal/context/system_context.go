package context

import (
	messagebus "github.com/Neneka448/gogoclaw/internal/message_bus"
	"github.com/Neneka448/gogoclaw/internal/provider"
)

type SystemContext struct {
	MessageBus messagebus.MessageBus
	Provider   provider.LLMProviderOpenaiCompatible
}
