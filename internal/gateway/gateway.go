package gateway

import (
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/Neneka448/gogoclaw/internal/agent"
	"github.com/Neneka448/gogoclaw/internal/context"
	messagebus "github.com/Neneka448/gogoclaw/internal/message_bus"
	"github.com/Neneka448/gogoclaw/internal/session"
)

var stderrWriter io.Writer = os.Stderr

type Gateway interface {
	// directly send a message to the agent and return the response, without starting a session listen loop
	DirectProcessAndReturn(msg messagebus.Message) ([]messagebus.Message, error)

	Start() error
	Stop() error
}

type gateway struct {
	context        context.SystemContext
	inboundStopCh  chan struct{}
	mu             sync.Mutex
	workersMu      sync.Mutex
	started        bool
	inboundWG      sync.WaitGroup
	outboundWG     sync.WaitGroup
	sessionWorkers map[string]chan messagebus.Message
	sessionWG      sync.WaitGroup
}

func NewGateway(context context.SystemContext) Gateway {
	return &gateway{
		context:        context,
		inboundStopCh:  make(chan struct{}),
		sessionWorkers: make(map[string]chan messagebus.Message),
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
	if err := g.ensureRuntimeReady(); err != nil {
		return nil, err
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
	if g.inboundStopCh == nil {
		g.inboundStopCh = make(chan struct{})
	}
	g.started = true
	g.mu.Unlock()

	if err := g.ensureRuntimeReady(); err != nil {
		g.mu.Lock()
		g.started = false
		g.mu.Unlock()
		return err
	}

	if g.context.CronService != nil && g.context.CronEnabled {
		if err := g.context.CronService.LoadAll(); err != nil {
			if g.context.VectorStore != nil {
				_ = g.context.VectorStore.Stop()
			}
			g.mu.Lock()
			g.started = false
			g.mu.Unlock()
			return err
		}
		if err := g.context.CronService.Start(); err != nil {
			if g.context.VectorStore != nil {
				_ = g.context.VectorStore.Stop()
			}
			g.mu.Lock()
			g.started = false
			g.mu.Unlock()
			return err
		}
	}

	if g.context.ChannelRegistry != nil {
		if err := g.context.ChannelRegistry.StartAll(); err != nil {
			if g.context.CronService != nil {
				_ = g.context.CronService.Stop()
			}
			if g.context.VectorStore != nil {
				_ = g.context.VectorStore.Stop()
			}
			g.mu.Lock()
			g.started = false
			g.mu.Unlock()
			return err
		}
	}

	inboundQueue, err := g.context.MessageBus.Get(messagebus.InboundQueue)
	if err != nil {
		if g.context.CronService != nil {
			_ = g.context.CronService.Stop()
		}
		if g.context.VectorStore != nil {
			_ = g.context.VectorStore.Stop()
		}
		g.mu.Lock()
		g.started = false
		g.mu.Unlock()
		return err
	}
	outboundQueue, err := g.context.MessageBus.Get(messagebus.OutboundQueue)
	if err != nil {
		if g.context.CronService != nil {
			_ = g.context.CronService.Stop()
		}
		if g.context.VectorStore != nil {
			_ = g.context.VectorStore.Stop()
		}
		g.mu.Lock()
		g.started = false
		g.mu.Unlock()
		return err
	}

	stopCh := g.inboundStopCh
	g.inboundWG.Add(1)
	go g.consumeInboundMessages(stopCh, inboundQueue)
	g.outboundWG.Add(1)
	go g.consumeOutboundMessages(outboundQueue)
	return nil
}

func (g *gateway) consumeInboundMessages(stopCh <-chan struct{}, inboundQueue <-chan messagebus.Message) {
	defer g.inboundWG.Done()
	for {
		select {
		case <-stopCh:
			return
		case msg, ok := <-inboundQueue:
			if !ok {
				return
			}
			if err := g.enqueueInboundMessage(msg); err != nil {
				g.logBackgroundError("inbound", msg, err)
			}
		}
	}
}

func (g *gateway) consumeOutboundMessages(outboundQueue <-chan messagebus.Message) {
	defer g.outboundWG.Done()
	for msg := range outboundQueue {
		if msg.Metadata["source"] == "cron" {
			continue
		}
		if err := g.dispatchOutboundMessage(msg); err != nil {
			g.logBackgroundError("outbound", msg, err)
		}
	}
}

func (g *gateway) Stop() error {
	g.mu.Lock()
	stopCh := g.inboundStopCh
	started := g.started
	if started {
		g.inboundStopCh = nil
		g.started = false
	}
	g.mu.Unlock()

	if started && stopCh != nil {
		close(stopCh)
	}
	g.inboundWG.Wait()

	if g.context.ChannelRegistry != nil {
		if err := g.context.ChannelRegistry.StopAll(); err != nil {
			return err
		}
	}
	if g.context.CronService != nil {
		if err := g.context.CronService.Stop(); err != nil {
			return err
		}
	}

	g.closeSessionWorkers()
	g.sessionWG.Wait()

	if g.context.SessionManager != nil {
		if err := g.context.SessionManager.Close(); err != nil {
			return err
		}
	}
	if err := g.syncDirtySessionsToMemory(); err != nil {
		return err
	}
	if g.context.MessageBus != nil {
		if err := g.context.MessageBus.Close(); err != nil {
			return err
		}
	}
	g.outboundWG.Wait()

	if g.context.ChannelRegistry != nil {
		g.context.ChannelRegistry = nil
	}
	if g.context.CronService != nil {
		g.context.CronService = nil
	}
	if g.context.VectorStore != nil {
		if err := g.context.VectorStore.Stop(); err != nil {
			return err
		}
		g.context.VectorStore = nil
	}
	if g.context.MCPService != nil {
		if err := g.context.MCPService.Close(); err != nil {
			return err
		}
		g.context.MCPService = nil
	}
	if g.context.SessionManager != nil {
		g.context.SessionManager = nil
	}
	g.context.MessageBus = nil

	return nil
}

func (g *gateway) ensureRuntimeReady() error {
	if g.context.VectorStore != nil {
		if err := g.context.VectorStore.Start(); err != nil {
			return err
		}
	}

	if g.context.MemoryService != nil && g.context.MemoryEnabled {
		if err := g.context.MemoryService.Initialize(); err != nil {
			if g.context.VectorStore != nil {
				_ = g.context.VectorStore.Stop()
			}
			return err
		}
		if err := g.syncDirtySessionsToMemory(); err != nil {
			if g.context.VectorStore != nil {
				_ = g.context.VectorStore.Stop()
			}
			return err
		}
	}

	return nil
}

func (g *gateway) enqueueInboundMessage(message messagebus.Message) error {
	sessionID := session.MakeSessionID(message.ChannelID, message.ChatID)

	g.workersMu.Lock()
	worker, ok := g.sessionWorkers[sessionID]
	if !ok {
		worker = make(chan messagebus.Message, 32)
		g.sessionWorkers[sessionID] = worker
		g.sessionWG.Add(1)
		go g.runSessionWorker(sessionID, worker)
	}
	g.workersMu.Unlock()

	g.mu.Lock()
	stopCh := g.inboundStopCh
	g.mu.Unlock()

	if stopCh == nil {
		return nil
	}

	select {
	case worker <- message:
		return nil
	case <-stopCh:
		return nil
	}
}

func (g *gateway) runSessionWorker(sessionID string, worker <-chan messagebus.Message) {
	defer g.sessionWG.Done()
	defer func() {
		g.workersMu.Lock()
		delete(g.sessionWorkers, sessionID)
		g.workersMu.Unlock()
	}()

	for message := range worker {
		if err := agent.NewAgentLoop(g.context).ProcessMessage(message); err != nil {
			g.logBackgroundError("inbound", message, err)
		}
	}
}

func (g *gateway) closeSessionWorkers() {
	g.workersMu.Lock()
	defer g.workersMu.Unlock()

	for sessionID, worker := range g.sessionWorkers {
		close(worker)
		delete(g.sessionWorkers, sessionID)
	}
}

func (g *gateway) syncDirtySessionsToMemory() error {
	if g.context.MemoryService == nil || !g.context.MemoryEnabled || g.context.SessionManager == nil {
		return nil
	}

	sessionIDs, err := g.context.SessionManager.ListSessionIDs()
	if err != nil {
		return err
	}
	for _, sessionID := range sessionIDs {
		currentSession, err := g.context.SessionManager.GetOrCreateSession(sessionID, "")
		if err != nil {
			return err
		}
		messages := currentSession.GetMessages(0)
		if len(messages) == 0 {
			continue
		}
		digest := session.MessagesDigest(messages)
		if digest != "" && digest == currentSession.GetMemoryIngestedDigest() {
			continue
		}
		if err := g.context.MemoryService.IngestSession(currentSession.GetSessionID(), messages); err != nil {
			return err
		}
		if digest != "" {
			if err := currentSession.MarkMemoryIngested(digest); err != nil {
				return err
			}
		}
	}
	return nil
}

func (g *gateway) logBackgroundError(direction string, message messagebus.Message, err error) {
	if err == nil {
		return
	}
	_, _ = fmt.Fprintf(
		stderrWriter,
		"gateway %s error: channel=%s chat=%s message_id=%s err=%v\n",
		direction,
		message.ChannelID,
		message.ChatID,
		message.MessageID,
		err,
	)
}
