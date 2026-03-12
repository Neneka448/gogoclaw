package gateway

import (
	"fmt"
	"os"
	"strings"
	"sync"
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
	stopCh  chan struct{}
	mu      sync.Mutex
	started bool
	wg      sync.WaitGroup
}

func NewGateway(context context.SystemContext) Gateway {
	return &gateway{
		context: context,
		stopCh:  make(chan struct{}),
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

	errCh := g.startAgentLoop(msg)

	return g.listenOutboundMessages(outboundQueue, errCh, true)
}

func (g *gateway) startAgentLoop(msg messagebus.Message) <-chan error {
	agentLoop := agent.NewAgentLoop(g.context)
	errCh := make(chan error, 1)
	go func() {
		errCh <- agentLoop.ProcessMessage(msg)
	}()

	return errCh
}

func (g *gateway) listenOutboundMessages(outboundQueue <-chan messagebus.Message, errCh <-chan error, printOutput bool) ([]messagebus.Message, error) {
	results := make([]messagebus.Message, 0, 4)
	for {
		select {
		case outbound := <-outboundQueue:
			if printOutput {
				if err := g.dispatchOutboundMessage(outbound); err != nil {
					return results, err
				}
			}
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

func (g *gateway) dispatchOutboundMessage(msg messagebus.Message) error {
	if g.context.ChannelRegistry == nil {
		return nil
	}
	return g.context.ChannelRegistry.Dispatch(msg)
}

func (g *gateway) Start() error {
	g.mu.Lock()
	if g.started {
		g.mu.Unlock()
		return nil
	}
	g.started = true
	g.mu.Unlock()

	if g.context.ChannelRegistry != nil {
		if err := g.context.ChannelRegistry.StartAll(); err != nil {
			return err
		}
	}

	inboundQueue, err := g.context.MessageBus.Get(messagebus.InboundQueue)
	if err != nil {
		return err
	}
	outboundQueue, err := g.context.MessageBus.Get(messagebus.OutboundQueue)
	if err != nil {
		return err
	}

	g.wg.Add(2)
	go g.consumeInboundMessages(inboundQueue)
	go g.consumeOutboundMessages(outboundQueue)
	return nil
}

func (g *gateway) consumeInboundMessages(inboundQueue <-chan messagebus.Message) {
	defer g.wg.Done()
	for {
		select {
		case <-g.stopCh:
			return
		case msg, ok := <-inboundQueue:
			if !ok {
				return
			}
			go func(message messagebus.Message) {
				if err := <-g.startAgentLoop(message); err != nil {
					_, _ = fmt.Fprintf(os.Stderr, "gateway inbound error: %v\n", err)
				}
			}(msg)
		}
	}
}

func (g *gateway) consumeOutboundMessages(outboundQueue <-chan messagebus.Message) {
	defer g.wg.Done()
	for {
		select {
		case <-g.stopCh:
			return
		case msg, ok := <-outboundQueue:
			if !ok {
				return
			}
			if err := g.dispatchOutboundMessage(msg); err != nil {
				_, _ = fmt.Fprintf(os.Stderr, "gateway outbound dispatch error: %v\n", err)
			}
		}
	}
}

func (g *gateway) Stop() error {
	g.mu.Lock()
	if !g.started {
		g.mu.Unlock()
	} else {
		close(g.stopCh)
		g.started = false
		g.mu.Unlock()
		g.wg.Wait()
	}
	if g.context.ChannelRegistry != nil {
		if err := g.context.ChannelRegistry.StopAll(); err != nil {
			return err
		}
	}
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
