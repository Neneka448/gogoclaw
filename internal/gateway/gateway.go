package gateway

import (
	"github.com/Neneka448/gogoclaw/internal/context"
	messagebus "github.com/Neneka448/gogoclaw/internal/message_bus"
)

type Gateway interface {
	// directly send a message to the agent and return the response, without starting a session listen loop
	DirectProcessAndReturn(msg messagebus.Message) error

	Start() error
	Stop() error
}

type gateway struct {
	context context.SystemContext
}

func NewGateway(context context.SystemContext) Gateway {
	return &gateway{
		context: context,
	}
}

func (g *gateway) DirectProcessAndReturn(msg messagebus.Message) error {

	return nil
}

func (g *gateway) Start() error {
	return nil
}

func (g *gateway) Stop() error {
	g.context.MessageBus.Close()
	return nil
}
