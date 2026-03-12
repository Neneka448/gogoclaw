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

const (
	toolCallColor  = "\x1b[32m"
	assistantColor = "\x1b[38;5;208m"
	resetColor     = "\x1b[0m"
)

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
	if strings.TrimSpace(msg.Message) == "" {
		return
	}
	if msg.FinishReason == "tool_calls" {
		printToolCallMessage(msg)
		return
	}
	if msg.FinishReason == "" {
		return
	}
	fmt.Printf("[message]:\n%s%s%s\n", assistantColor, msg.Message, resetColor)

}
func printToolCallMessage(msg messagebus.Message) {
	fmt.Printf("%s[tool call]: %s%s\n", toolCallColor, msg.Message, resetColor)
}

func (g *gateway) Start() error {
	return nil
}

func (g *gateway) Stop() error {
	if g.context.SessionManager != nil {
		if err := g.context.SessionManager.Close(); err != nil {
			return err
		}
	}
	if g.context.MessageBus != nil {
		if err := g.context.MessageBus.Close(); err != nil {
			return err
		}
	}

	return nil
}
