package gateway

import (
	"fmt"
	"io"
	"os"
	"runtime"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"github.com/Neneka448/gogoclaw/internal/agent"
	"github.com/Neneka448/gogoclaw/internal/context"
	messagebus "github.com/Neneka448/gogoclaw/internal/message_bus"
	"github.com/Neneka448/gogoclaw/internal/session"
)

var stderrWriter io.Writer = os.Stderr

const minimumGatewayWorkerCount = 2

type sessionMailbox struct {
	pending []messagebus.Message
	queued  bool
	running bool
}

type Gateway interface {
	// directly send a message to the agent and return the response, without starting a session listen loop
	DirectProcessAndReturn(msg messagebus.Message) ([]messagebus.Message, error)

	Start() error
	Stop() error
}

type gateway struct {
	context             context.SystemContext
	inboundStopCh       chan struct{}
	mu                  sync.Mutex
	started             bool
	inboundWG           sync.WaitGroup
	outboundWG          sync.WaitGroup
	workerWG            sync.WaitGroup
	workerCount         int
	schedulerMu         sync.Mutex
	schedulerCond       *sync.Cond
	schedulerStopping   bool
	sessionMailboxes    map[string]*sessionMailbox
	readySessions       []string
}

func NewGateway(context context.SystemContext) Gateway {
	gateway := &gateway{
		context:          context,
		inboundStopCh:    make(chan struct{}),
		workerCount:      defaultGatewayWorkerCount(),
		sessionMailboxes: make(map[string]*sessionMailbox),
	}
	gateway.schedulerCond = sync.NewCond(&gateway.schedulerMu)
	return gateway
}

func (g *gateway) DirectProcessAndReturn(msg messagebus.Message) ([]messagebus.Message, error) {
	if g.context.Invoker == nil && g.context.SessionManager == nil {
		return nil, nil
	}
	if g.context.Invoker == nil {
		_, err := g.context.SessionManager.GetOrCreateSession(session.MakeSessionID(msg.ChannelID, msg.ChatID), msg.SenderID)
		if err != nil {
			return nil, err
		}
		if strings.TrimSpace(msg.Message) == "" {
			return nil, nil
		}
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
	if g.context.Invoker != nil {
		errCh, err := g.context.Invoker.InvokeAsync(context.InvocationRequest{
			ProfileName: profileNameForMessage(msg),
			Message:     msg,
			Mode:        context.InvocationModeForeground,
		})
		if err != nil {
			failed := make(chan error, 1)
			failed <- err
			return failed
		}
		return errCh
	}
	agentLoop := agent.NewAgentLoop(g.context)
	errCh := make(chan error, 1)
	go func() {
		defer func() {
			if recovered := recover(); recovered != nil {
				errCh <- fmt.Errorf("agent loop panic: %v\n%s", recovered, debug.Stack())
			}
		}()
		errCh <- agentLoop.ProcessMessage(msg)
	}()

	return errCh
}

func (g *gateway) listenOutboundMessages(outboundQueue <-chan messagebus.Message, errCh <-chan error, printOutput bool) ([]messagebus.Message, error) {
	results := make([]messagebus.Message, 0, 4)
	agentDone := false
	for {
		if agentDone {
			for {
				select {
				case outbound, ok := <-outboundQueue:
					if !ok {
						return results, nil
					}
					if printOutput {
						if err := g.dispatchOutboundMessage(outbound); err != nil {
							return results, err
						}
					}
					results = append(results, outbound)
					if outbound.FinishReason != "" && outbound.FinishReason != "tool_calls" {
						return results, nil
					}
				default:
					return results, nil
				}
			}
		}
		select {
		case outbound, ok := <-outboundQueue:
			if !ok {
				return results, nil
			}
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
			agentDone = true
			errCh = nil
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
	g.resetSchedulerStateLocked()
	g.started = true
	g.mu.Unlock()

	if err := g.ensureRuntimeReady(); err != nil {
		return g.failStart(err)
	}

	if g.context.CronService != nil && g.context.CronEnabled {
		if err := g.context.CronService.LoadAll(); err != nil {
			return g.failStart(err)
		}
		if crons, err := g.context.CronService.ListCrons(); err == nil {
			if len(crons) == 0 {
				_, _ = fmt.Fprintf(stderrWriter, "[cron] no cron tasks found\n")
			} else {
				_, _ = fmt.Fprintf(stderrWriter, "[cron] loaded %d cron task(s):\n", len(crons))
				for _, c := range crons {
					status := "disabled"
					if c.Config.Enabled {
						status = "enabled"
					}
					_, _ = fmt.Fprintf(stderrWriter, "[cron]   %-20s  %-16s  %s\n", c.Config.CronID, c.Config.CronExpression, status)
				}
			}
		}
		if err := g.context.CronService.Start(); err != nil {
			return g.failStart(err)
		}
		_, _ = fmt.Fprintf(stderrWriter, "[cron] scheduler started\n")
	}

	if g.context.ChannelRegistry != nil {
		if err := g.context.ChannelRegistry.StartAll(); err != nil {
			if g.context.CronService != nil {
				_ = g.context.CronService.Stop()
			}
			return g.failStart(err)
		}
	}

	inboundQueue, err := g.context.MessageBus.Get(messagebus.InboundQueue)
	if err != nil {
		if g.context.CronService != nil {
			_ = g.context.CronService.Stop()
		}
		return g.failStart(err)
	}
	outboundQueue, err := g.context.MessageBus.Get(messagebus.OutboundQueue)
	if err != nil {
		if g.context.CronService != nil {
			_ = g.context.CronService.Stop()
		}
		return g.failStart(err)
	}

	stopCh := g.inboundStopCh
	g.workerWG.Add(g.workerCount)
	for i := 0; i < g.workerCount; i++ {
		go g.runInboundWorker()
	}
	g.inboundWG.Add(1)
	go g.consumeInboundMessages(stopCh, inboundQueue)
	g.outboundWG.Add(1)
	go g.consumeOutboundMessages(outboundQueue)
	return nil
}

func (g *gateway) failStart(startErr error) error {
	if g.context.Invoker != nil {
		_ = g.context.Invoker.Close()
	} else if g.context.VectorStore != nil {
		_ = g.context.VectorStore.Stop()
	}
	g.mu.Lock()
	g.started = false
	g.mu.Unlock()
	return startErr
}

func (g *gateway) consumeInboundMessages(stopCh <-chan struct{}, inboundQueue <-chan messagebus.Message) {
	defer g.inboundWG.Done()
	for {
		select {
		case <-stopCh:
			g.drainInboundMessages(inboundQueue)
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

	if started && stopCh != nil {
		close(stopCh)
	}
	g.inboundWG.Wait()

	g.schedulerMu.Lock()
	g.schedulerStopping = true
	g.schedulerCond.Broadcast()
	g.schedulerMu.Unlock()
	g.workerWG.Wait()

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
	g.outboundWG.Wait()

	if g.context.ChannelRegistry != nil {
		g.context.ChannelRegistry = nil
	}
	if g.context.CronService != nil {
		g.context.CronService = nil
	}
	if g.context.Invoker != nil {
		if err := g.context.Invoker.Close(); err != nil {
			return err
		}
		g.context.Invoker = nil
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
	if g.context.Invoker != nil {
		return g.context.Invoker.EnsureProfile("default")
	}
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
	}

	return nil
}

func (g *gateway) enqueueInboundMessage(message messagebus.Message) error {
	sessionID := session.MakeSessionID(message.ChannelID, message.ChatID)
	g.schedulerMu.Lock()
	defer g.schedulerMu.Unlock()

	if g.schedulerStopping {
		return nil
	}

	mailbox, ok := g.sessionMailboxes[sessionID]
	if !ok {
		mailbox = &sessionMailbox{}
		g.sessionMailboxes[sessionID] = mailbox
	}
	mailbox.pending = append(mailbox.pending, message)
	if !mailbox.running && !mailbox.queued {
		g.queueReadySessionLocked(sessionID, mailbox)
	}
	return nil
}

func (g *gateway) runInboundWorker() {
	defer g.workerWG.Done()

	for {
		sessionID, message, ok := g.nextInboundMessage()
		if !ok {
			return
		}
		if g.context.Invoker != nil {
			if err := g.context.Invoker.Invoke(context.InvocationRequest{
				ProfileName: profileNameForMessage(message),
				Message:     message,
				Mode:        context.InvocationModeBackground,
			}); err != nil {
				g.logBackgroundError("inbound", message, err)
			}
			g.completeInboundMessage(sessionID)
			continue
		}
		if err := agent.NewAgentLoop(g.context).ProcessMessage(message); err != nil {
			g.logBackgroundError("inbound", message, err)
		}
		g.completeInboundMessage(sessionID)
	}
}

func profileNameForMessage(msg messagebus.Message) string {
	if msg.Metadata == nil {
		return "default"
	}
	name := strings.TrimSpace(msg.Metadata["agent_profile"])
	if name == "" {
		return "default"
	}
	return name
}

func (g *gateway) nextInboundMessage() (string, messagebus.Message, bool) {
	g.schedulerMu.Lock()
	defer g.schedulerMu.Unlock()

	for {
		for len(g.readySessions) > 0 {
			sessionID := g.readySessions[0]
			g.readySessions = g.readySessions[1:]

			mailbox, ok := g.sessionMailboxes[sessionID]
			if !ok {
				continue
			}
			mailbox.queued = false
			if mailbox.running || len(mailbox.pending) == 0 {
				if !mailbox.running && len(mailbox.pending) == 0 {
					delete(g.sessionMailboxes, sessionID)
				}
				continue
			}

			message := mailbox.pending[0]
			mailbox.pending = mailbox.pending[1:]
			mailbox.running = true
			return sessionID, message, true
		}

		if g.schedulerStopping {
			return "", messagebus.Message{}, false
		}
		g.schedulerCond.Wait()
	}
}

func (g *gateway) completeInboundMessage(sessionID string) {
	g.schedulerMu.Lock()
	defer g.schedulerMu.Unlock()

	mailbox, ok := g.sessionMailboxes[sessionID]
	if !ok {
		return
	}
	mailbox.running = false
	if len(mailbox.pending) == 0 {
		delete(g.sessionMailboxes, sessionID)
		return
	}
	if !mailbox.queued {
		g.queueReadySessionLocked(sessionID, mailbox)
	}
}

func (g *gateway) queueReadySessionLocked(sessionID string, mailbox *sessionMailbox) {
	if mailbox == nil || mailbox.queued {
		return
	}
	mailbox.queued = true
	g.readySessions = append(g.readySessions, sessionID)
	g.schedulerCond.Signal()
}

func (g *gateway) drainInboundMessages(inboundQueue <-chan messagebus.Message) {
	for {
		select {
		case msg, ok := <-inboundQueue:
			if !ok {
				return
			}
			if err := g.enqueueInboundMessage(msg); err != nil {
				g.logBackgroundError("inbound", msg, err)
			}
		default:
			return
		}
	}
}

func (g *gateway) resetSchedulerStateLocked() {
	g.schedulerMu.Lock()
	defer g.schedulerMu.Unlock()

	if g.workerCount <= 0 {
		g.workerCount = defaultGatewayWorkerCount()
	}
	if g.schedulerCond == nil {
		g.schedulerCond = sync.NewCond(&g.schedulerMu)
	}
	if g.sessionMailboxes == nil {
		g.sessionMailboxes = make(map[string]*sessionMailbox)
	}
	g.schedulerStopping = false
	g.readySessions = nil
	clear(g.sessionMailboxes)
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

func defaultGatewayWorkerCount() int {
	count := runtime.GOMAXPROCS(0)
	if count < minimumGatewayWorkerCount {
		return minimumGatewayWorkerCount
	}
	return count
}
