package gateway

import (
	"fmt"
	"strings"
	"time"

	"github.com/Neneka448/gogoclaw/internal/agent"
	"github.com/Neneka448/gogoclaw/internal/context"
	messagebus "github.com/Neneka448/gogoclaw/internal/message_bus"
	"github.com/Neneka448/gogoclaw/internal/session"
)

type Gateway interface {
	// directly send a message to the agent and return the response, without starting a session listen loop
	DirectProcessAndReturn(msg messagebus.Message) ([]messagebus.Message, error)

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

func (g *gateway) DirectProcessAndReturn(msg messagebus.Message) ([]messagebus.Message, error) {
	if g.context.SessionManager == nil {
		return nil, nil
	}

	_, err := g.context.SessionManager.GetOrCreateSession(session.MakeSessionID(msg.ChannelID, msg.ChatID), msg.SenderID)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(msg.Message) == "" {
		return nil, nil
	}
	outboundQueue, err := g.context.MessageBus.Get(messagebus.OutboundQueue)
	if err != nil {
		return nil, err
	}

	agentLoop := agent.NewAgentLoop(g.context)
	errCh := make(chan error, 1)
	go func() {
		errCh <- agentLoop.ProcessMessage(msg)
	}()

	results := make([]messagebus.Message, 0, 4)
	for {
		select {
		case outbound := <-outboundQueue:
			printMessage(outbound)
			results = append(results, outbound)
			if outbound.FinishReason != "" && outbound.FinishReason != "tool_calls" {
				return results, nil
			}
		case err := <-errCh:
			if err != nil {
				return results, err
			}
			return results, nil
		case <-time.After(time.Second):
			continue
		}
	}
}
func printMessage(msg messagebus.Message) {
	if msg.FinishReason == "tool_calls" {
		printToolCallMessage(msg)
		return
	}
	fmt.Printf("\x1b[36m%s\x1b[0m\n", msg.Message)

}
func printToolCallMessage(msg messagebus.Message) {
	fmt.Printf("\x1b[32m[tool call]: %s\x1b[0m\n", msg.Message)
}

func (g *gateway) Start() error {
	return nil
}

func (g *gateway) Stop() error {
	g.context.MessageBus.Close()
	return nil
}
