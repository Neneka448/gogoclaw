package agent

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Neneka448/gogoclaw/internal/config"
	internalcontext "github.com/Neneka448/gogoclaw/internal/context"
	cronpkg "github.com/Neneka448/gogoclaw/internal/cron"
	"github.com/Neneka448/gogoclaw/internal/memory"
	messagebus "github.com/Neneka448/gogoclaw/internal/message_bus"
	"github.com/Neneka448/gogoclaw/internal/provider"
	"github.com/Neneka448/gogoclaw/internal/session"
	"github.com/Neneka448/gogoclaw/internal/skills"
	"github.com/Neneka448/gogoclaw/internal/systemprompt"
	"github.com/Neneka448/gogoclaw/internal/tools"
	openai "github.com/sashabaranov/go-openai"
)

type fakeProvider struct {
	responses []provider.LLMCommonResponse
	errors    []error
	requests  []openai.ChatCompletionRequest
}

func (p *fakeProvider) ChatCompletion(request openai.ChatCompletionRequest) (provider.LLMCommonResponse, error) {
	p.requests = append(p.requests, request)
	var err error
	if len(p.errors) > 0 {
		err = p.errors[0]
		p.errors = p.errors[1:]
	}
	if err != nil {
		return provider.NormalizedResponse{}, err
	}
	response := p.responses[0]
	p.responses = p.responses[1:]
	return response, nil
}

type fakeMemoryService struct {
	initializeCalls int
	ingestCalls     int
	ingestErr       error
	sessionIDs      []string
	messages        [][]openai.ChatCompletionMessage
	blockCh         <-chan struct{}
}

func (service *fakeMemoryService) Initialize() error {
	service.initializeCalls++
	return nil
}

func (service *fakeMemoryService) Close() error {
	return nil
}

func (service *fakeMemoryService) IngestSession(sessionID string, messages []openai.ChatCompletionMessage) error {
	service.ingestCalls++
	service.sessionIDs = append(service.sessionIDs, sessionID)
	service.messages = append(service.messages, append([]openai.ChatCompletionMessage(nil), messages...))
	if service.blockCh != nil {
		<-service.blockCh
	}
	return service.ingestErr
}

func (service *fakeMemoryService) Recall(queryText string, topK int, minSimilarity float64) ([]memory.MemoryNode, error) {
	return nil, nil
}

func (service *fakeMemoryService) GetNode(nodeID string) (*memory.MemoryNode, error) {
	return nil, nil
}

type fakeTool struct {
	result string
	err    error
}

func (tool fakeTool) Execute(args string) (string, error) {
	if tool.err != nil {
		return "", tool.err
	}
	return tool.result, nil
}

type fakeToolRegistry struct {
	tools map[string]tools.ToolDescriptor
}

func mustFlushAgentSessionForTest(t *testing.T, currentSession session.Session) {
	t.Helper()
	if err := currentSession.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
}

func sessionFilePathForTest(workspace string, sessionID string) string {
	return filepath.Join(workspace, "sessions", sessionID+".json")
}

func newAgentTestSessionManager(t *testing.T, workspace string) session.SessionManager {
	t.Helper()
	manager := session.NewSessionManager(workspace)
	t.Cleanup(func() {
		if err := manager.Close(); err != nil {
			t.Fatalf("Close() error = %v", err)
		}
	})
	return manager
}

func newAgentTestCurrentSession(t *testing.T, manager session.SessionManager, msg messagebus.Message) session.Session {
	t.Helper()
	currentSession, err := manager.GetOrCreateSession(session.MakeSessionID(msg.ChannelID, msg.ChatID), msg.SenderID)
	if err != nil {
		t.Fatalf("GetOrCreateSession() error = %v", err)
	}
	if err := currentSession.UpdateMetadata(msg.ChannelID, sessionTypeFromMessage(msg)); err != nil {
		t.Fatalf("UpdateMetadata() error = %v", err)
	}
	return currentSession
}

func (registry *fakeToolRegistry) RegisterTool(name string, tool tools.ToolDescriptor) error {
	if registry.tools == nil {
		registry.tools = make(map[string]tools.ToolDescriptor)
	}
	registry.tools[name] = tool
	return nil
}

func (registry *fakeToolRegistry) GetTool(name string) (tools.ToolDescriptor, error) {
	tool, ok := registry.tools[name]
	if !ok {
		return tools.ToolDescriptor{}, errors.New("tool not found: " + name)
	}
	return tool, nil
}

func (registry *fakeToolRegistry) GetAllTools() []tools.ToolDescriptor {
	all := make([]tools.ToolDescriptor, 0, len(registry.tools))
	for _, tool := range registry.tools {
		all = append(all, tool)
	}
	return all
}

func TestNewAgentLoopRequiresCurrentSession(t *testing.T) {
	_, err := NewAgentLoop(internalcontext.SystemContext{})
	if err == nil || !strings.Contains(err.Error(), "current session") {
		t.Fatalf("NewAgentLoop() error = %v, want missing current session", err)
	}
}

func TestNewAgentLoopRequiresResolvedRuntimeContext(t *testing.T) {
	_, err := NewAgentLoop(internalcontext.SystemContext{
		CurrentSession: newAgentTestCurrentSession(t, newAgentTestSessionManager(t, t.TempDir()), messagebus.Message{
			ChannelID: "cli",
			ChatID:    "chat-1",
			SenderID:  "user-1",
		}),
	})
	if err == nil || !strings.Contains(err.Error(), "resolved runtime context") {
		t.Fatalf("NewAgentLoop() error = %v, want missing runtime context", err)
	}
}

func TestAgentLoopAppendsAssistantAndToolMessagesToSession(t *testing.T) {
	workspace := t.TempDir()
	sessionManager := newAgentTestSessionManager(t, workspace)
	bus := messagebus.NewMessageBus()
	imagePath := filepath.Join(t.TempDir(), "chart.png")
	if err := os.WriteFile(imagePath, []byte("png"), 0644); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}
	providerStub := &fakeProvider{
		responses: []provider.LLMCommonResponse{
			provider.NormalizedResponse{ToolCalls: []provider.LLMToolCall{{
				ID:        "call_1",
				Name:      "search_docs",
				Arguments: `{"query":"go"}`,
				Type:      string(openai.ToolTypeFunction),
			}}},
			provider.NormalizedResponse{Content: "done"},
		},
	}

	toolRegistry := &fakeToolRegistry{tools: map[string]tools.ToolDescriptor{
		"search_docs": {
			Name: "search_docs",
			Tool: fakeTool{result: `{"content":"chart ready","media_paths":["` + imagePath + `"]}`},
			ToolForLLM: openai.Tool{
				Type: openai.ToolTypeFunction,
				Function: &openai.FunctionDefinition{
					Name: "search_docs",
				},
			},
		},
	}}

	inboundMessage := messagebus.Message{
		ChannelID:   "test-channel",
		Message:     "hello",
		MessageID:   "msg-1",
		MessageType: "group",
		ChatID:      "chat-1",
		SenderID:    "user-1",
	}

	loop, err := NewAgentLoop(internalcontext.SystemContext{
		MessageBus:     bus,
		OutputSink:     messagebus.NewMessageBusOutputSink(bus),
		Provider:       providerStub,
		ToolRegistry:   toolRegistry,
		SessionManager: sessionManager,
		CurrentSession: newAgentTestCurrentSession(t, sessionManager, inboundMessage),
		Runtime:        newTestRuntimeContext(workspace, 4),
	})
	if err != nil {
		t.Fatalf("NewAgentLoop() error = %v", err)
	}

	if err := loop.ProcessMessage(inboundMessage); err != nil {
		t.Fatalf("ProcessMessage() error = %v", err)
	}

	sessionStore, err := sessionManager.GetOrCreateSession(session.MakeSessionID(inboundMessage.ChannelID, inboundMessage.ChatID), inboundMessage.SenderID)
	if err != nil {
		t.Fatalf("GetOrCreateSession() error = %v", err)
	}
	messages := sessionStore.GetMessages(10)
	if len(messages) != 4 {
		t.Fatalf("len(messages) = %d, want 4", len(messages))
	}
	if messages[0].Role != openai.ChatMessageRoleUser || messages[0].Content != "hello" {
		t.Fatalf("messages[0] = %#v, want user message", messages[0])
	}
	if messages[1].Role != openai.ChatMessageRoleAssistant || len(messages[1].ToolCalls) != 1 {
		t.Fatalf("messages[1] = %#v, want assistant tool call message", messages[1])
	}
	if messages[2].Role != openai.ChatMessageRoleTool || messages[2].ToolCallID != "call_1" {
		t.Fatalf("messages[2] = %#v, want tool response", messages[2])
	}
	if messages[3].Role != openai.ChatMessageRoleAssistant || messages[3].Content != "done" {
		t.Fatalf("messages[3] = %#v, want final assistant message", messages[3])
	}

	mustFlushAgentSessionForTest(t, sessionStore)
	content, err := os.ReadFile(sessionFilePathForTest(workspace, session.MakeSessionID(inboundMessage.ChannelID, inboundMessage.ChatID)))
	if err != nil {
		t.Fatalf("os.ReadFile() error = %v", err)
	}
	var data session.SessionFile
	if err := json.Unmarshal(content, &data); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if data.Meta.Channel != inboundMessage.ChannelID {
		t.Fatalf("Meta.Channel = %q, want %q", data.Meta.Channel, inboundMessage.ChannelID)
	}

	if len(providerStub.requests) != 2 {
		t.Fatalf("len(requests) = %d, want 2", len(providerStub.requests))
	}
	if len(providerStub.requests[0].Messages) != 1 {
		t.Fatalf("first request len(Messages) = %d, want 1", len(providerStub.requests[0].Messages))
	}
	if len(providerStub.requests[1].Messages) != 3 {
		t.Fatalf("second request len(Messages) = %d, want 3", len(providerStub.requests[1].Messages))
	}

	outboundQueue, err := bus.Get(messagebus.OutboundQueue)
	if err != nil {
		t.Fatalf("Get(OutboundQueue) error = %v", err)
	}
	select {
	case message := <-outboundQueue:
		if message.Message != `search_docs({"query":"go"})` {
			t.Fatalf("first message.Message = %q, want tool call", message.Message)
		}
		if message.FinishReason != "tool_calls" {
			t.Fatalf("first message.FinishReason = %q, want tool_calls", message.FinishReason)
		}
		if message.MessageType != inboundMessage.MessageType {
			t.Fatalf("first message.MessageType = %q, want %q", message.MessageType, inboundMessage.MessageType)
		}
		if message.ChatID != inboundMessage.ChatID {
			t.Fatalf("first message.ChatID = %q, want %q", message.ChatID, inboundMessage.ChatID)
		}
		if message.SenderID != inboundMessage.SenderID {
			t.Fatalf("first message.SenderID = %q, want %q", message.SenderID, inboundMessage.SenderID)
		}
	default:
		t.Fatal("expected tool outbound message")
	}

	select {
	case message := <-outboundQueue:
		if message.Message != "chart ready" {
			t.Fatalf("second message.Message = %q, want chart ready", message.Message)
		}
		if message.FinishReason != "" {
			t.Fatalf("second message.FinishReason = %q, want empty", message.FinishReason)
		}
		if message.Metadata["message_kind"] != "tool_result" {
			t.Fatalf("second message.Metadata[message_kind] = %q, want tool_result", message.Metadata["message_kind"])
		}
		if len(message.MediaPaths) != 1 || message.MediaPaths[0] != imagePath {
			t.Fatalf("second message.MediaPaths = %#v, want [%s]", message.MediaPaths, imagePath)
		}
		if message.MessageType != inboundMessage.MessageType {
			t.Fatalf("second message.MessageType = %q, want %q", message.MessageType, inboundMessage.MessageType)
		}
		if message.ChatID != inboundMessage.ChatID {
			t.Fatalf("second message.ChatID = %q, want %q", message.ChatID, inboundMessage.ChatID)
		}
		if message.SenderID != inboundMessage.SenderID {
			t.Fatalf("second message.SenderID = %q, want %q", message.SenderID, inboundMessage.SenderID)
		}
	default:
		t.Fatal("expected tool result outbound message")
	}

	select {
	case message := <-outboundQueue:
		if message.FinishReason != "stop" {
			t.Fatalf("third message.FinishReason = %q, want stop", message.FinishReason)
		}
		if message.MessageType != inboundMessage.MessageType {
			t.Fatalf("third message.MessageType = %q, want %q", message.MessageType, inboundMessage.MessageType)
		}
		if message.Message != "done" {
			t.Fatalf("third message.Message = %q, want done", message.Message)
		}
		if message.ChatID != inboundMessage.ChatID {
			t.Fatalf("third message.ChatID = %q, want %q", message.ChatID, inboundMessage.ChatID)
		}
		if message.SenderID != inboundMessage.SenderID {
			t.Fatalf("third message.SenderID = %q, want %q", message.SenderID, inboundMessage.SenderID)
		}
	default:
		t.Fatal("expected final outbound message")
	}
}

func TestExtractOutboundToolPayloadFallsBackToRawContent(t *testing.T) {
	content, mediaPaths := extractOutboundToolPayload(`{"result":"ok"}`)
	if content != `{"result":"ok"}` {
		t.Fatalf("content = %q, want raw payload", content)
	}
	if len(mediaPaths) != 0 {
		t.Fatalf("len(mediaPaths) = %d, want 0", len(mediaPaths))
	}
}

func TestExtractOutboundToolPayloadExtractsMediaPaths(t *testing.T) {
	content, mediaPaths := extractOutboundToolPayload(`{"content":"done","media_path":"/tmp/a.png","media_paths":["/tmp/b.png"]}`)
	if content != "done" {
		t.Fatalf("content = %q, want done", content)
	}
	if len(mediaPaths) != 2 || mediaPaths[0] != "/tmp/b.png" || mediaPaths[1] != "/tmp/a.png" {
		t.Fatalf("mediaPaths = %#v, want [/tmp/b.png /tmp/a.png]", mediaPaths)
	}
}

func TestAgentLoopSuppressesFinalReplyAfterMessageToolSend(t *testing.T) {
	workerWorkspace := t.TempDir()
	sessionManager := newAgentTestSessionManager(t, workerWorkspace)
	bus := messagebus.NewMessageBus()
	providerStub := &fakeProvider{
		responses: []provider.LLMCommonResponse{
			provider.NormalizedResponse{ToolCalls: []provider.LLMToolCall{{
				ID:        "call_1",
				Name:      "message",
				Arguments: `{"content":"sent now"}`,
				Type:      string(openai.ToolTypeFunction),
			}}},
			provider.NormalizedResponse{Content: "done"},
		},
	}

	toolRegistry := &fakeToolRegistry{tools: map[string]tools.ToolDescriptor{
		"message": tools.NewMessageTool(messagebus.NewMessageBusOutputSink(bus)),
	}}

	inboundMessage := messagebus.Message{
		ChannelID:   "feishu",
		Message:     "hello",
		MessageID:   "msg-1",
		MessageType: "group",
		ChatID:      "chat-1",
		SenderID:    "user-1",
		Metadata:    map[string]string{"thread_id": "omt_thread"},
	}

	loop, err := NewAgentLoop(internalcontext.SystemContext{
		MessageBus:     bus,
		OutputSink:     messagebus.NewMessageBusOutputSink(bus),
		Provider:       providerStub,
		ToolRegistry:   toolRegistry,
		SessionManager: sessionManager,
		CurrentSession: newAgentTestCurrentSession(t, sessionManager, inboundMessage),
		Runtime: internalcontext.RuntimeContext{
			ProfileName: "worker",
			Profile: config.ProfileConfig{
				Workspace:         workerWorkspace,
				Model:             "gpt-5.4",
				MaxTokens:         512,
				Temperature:       0.1,
				MaxToolIterations: 4,
				MemoryWindow:      10,
				MaxRetryTimes:     1,
			},
			EmbeddingProfileName: "worker-embedding",
			Workspace:            workerWorkspace,
			InvocationMode:       internalcontext.InvocationModeBackground,
		},
	})
	if err != nil {
		t.Fatalf("NewAgentLoop() error = %v", err)
	}
	if err := loop.ProcessMessage(inboundMessage); err != nil {
		t.Fatalf("ProcessMessage() error = %v", err)
	}

	outboundQueue, err := bus.Get(messagebus.OutboundQueue)
	if err != nil {
		t.Fatalf("Get(OutboundQueue) error = %v", err)
	}

	first := <-outboundQueue
	if first.Message != `message({"content":"sent now"})` {
		t.Fatalf("first.Message = %q, want tool call", first.Message)
	}
	second := <-outboundQueue
	if second.Message != "sent now" {
		t.Fatalf("second.Message = %q, want sent now", second.Message)
	}
	if second.Metadata["message_kind"] != "active_message" {
		t.Fatalf("second.Metadata[message_kind] = %q, want active_message", second.Metadata["message_kind"])
	}
	if second.Metadata["workspace"] != workerWorkspace {
		t.Fatalf("second.Metadata[workspace] = %q, want %q", second.Metadata["workspace"], workerWorkspace)
	}
	if second.Metadata["agent_profile"] != "worker" {
		t.Fatalf("second.Metadata[agent_profile] = %q, want worker", second.Metadata["agent_profile"])
	}
	if second.Metadata["invocation_mode"] != string(internalcontext.InvocationModeBackground) {
		t.Fatalf("second.Metadata[invocation_mode] = %q, want %q", second.Metadata["invocation_mode"], internalcontext.InvocationModeBackground)
	}
	if second.Metadata["thread_id"] != "omt_thread" {
		t.Fatalf("second.Metadata[thread_id] = %q, want omt_thread", second.Metadata["thread_id"])
	}
	if _, exists := inboundMessage.Metadata["workspace"]; exists {
		t.Fatal("inbound message metadata unexpectedly mutated with workspace")
	}

	select {
	case extra := <-outboundQueue:
		t.Fatalf("unexpected extra outbound message: %#v", extra)
	default:
	}
}

func TestAgentLoopProvidesRuntimeMetadataToCreateCronTool(t *testing.T) {
	defaultWorkspace := t.TempDir()
	workerWorkspace := t.TempDir()
	sessionManager := newAgentTestSessionManager(t, workerWorkspace)
	bus := messagebus.NewMessageBus()
	providerStub := &fakeProvider{
		responses: []provider.LLMCommonResponse{
			provider.NormalizedResponse{ToolCalls: []provider.LLMToolCall{{
				ID:        "call_1",
				Name:      "create_cron",
				Arguments: `{"cron_id":"worker-report","cron_expression":"0 * * * *","task":"render report","enabled":true}`,
				Type:      string(openai.ToolTypeFunction),
			}}},
			provider.NormalizedResponse{Content: "done"},
		},
	}
	cronResolver := config.NewProfileResolver(map[string]config.ProfileConfig{
		"default": {Workspace: defaultWorkspace},
		"worker":  {Workspace: workerWorkspace},
	}, "default")
	cronService := cronpkg.NewCronService(cronResolver, nil, nil, nil)
	toolRegistry := &fakeToolRegistry{tools: map[string]tools.ToolDescriptor{
		"create_cron": tools.NewCreateCronTool(cronService),
	}}

	inboundMessage := messagebus.Message{
		ChannelID: "feishu",
		ChatID:    "chat-1",
		SenderID:  "user-1",
		Message:   "schedule a report",
	}

	loop, err := NewAgentLoop(internalcontext.SystemContext{
		MessageBus:     bus,
		OutputSink:     messagebus.NewMessageBusOutputSink(bus),
		Provider:       providerStub,
		ToolRegistry:   toolRegistry,
		SessionManager: sessionManager,
		CurrentSession: newAgentTestCurrentSession(t, sessionManager, inboundMessage),
		Runtime: internalcontext.RuntimeContext{
			ProfileName: "worker",
			Profile: config.ProfileConfig{
				Workspace:         workerWorkspace,
				Model:             "gpt-5.4",
				MaxTokens:         512,
				Temperature:       0.1,
				MaxToolIterations: 4,
				MemoryWindow:      10,
				MaxRetryTimes:     1,
			},
			Workspace:      workerWorkspace,
			InvocationMode: internalcontext.InvocationModeBackground,
		},
	})
	if err != nil {
		t.Fatalf("NewAgentLoop() error = %v", err)
	}

	if err := loop.ProcessMessage(inboundMessage); err != nil {
		t.Fatalf("ProcessMessage() error = %v", err)
	}

	storedCron, err := cronService.GetCron("worker-report")
	if err != nil {
		t.Fatalf("GetCron() error = %v", err)
	}
	if storedCron.Path != filepath.Join(workerWorkspace, "crons", "worker-report") {
		t.Fatalf("storedCron.Path = %q, want worker workspace cron dir", storedCron.Path)
	}
	if storedCron.Config.ProfileName != "worker" {
		t.Fatalf("storedCron.Config.ProfileName = %q, want worker", storedCron.Config.ProfileName)
	}
	if storedCron.Config.InvocationMode != string(internalcontext.InvocationModeBackground) {
		t.Fatalf("storedCron.Config.InvocationMode = %q, want %q", storedCron.Config.InvocationMode, internalcontext.InvocationModeBackground)
	}
}

func TestAgentLoopReturnsMaxIterationsMessageWhenNotCompleted(t *testing.T) {
	workspace := t.TempDir()
	sessionManager := newAgentTestSessionManager(t, workspace)
	bus := messagebus.NewMessageBus()
	providerStub := &fakeProvider{
		responses: []provider.LLMCommonResponse{
			provider.NormalizedResponse{
				FinishReason: "tool_calls",
				ToolCalls: []provider.LLMToolCall{{
					ID:        "call_1",
					Name:      "search_docs",
					Arguments: `{"query":"go"}`,
					Type:      string(openai.ToolTypeFunction),
				}},
			},
		},
	}

	toolRegistry := &fakeToolRegistry{tools: map[string]tools.ToolDescriptor{
		"search_docs": {
			Name: "search_docs",
			Tool: fakeTool{result: `{"result":"ok"}`},
			ToolForLLM: openai.Tool{
				Type:     openai.ToolTypeFunction,
				Function: &openai.FunctionDefinition{Name: "search_docs"},
			},
		},
	}}

	inboundMessage := messagebus.Message{
		ChannelID:   "test-channel",
		Message:     "hello",
		MessageID:   "msg-1",
		MessageType: "group",
		ChatID:      "chat-1",
		SenderID:    "user-1",
	}

	loop, err := NewAgentLoop(internalcontext.SystemContext{
		MessageBus:     bus,
		OutputSink:     messagebus.NewMessageBusOutputSink(bus),
		Provider:       providerStub,
		ToolRegistry:   toolRegistry,
		SessionManager: sessionManager,
		CurrentSession: newAgentTestCurrentSession(t, sessionManager, inboundMessage),
		Runtime:        newTestRuntimeContext(workspace, 1),
	})
	if err != nil {
		t.Fatalf("NewAgentLoop() error = %v", err)
	}

	if err := loop.ProcessMessage(inboundMessage); err != nil {
		t.Fatalf("ProcessMessage() error = %v, want nil", err)
	}

	sessionStore, err := sessionManager.GetOrCreateSession(session.MakeSessionID(inboundMessage.ChannelID, inboundMessage.ChatID), inboundMessage.SenderID)
	if err != nil {
		t.Fatalf("GetOrCreateSession() error = %v", err)
	}
	messages := sessionStore.GetMessages(10)
	if len(messages) != 4 {
		t.Fatalf("len(messages) = %d, want 4", len(messages))
	}
	last := messages[3]
	if last.Role != openai.ChatMessageRoleAssistant {
		t.Fatalf("last.Role = %q, want assistant", last.Role)
	}
	want := "I reached the maximum number of tool call iterations (1) without finishing. If you want me to continue, please reply \"continue\"."
	if last.Content != want {
		t.Fatalf("last.Content = %q, want %q", last.Content, want)
	}

	outboundQueue, err := bus.Get(messagebus.OutboundQueue)
	if err != nil {
		t.Fatalf("Get(OutboundQueue) error = %v", err)
	}
	<-outboundQueue
	<-outboundQueue
	message := <-outboundQueue
	if message.FinishReason != "max_iterations" {
		t.Fatalf("message.FinishReason = %q, want max_iterations", message.FinishReason)
	}
	if message.Message != want {
		t.Fatalf("message.Message = %q, want %q", message.Message, want)
	}
	mustFlushAgentSessionForTest(t, sessionStore)
}

func TestAgentLoopContinuesAfterToolExecutionError(t *testing.T) {
	workspace := t.TempDir()
	sessionManager := newAgentTestSessionManager(t, workspace)
	bus := messagebus.NewMessageBus()
	providerStub := &fakeProvider{
		responses: []provider.LLMCommonResponse{
			provider.NormalizedResponse{ToolCalls: []provider.LLMToolCall{{
				ID:        "call_1",
				Name:      "search_docs",
				Arguments: `{"query":"go"}`,
				Type:      string(openai.ToolTypeFunction),
			}}},
			provider.NormalizedResponse{Content: "recovered"},
		},
	}

	toolRegistry := &fakeToolRegistry{tools: map[string]tools.ToolDescriptor{
		"search_docs": {
			Name: "search_docs",
			Tool: fakeTool{err: errors.New("boom")},
			ToolForLLM: openai.Tool{
				Type: openai.ToolTypeFunction,
				Function: &openai.FunctionDefinition{
					Name: "search_docs",
				},
			},
		},
	}}

	inboundMessage := messagebus.Message{
		ChannelID:   "test-channel",
		Message:     "hello",
		MessageID:   "msg-1",
		MessageType: "group",
		ChatID:      "chat-1",
		SenderID:    "user-1",
	}

	loop, err := NewAgentLoop(internalcontext.SystemContext{
		MessageBus:     bus,
		OutputSink:     messagebus.NewMessageBusOutputSink(bus),
		Provider:       providerStub,
		ToolRegistry:   toolRegistry,
		SessionManager: sessionManager,
		CurrentSession: newAgentTestCurrentSession(t, sessionManager, inboundMessage),
		Runtime:        newTestRuntimeContext(workspace, 4),
	})
	if err != nil {
		t.Fatalf("NewAgentLoop() error = %v", err)
	}

	if err := loop.ProcessMessage(inboundMessage); err != nil {
		t.Fatalf("ProcessMessage() error = %v", err)
	}

	if len(providerStub.requests) != 2 {
		t.Fatalf("len(providerStub.requests) = %d, want 2", len(providerStub.requests))
	}
	secondRequest := providerStub.requests[1]
	if len(secondRequest.Messages) != 3 {
		t.Fatalf("len(secondRequest.Messages) = %d, want 3", len(secondRequest.Messages))
	}
	if secondRequest.Messages[2].Role != openai.ChatMessageRoleTool {
		t.Fatalf("secondRequest.Messages[2].Role = %q, want tool", secondRequest.Messages[2].Role)
	}
	if secondRequest.Messages[2].ToolCallID != "call_1" {
		t.Fatalf("secondRequest.Messages[2].ToolCallID = %q, want call_1", secondRequest.Messages[2].ToolCallID)
	}
	if !strings.Contains(secondRequest.Messages[2].Content, "\"error\":\"boom\"") {
		t.Fatalf("secondRequest.Messages[2].Content = %q, want serialized tool error", secondRequest.Messages[2].Content)
	}

	currentSession, err := sessionManager.GetOrCreateSession(session.MakeSessionID(inboundMessage.ChannelID, inboundMessage.ChatID), inboundMessage.SenderID)
	if err != nil {
		t.Fatalf("GetOrCreateSession() error = %v", err)
	}
	messages := currentSession.GetMessages(10)
	if len(messages) != 4 {
		t.Fatalf("len(messages) = %d, want 4", len(messages))
	}
	if messages[2].Role != openai.ChatMessageRoleTool || messages[2].ToolCallID != "call_1" {
		t.Fatalf("messages[2] = %#v, want tool error output", messages[2])
	}
	if !strings.Contains(messages[2].Content, "Tool search_docs failed: boom") {
		t.Fatalf("messages[2].Content = %q, want readable tool error", messages[2].Content)
	}
	if messages[3].Content != "recovered" {
		t.Fatalf("messages[3].Content = %q, want recovered", messages[3].Content)
	}

	outboundQueue, err := bus.Get(messagebus.OutboundQueue)
	if err != nil {
		t.Fatalf("Get(OutboundQueue) error = %v", err)
	}
	<-outboundQueue
	errorMessage := <-outboundQueue
	if errorMessage.Metadata["message_kind"] != "tool_result" {
		t.Fatalf("errorMessage.Metadata[message_kind] = %q, want tool_result", errorMessage.Metadata["message_kind"])
	}
	if errorMessage.Message != "Tool search_docs failed: boom" {
		t.Fatalf("errorMessage.Message = %q, want readable tool error", errorMessage.Message)
	}
	finalMessage := <-outboundQueue
	if finalMessage.Message != "recovered" {
		t.Fatalf("finalMessage.Message = %q, want recovered", finalMessage.Message)
	}
}

func TestAgentLoopStartsNewSessionOnSlashNew(t *testing.T) {
	previousNow := session.SessionNowForTest(func() time.Time { return time.Unix(1700000001, 0) })
	defer previousNow()

	workspace := t.TempDir()
	sessionManager := newAgentTestSessionManager(t, workspace)
	bus := messagebus.NewMessageBus()
	providerStub := &fakeProvider{}
	inboundMessage := messagebus.Message{ChannelID: "feishu", Message: "/new", MessageID: "msg-1", MessageType: "text", ChatID: "chat-1", SenderID: "user-1"}
	currentSession := newAgentTestCurrentSession(t, sessionManager, inboundMessage)

	loop, err := NewAgentLoop(internalcontext.SystemContext{
		MessageBus:     bus,
		OutputSink:     messagebus.NewMessageBusOutputSink(bus),
		Provider:       providerStub,
		ToolRegistry:   &fakeToolRegistry{},
		SessionManager: sessionManager,
		CurrentSession: currentSession,
		Runtime:        newTestRuntimeContext(workspace, 4),
	})
	if err != nil {
		t.Fatalf("NewAgentLoop() error = %v", err)
	}

	if err := currentSession.AppendMessage(openai.ChatCompletionMessage{Role: openai.ChatMessageRoleUser, Content: "history"}); err != nil {
		t.Fatalf("AppendMessage() error = %v", err)
	}
	mustFlushAgentSessionForTest(t, currentSession)

	if err := loop.ProcessMessage(inboundMessage); err != nil {
		t.Fatalf("ProcessMessage() error = %v", err)
	}
	if len(providerStub.requests) != 0 {
		t.Fatalf("len(providerStub.requests) = %d, want 0", len(providerStub.requests))
	}
	if got := currentSession.GetMessages(10); len(got) != 0 {
		t.Fatalf("len(currentSession.GetMessages()) = %d, want 0", len(got))
	}

	archiveFiles, err := filepath.Glob(filepath.Join(workspace, "sessions", session.ArchiveDirName, "*.json"+session.ArchiveFileSuffixToken+"1700000001"))
	if err != nil {
		t.Fatalf("filepath.Glob() error = %v", err)
	}
	if len(archiveFiles) != 1 {
		t.Fatalf("len(archiveFiles) = %d, want 1", len(archiveFiles))
	}

	outboundQueue, err := bus.Get(messagebus.OutboundQueue)
	if err != nil {
		t.Fatalf("Get(OutboundQueue) error = %v", err)
	}
	message := <-outboundQueue
	if message.Message != newSessionReply {
		t.Fatalf("message.Message = %q, want %q", message.Message, newSessionReply)
	}
	if message.FinishReason != "new_session" {
		t.Fatalf("message.FinishReason = %q, want new_session", message.FinishReason)
	}
	if !strings.Contains(archiveFiles[0], filepath.Join("sessions", session.ArchiveDirName)) {
		t.Fatalf("archive file path = %q, want %s folder", archiveFiles[0], session.ArchiveDirName)
	}
}

func TestAgentLoopIngestsFullSessionMemorySynchronouslyOnNew(t *testing.T) {
	configPath := writeTestConfig(t)
	workspace := tempWorkspaceFromConfig(t, configPath)
	sessionManager := newAgentTestSessionManager(t, workspace)
	bus := messagebus.NewMessageBus()
	blockCh := make(chan struct{})
	memoryService := &fakeMemoryService{blockCh: blockCh}
	inboundMessage := messagebus.Message{
		ChannelID:   "feishu",
		Message:     "/new",
		MessageID:   "msg-1",
		MessageType: "text",
		ChatID:      "chat-1",
		SenderID:    "user-1",
	}
	currentSession := newAgentTestCurrentSession(t, sessionManager, inboundMessage)

	loop, err := NewAgentLoop(internalcontext.SystemContext{
		MessageBus:     bus,
		OutputSink:     messagebus.NewMessageBusOutputSink(bus),
		Provider:       &fakeProvider{},
		ToolRegistry:   &fakeToolRegistry{},
		SessionManager: sessionManager,
		CurrentSession: currentSession,
		MemoryService:  memoryService,
		MemoryEnabled:  true,
		Runtime:        newTestRuntimeContext(workspace, 4),
	})
	if err != nil {
		t.Fatalf("NewAgentLoop() error = %v", err)
	}
	for i := 0; i < 12; i++ {
		if err := currentSession.AppendMessage(openai.ChatCompletionMessage{
			Role:    openai.ChatMessageRoleUser,
			Content: "history-" + strconv.Itoa(i),
		}); err != nil {
			t.Fatalf("AppendMessage() error = %v", err)
		}
	}
	mustFlushAgentSessionForTest(t, currentSession)

	doneCh := make(chan error, 1)
	go func() {
		doneCh <- loop.ProcessMessage(inboundMessage)
	}()

	select {
	case err := <-doneCh:
		t.Fatalf("ProcessMessage() returned before memory ingestion completed: %v", err)
	case <-time.After(100 * time.Millisecond):
	}

	outboundQueue, err := bus.Get(messagebus.OutboundQueue)
	if err != nil {
		t.Fatalf("Get(OutboundQueue) error = %v", err)
	}
	select {
	case message := <-outboundQueue:
		if message.Message != memoryProgressMessage {
			t.Fatalf("progress message.Message = %q, want %q", message.Message, memoryProgressMessage)
		}
		if message.Metadata["message_kind"] != "progress" {
			t.Fatalf("progress message.Metadata[message_kind] = %q, want progress", message.Metadata["message_kind"])
		}
		if message.Metadata["progress_kind"] != "memory" {
			t.Fatalf("progress message.Metadata[progress_kind] = %q, want memory", message.Metadata["progress_kind"])
		}
	default:
		t.Fatal("expected memory progress outbound message before ingestion finishes")
	}

	if memoryService.ingestCalls != 1 {
		t.Fatalf("memoryService.ingestCalls = %d, want 1", memoryService.ingestCalls)
	}
	if got := len(memoryService.messages[0]); got != 12 {
		t.Fatalf("len(memoryService.messages[0]) = %d, want 12", got)
	}

	close(blockCh)

	if err := <-doneCh; err != nil {
		t.Fatalf("ProcessMessage() error = %v", err)
	}
	select {
	case message := <-outboundQueue:
		if message.Message != newSessionReply {
			t.Fatalf("final message.Message = %q, want %q", message.Message, newSessionReply)
		}
		if message.FinishReason != "new_session" {
			t.Fatalf("final message.FinishReason = %q, want new_session", message.FinishReason)
		}
	default:
		t.Fatal("expected new session outbound reply after memory ingestion finishes")
	}
	if got := currentSession.GetMessages(10); len(got) != 0 {
		t.Fatalf("len(currentSession.GetMessages()) = %d, want 0", len(got))
	}
}

func TestAgentLoopSlashNewReportsMemoryIngestionFailure(t *testing.T) {
	configPath := writeTestConfig(t)
	workspace := tempWorkspaceFromConfig(t, configPath)
	sessionManager := newAgentTestSessionManager(t, workspace)
	bus := messagebus.NewMessageBus()
	memoryService := &fakeMemoryService{ingestErr: errors.New("embedding service unavailable")}
	inboundMessage := messagebus.Message{
		ChannelID:   "feishu",
		Message:     "/new",
		MessageID:   "msg-1",
		MessageType: "text",
		ChatID:      "chat-1",
		SenderID:    "user-1",
	}
	currentSession := newAgentTestCurrentSession(t, sessionManager, inboundMessage)

	loop, err := NewAgentLoop(internalcontext.SystemContext{
		MessageBus:     bus,
		OutputSink:     messagebus.NewMessageBusOutputSink(bus),
		Provider:       &fakeProvider{},
		ToolRegistry:   &fakeToolRegistry{},
		SessionManager: sessionManager,
		CurrentSession: currentSession,
		MemoryService:  memoryService,
		MemoryEnabled:  true,
		Runtime:        newTestRuntimeContext(workspace, 4),
	})
	if err != nil {
		t.Fatalf("NewAgentLoop() error = %v", err)
	}
	for i := 0; i < 12; i++ {
		if err := currentSession.AppendMessage(openai.ChatCompletionMessage{
			Role:    openai.ChatMessageRoleUser,
			Content: "history-" + strconv.Itoa(i),
		}); err != nil {
			t.Fatalf("AppendMessage() error = %v", err)
		}
	}
	mustFlushAgentSessionForTest(t, currentSession)

	if err := loop.ProcessMessage(inboundMessage); err != nil {
		t.Fatalf("ProcessMessage() error = %v", err)
	}

	// Session should still be reset despite ingestion failure.
	if got := currentSession.GetMessages(10); len(got) != 0 {
		t.Fatalf("len(currentSession.GetMessages()) = %d, want 0", len(got))
	}

	outboundQueue, err := bus.Get(messagebus.OutboundQueue)
	if err != nil {
		t.Fatalf("Get(OutboundQueue) error = %v", err)
	}
	// Drain the progress message first.
	<-outboundQueue
	// The final reply should contain both the new-session text and the warning.
	message := <-outboundQueue
	if !strings.Contains(message.Message, "new session") {
		t.Fatalf("reply missing new session text: %q", message.Message)
	}
	if !strings.Contains(message.Message, "embedding service unavailable") {
		t.Fatalf("reply missing ingestion error: %q", message.Message)
	}
	if message.FinishReason != "new_session" {
		t.Fatalf("message.FinishReason = %q, want new_session", message.FinishReason)
	}
}

func TestAgentLoopInjectsSkillSystemPrompt(t *testing.T) {
	configPath := writeTestConfig(t)
	workspace := tempWorkspaceFromConfig(t, configPath)
	sessionManager := newAgentTestSessionManager(t, t.TempDir())
	providerStub := &fakeProvider{
		responses: []provider.LLMCommonResponse{
			provider.NormalizedResponse{Content: "done"},
		},
	}
	skillRegistry := skills.NewRegistry()
	if err := skillRegistry.Register(skills.Skill{
		Name: "article-summarize",
		FrontMatter: map[string]any{
			"description": "summarize article links",
			"triggers":    []string{"summarize article", "read this link"},
		},
	}); err != nil {
		t.Fatalf("skillRegistry.Register() error = %v", err)
	}

	toolRegistry := &fakeToolRegistry{tools: map[string]tools.ToolDescriptor{
		"get_skill": tools.NewGetSkillTool(skillRegistry),
	}}

	inboundMessage := messagebus.Message{
		ChannelID: "test-channel",
		ChatID:    "chat-1",
		SenderID:  "user-1",
		Message:   "Please summarize this article",
	}

	loop, err := NewAgentLoop(internalcontext.SystemContext{
		Provider:       providerStub,
		ToolRegistry:   toolRegistry,
		Skills:         skillRegistry,
		SystemPrompt:   systemprompt.NewService(workspace),
		SessionManager: sessionManager,
		CurrentSession: newAgentTestCurrentSession(t, sessionManager, inboundMessage),
		Runtime:        newTestRuntimeContext(workspace, 4),
	})
	if err != nil {
		t.Fatalf("NewAgentLoop() error = %v", err)
	}
	if err := loop.ProcessMessage(inboundMessage); err != nil {
		t.Fatalf("ProcessMessage() error = %v", err)
	}
	if len(providerStub.requests) != 1 {
		t.Fatalf("len(providerStub.requests) = %d, want 1", len(providerStub.requests))
	}
	request := providerStub.requests[0]
	if len(request.Messages) != 2 {
		t.Fatalf("len(request.Messages) = %d, want 2", len(request.Messages))
	}
	if request.Messages[0].Role != openai.ChatMessageRoleSystem {
		t.Fatalf("request.Messages[0].Role = %q, want system", request.Messages[0].Role)
	}
	if !strings.Contains(request.Messages[0].Content, "get_skill") {
		t.Fatalf("system prompt = %q, want get_skill guidance", request.Messages[0].Content)
	}
	if !strings.Contains(request.Messages[0].Content, "article-summarize") {
		t.Fatalf("system prompt = %q, want skill name", request.Messages[0].Content)
	}
	if !strings.Contains(request.Messages[0].Content, "summarize article links") {
		t.Fatalf("system prompt = %q, want skill metadata", request.Messages[0].Content)
	}
	if request.Messages[1].Role != openai.ChatMessageRoleUser {
		t.Fatalf("request.Messages[1].Role = %q, want user", request.Messages[1].Role)
	}
}

func writeTestConfig(t *testing.T) string {
	t.Helper()
	return writeTestConfigWithIterations(t, 4)
}

func tempWorkspaceFromConfig(t *testing.T, configPath string) string {
	t.Helper()
	manager := config.NewConfigManager(configPath)
	profile, err := manager.GetAgentProfileConfig("default")
	if err != nil {
		t.Fatalf("GetAgentProfileConfig() error = %v", err)
	}
	return profile.Workspace
}

func newTestRuntimeContext(workspace string, maxIterations int) internalcontext.RuntimeContext {
	return internalcontext.RuntimeContext{
		ProfileName: "default",
		Profile: config.ProfileConfig{
			Workspace:         workspace,
			Provider:          "codex",
			Model:             "gpt-5.4",
			MaxTokens:         512,
			Temperature:       0.1,
			MaxToolIterations: maxIterations,
			MemoryWindow:      10,
			MaxRetryTimes:     1,
		},
		Workspace:      workspace,
		InvocationMode: internalcontext.InvocationModeForeground,
	}
}

func writeTestConfigWithIterations(t *testing.T, maxIterations int) string {
	t.Helper()

	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.json")
	defaultConfig := config.CreateDefaultConfig()
	defaultConfig.Agents.Profiles["default"] = config.ProfileConfig{
		Workspace:         tempDir,
		Provider:          "codex",
		Model:             "gpt-5.4",
		MaxTokens:         512,
		Temperature:       0.1,
		MaxToolIterations: maxIterations,
		MemoryWindow:      10,
		MaxRetryTimes:     1,
	}

	encoded, err := json.Marshal(defaultConfig)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	if err := os.WriteFile(configPath, encoded, 0644); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	return configPath
}

func newTestRuntimeContextWithRetries(workspace string, maxIterations int, maxRetries int) internalcontext.RuntimeContext {
	ctx := newTestRuntimeContext(workspace, maxIterations)
	ctx.Profile.MaxRetryTimes = maxRetries
	return ctx
}

func TestAgentLoopRetriesLLMCallAndSucceeds(t *testing.T) {
	workspace := t.TempDir()
	sessionManager := newAgentTestSessionManager(t, workspace)
	bus := messagebus.NewMessageBus()
	providerStub := &fakeProvider{
		errors:    []error{errors.New("network timeout"), nil},
		responses: []provider.LLMCommonResponse{provider.NormalizedResponse{Content: "hello back"}},
	}

	inboundMessage := messagebus.Message{
		ChannelID: "cli", Message: "hi", ChatID: "chat-1", SenderID: "user-1",
	}

	loop, err := NewAgentLoop(internalcontext.SystemContext{
		MessageBus:     bus,
		OutputSink:     messagebus.NewMessageBusOutputSink(bus),
		Provider:       providerStub,
		ToolRegistry:   &fakeToolRegistry{},
		SessionManager: sessionManager,
		CurrentSession: newAgentTestCurrentSession(t, sessionManager, inboundMessage),
		Runtime:        newTestRuntimeContextWithRetries(workspace, 4, 3),
	})
	if err != nil {
		t.Fatalf("NewAgentLoop() error = %v", err)
	}

	if err := loop.ProcessMessage(inboundMessage); err != nil {
		t.Fatalf("ProcessMessage() error = %v", err)
	}
	if len(providerStub.requests) != 2 {
		t.Fatalf("expected 2 LLM calls (1 fail + 1 success), got %d", len(providerStub.requests))
	}
}

func TestAgentLoopSendsErrorToUserAfterAllRetriesFail(t *testing.T) {
	workspace := t.TempDir()
	sessionManager := newAgentTestSessionManager(t, workspace)
	bus := messagebus.NewMessageBus()
	providerStub := &fakeProvider{
		errors: []error{errors.New("rate limited"), errors.New("rate limited")},
	}

	inboundMessage := messagebus.Message{
		ChannelID: "cli", Message: "hi", ChatID: "chat-1", SenderID: "user-1",
	}

	loop, err := NewAgentLoop(internalcontext.SystemContext{
		MessageBus:     bus,
		OutputSink:     messagebus.NewMessageBusOutputSink(bus),
		Provider:       providerStub,
		ToolRegistry:   &fakeToolRegistry{},
		SessionManager: sessionManager,
		CurrentSession: newAgentTestCurrentSession(t, sessionManager, inboundMessage),
		Runtime:        newTestRuntimeContextWithRetries(workspace, 4, 2),
	})
	if err != nil {
		t.Fatalf("NewAgentLoop() error = %v", err)
	}

	err = loop.ProcessMessage(inboundMessage)
	if err == nil || !strings.Contains(err.Error(), "rate limited") {
		t.Fatalf("ProcessMessage() error = %v, want rate limited", err)
	}
	if len(providerStub.requests) != 2 {
		t.Fatalf("expected 2 LLM calls, got %d", len(providerStub.requests))
	}

	// Verify error message was sent to user via outbound.
	outboundCh, outErr := bus.Get(messagebus.OutboundQueue)
	if outErr != nil {
		t.Fatalf("bus.Get(Outbound) error = %v", outErr)
	}
	select {
	case outbound := <-outboundCh:
		if !strings.Contains(outbound.Message, "LLM request failed") {
			t.Fatalf("outbound message = %q, want error notification", outbound.Message)
		}
		if outbound.FinishReason != "error" {
			t.Fatalf("outbound FinishReason = %q, want error", outbound.FinishReason)
		}
	default:
		t.Fatal("expected error message on outbound queue, got none")
	}
}

type slowTool struct {
	delay time.Duration
}

func (s slowTool) Execute(args string) (string, error) {
	time.Sleep(s.delay)
	return `{"content":"done"}`, nil
}

func TestAgentLoopToolExecutionTimesOut(t *testing.T) {
	workspace := t.TempDir()
	sessionManager := newAgentTestSessionManager(t, workspace)
	bus := messagebus.NewMessageBus()
	providerStub := &fakeProvider{
		responses: []provider.LLMCommonResponse{
			provider.NormalizedResponse{ToolCalls: []provider.LLMToolCall{{
				ID:        "call_1",
				Name:      "slow_tool",
				Arguments: `{}`,
				Type:      string(openai.ToolTypeFunction),
			}}},
			provider.NormalizedResponse{Content: "handled"},
		},
	}

	toolRegistry := &fakeToolRegistry{tools: map[string]tools.ToolDescriptor{
		"slow_tool": {
			Name:    "slow_tool",
			Tool:    slowTool{delay: 5 * time.Second},
			Timeout: 100 * time.Millisecond,
			ToolForLLM: openai.Tool{
				Type:     openai.ToolTypeFunction,
				Function: &openai.FunctionDefinition{Name: "slow_tool"},
			},
		},
	}}

	inboundMessage := messagebus.Message{
		ChannelID: "cli", Message: "run it", ChatID: "chat-1", SenderID: "user-1",
	}

	loop, err := NewAgentLoop(internalcontext.SystemContext{
		MessageBus:     bus,
		OutputSink:     messagebus.NewMessageBusOutputSink(bus),
		Provider:       providerStub,
		ToolRegistry:   toolRegistry,
		SessionManager: sessionManager,
		CurrentSession: newAgentTestCurrentSession(t, sessionManager, inboundMessage),
		Runtime:        newTestRuntimeContext(workspace, 4),
	})
	if err != nil {
		t.Fatalf("NewAgentLoop() error = %v", err)
	}

	if err := loop.ProcessMessage(inboundMessage); err != nil {
		t.Fatalf("ProcessMessage() error = %v", err)
	}

	// The tool timed out, so the second LLM call should have received a timeout error in the tool response.
	if len(providerStub.requests) != 2 {
		t.Fatalf("expected 2 LLM calls, got %d", len(providerStub.requests))
	}
	toolResultMessages := providerStub.requests[1].Messages
	foundTimeout := false
	for _, msg := range toolResultMessages {
		if msg.Role == openai.ChatMessageRoleTool && strings.Contains(msg.Content, "timed out") {
			foundTimeout = true
			break
		}
	}
	if !foundTimeout {
		t.Fatal("expected tool timeout error in second LLM request messages")
	}
}
