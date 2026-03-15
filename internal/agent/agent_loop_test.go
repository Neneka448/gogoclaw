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
	requests  []openai.ChatCompletionRequest
}

func (provider *fakeProvider) ChatCompletion(request openai.ChatCompletionRequest) (provider.LLMCommonResponse, error) {
	provider.requests = append(provider.requests, request)
	response := provider.responses[0]
	provider.responses = provider.responses[1:]
	return response, nil
}

type fakeMemoryService struct {
	initializeCalls int
	ingestCalls     int
	sessionIDs      []string
	messages        [][]openai.ChatCompletionMessage
	blockCh         <-chan struct{}
}

func (service *fakeMemoryService) Initialize() error {
	service.initializeCalls++
	return nil
}

func (service *fakeMemoryService) IngestSession(sessionID string, messages []openai.ChatCompletionMessage) error {
	service.ingestCalls++
	service.sessionIDs = append(service.sessionIDs, sessionID)
	service.messages = append(service.messages, append([]openai.ChatCompletionMessage(nil), messages...))
	if service.blockCh != nil {
		<-service.blockCh
	}
	return nil
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

func TestAgentLoopAppendsAssistantAndToolMessagesToSession(t *testing.T) {
	configPath := writeTestConfig(t)
	sessionManager := newAgentTestSessionManager(t, t.TempDir())
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

	loop := NewAgentLoop(internalcontext.SystemContext{
		ConfigManager:  config.NewConfigManager(configPath),
		MessageBus:     bus,
		Provider:       providerStub,
		ToolRegistry:   toolRegistry,
		SessionManager: sessionManager,
	})

	inboundMessage := messagebus.Message{
		ChannelID:   "test-channel",
		Message:     "hello",
		MessageID:   "msg-1",
		MessageType: "group",
		ChatID:      "chat-1",
		SenderID:    "user-1",
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
	configPath := writeTestConfig(t)
	sessionManager := newAgentTestSessionManager(t, t.TempDir())
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
		"message": tools.NewMessageTool(bus),
	}}

	loop := NewAgentLoop(internalcontext.SystemContext{
		ConfigManager:  config.NewConfigManager(configPath),
		MessageBus:     bus,
		Provider:       providerStub,
		ToolRegistry:   toolRegistry,
		SessionManager: sessionManager,
	})

	inboundMessage := messagebus.Message{ChannelID: "feishu", Message: "hello", MessageID: "msg-1", MessageType: "group", ChatID: "chat-1", SenderID: "user-1"}
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

	select {
	case extra := <-outboundQueue:
		t.Fatalf("unexpected extra outbound message: %#v", extra)
	default:
	}
}

func TestAgentLoopReturnsMaxIterationsMessageWhenNotCompleted(t *testing.T) {
	configPath := writeTestConfigWithIterations(t, 1)
	sessionManager := newAgentTestSessionManager(t, t.TempDir())
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

	loop := NewAgentLoop(internalcontext.SystemContext{
		ConfigManager:  config.NewConfigManager(configPath),
		MessageBus:     bus,
		Provider:       providerStub,
		ToolRegistry:   toolRegistry,
		SessionManager: sessionManager,
	})

	inboundMessage := messagebus.Message{
		ChannelID:   "test-channel",
		Message:     "hello",
		MessageID:   "msg-1",
		MessageType: "group",
		ChatID:      "chat-1",
		SenderID:    "user-1",
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
}

func TestAgentLoopContinuesAfterToolExecutionError(t *testing.T) {
	configPath := writeTestConfig(t)
	sessionManager := newAgentTestSessionManager(t, t.TempDir())
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

	loop := NewAgentLoop(internalcontext.SystemContext{
		ConfigManager:  config.NewConfigManager(configPath),
		MessageBus:     bus,
		Provider:       providerStub,
		ToolRegistry:   toolRegistry,
		SessionManager: sessionManager,
	})

	inboundMessage := messagebus.Message{
		ChannelID:   "test-channel",
		Message:     "hello",
		MessageID:   "msg-1",
		MessageType: "group",
		ChatID:      "chat-1",
		SenderID:    "user-1",
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

	configPath := writeTestConfig(t)
	workspace := t.TempDir()
	sessionManager := newAgentTestSessionManager(t, workspace)
	bus := messagebus.NewMessageBus()
	providerStub := &fakeProvider{}

	loop := NewAgentLoop(internalcontext.SystemContext{
		ConfigManager:  config.NewConfigManager(configPath),
		MessageBus:     bus,
		Provider:       providerStub,
		ToolRegistry:   &fakeToolRegistry{},
		SessionManager: sessionManager,
	})

	currentSession, err := sessionManager.GetOrCreateSession(session.MakeSessionID("feishu", "chat-1"), "user-1")
	if err != nil {
		t.Fatalf("GetOrCreateSession() error = %v", err)
	}
	if err := currentSession.AppendMessage(openai.ChatCompletionMessage{Role: openai.ChatMessageRoleUser, Content: "history"}); err != nil {
		t.Fatalf("AppendMessage() error = %v", err)
	}
	if err := currentSession.WriteSessionFile(); err != nil {
		t.Fatalf("WriteSessionFile() error = %v", err)
	}

	inboundMessage := messagebus.Message{ChannelID: "feishu", Message: "/new", MessageID: "msg-1", MessageType: "text", ChatID: "chat-1", SenderID: "user-1"}
	if err := loop.ProcessMessage(inboundMessage); err != nil {
		t.Fatalf("ProcessMessage() error = %v", err)
	}
	if len(providerStub.requests) != 0 {
		t.Fatalf("len(providerStub.requests) = %d, want 0", len(providerStub.requests))
	}
	if got := currentSession.GetMessages(10); len(got) != 0 {
		t.Fatalf("len(currentSession.GetMessages()) = %d, want 0", len(got))
	}

	archiveFiles, err := filepath.Glob(filepath.Join(workspace, "sessions", "achrive", "*.json_achrive_1700000001"))
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
	if !strings.Contains(archiveFiles[0], filepath.Join("sessions", "achrive")) {
		t.Fatalf("archive file path = %q, want achrive folder", archiveFiles[0])
	}
}

func TestAgentLoopIngestsFullSessionMemorySynchronouslyOnNew(t *testing.T) {
	configPath := writeTestConfig(t)
	workspace := tempWorkspaceFromConfig(t, configPath)
	sessionManager := newAgentTestSessionManager(t, workspace)
	bus := messagebus.NewMessageBus()
	blockCh := make(chan struct{})
	memoryService := &fakeMemoryService{blockCh: blockCh}

	loop := NewAgentLoop(internalcontext.SystemContext{
		ConfigManager:  config.NewConfigManager(configPath),
		MessageBus:     bus,
		Provider:       &fakeProvider{},
		ToolRegistry:   &fakeToolRegistry{},
		SessionManager: sessionManager,
		MemoryService:  memoryService,
		MemoryEnabled:  true,
	})

	currentSession, err := sessionManager.GetOrCreateSession(session.MakeSessionID("feishu", "chat-1"), "user-1")
	if err != nil {
		t.Fatalf("GetOrCreateSession() error = %v", err)
	}
	for i := 0; i < 12; i++ {
		if err := currentSession.AppendMessage(openai.ChatCompletionMessage{
			Role:    openai.ChatMessageRoleUser,
			Content: "history-" + strconv.Itoa(i),
		}); err != nil {
			t.Fatalf("AppendMessage() error = %v", err)
		}
	}
	if err := currentSession.WriteSessionFile(); err != nil {
		t.Fatalf("WriteSessionFile() error = %v", err)
	}

	doneCh := make(chan error, 1)
	go func() {
		doneCh <- loop.ProcessMessage(messagebus.Message{
			ChannelID:   "feishu",
			Message:     "/new",
			MessageID:   "msg-1",
			MessageType: "text",
			ChatID:      "chat-1",
			SenderID:    "user-1",
		})
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

func TestAgentLoopInjectsSkillSystemPrompt(t *testing.T) {
	configPath := writeTestConfig(t)
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

	loop := NewAgentLoop(internalcontext.SystemContext{
		ConfigManager:  config.NewConfigManager(configPath),
		Provider:       providerStub,
		ToolRegistry:   toolRegistry,
		Skills:         skillRegistry,
		SystemPrompt:   systemprompt.NewService(tempWorkspaceFromConfig(t, configPath)),
		SessionManager: sessionManager,
	})

	inboundMessage := messagebus.Message{
		ChannelID: "test-channel",
		ChatID:    "chat-1",
		SenderID:  "user-1",
		Message:   "Please summarize this article",
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
