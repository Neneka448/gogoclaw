package bootstrap

import (
	"github.com/Neneka448/gogoclaw/internal/context"
	"github.com/Neneka448/gogoclaw/internal/gateway"
	messagebus "github.com/Neneka448/gogoclaw/internal/message_bus"
)

func Bootstrap() (*gateway.Gateway, error) {
	context := context.SystemContext{
		MessageBus: messagebus.NewMessageBus(),
	}

	gateway := gateway.NewGateway(context)

	return &gateway, nil
}
