package session

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	openai "github.com/sashabaranov/go-openai"
)

func TestSessionRespectsMemoryWindow(t *testing.T) {
	manager := NewSessionManager(t.TempDir())
	session, err := manager.GetOrCreateSession("session-1", "user-1")
	if err != nil {
		t.Fatalf("GetOrCreateSession() error = %v", err)
	}
	if err := session.AppendMessages([]openai.ChatCompletionMessage{
		{Role: openai.ChatMessageRoleSystem, Content: "system"},
		{Role: openai.ChatMessageRoleUser, Content: "first"},
		{Role: openai.ChatMessageRoleAssistant, Content: "second"},
	}); err != nil {
		t.Fatalf("AppendMessages() error = %v", err)
	}

	messages := session.GetMessages(2)

	if len(messages) != 2 {
		t.Fatalf("len(messages) = %d, want 2", len(messages))
	}
	if messages[0].Content != "first" || messages[1].Content != "second" {
		t.Fatalf("messages = %#v, want last two messages", messages)
	}
}

func TestSessionBacktracksWindowToIncludeAssistantToolCall(t *testing.T) {
	manager := NewSessionManager(t.TempDir())
	session, err := manager.GetOrCreateSession("session-1", "user-1")
	if err != nil {
		t.Fatalf("GetOrCreateSession() error = %v", err)
	}
	if err := session.AppendMessages([]openai.ChatCompletionMessage{
		{Role: openai.ChatMessageRoleUser, Content: "hello"},
		{
			Role: openai.ChatMessageRoleAssistant,
			ToolCalls: []openai.ToolCall{{
				ID:   "call_1",
				Type: openai.ToolTypeFunction,
				Function: openai.FunctionCall{
					Name:      "read_file",
					Arguments: `{"path":"README.md"}`,
				},
			}},
		},
		{Role: openai.ChatMessageRoleTool, ToolCallID: "call_1", Content: `{"content":"ok"}`},
		{Role: openai.ChatMessageRoleAssistant, Content: "done"},
	}); err != nil {
		t.Fatalf("AppendMessages() error = %v", err)
	}

	messages := session.GetMessages(2)

	if len(messages) != 3 {
		t.Fatalf("len(messages) = %d, want 3", len(messages))
	}
	if messages[0].Role != openai.ChatMessageRoleAssistant || len(messages[0].ToolCalls) != 1 {
		t.Fatalf("messages[0] = %#v, want assistant tool call", messages[0])
	}
	if messages[1].Role != openai.ChatMessageRoleTool || messages[1].ToolCallID != "call_1" {
		t.Fatalf("messages[1] = %#v, want matching tool result", messages[1])
	}
	if messages[2].Content != "done" {
		t.Fatalf("messages[2] = %#v, want final assistant reply", messages[2])
	}
}

func TestSessionReturnsCopies(t *testing.T) {
	manager := NewSessionManager(t.TempDir())
	session, err := manager.GetOrCreateSession("session-1", "user-1")
	if err != nil {
		t.Fatalf("GetOrCreateSession() error = %v", err)
	}
	if err := session.AppendMessage(openai.ChatCompletionMessage{
		Role: openai.ChatMessageRoleAssistant,
		ToolCalls: []openai.ToolCall{{
			ID:   "call_1",
			Type: openai.ToolTypeFunction,
			Function: openai.FunctionCall{
				Name:      "search_docs",
				Arguments: `{"query":"go"}`,
			},
		}},
	}); err != nil {
		t.Fatalf("AppendMessage() error = %v", err)
	}

	messages := session.GetMessages(10)
	messages[0].ToolCalls[0].Function.Name = "mutated"

	again := session.GetMessages(10)
	if again[0].ToolCalls[0].Function.Name != "search_docs" {
		t.Fatalf("ToolCalls[0].Function.Name = %q, want search_docs", again[0].ToolCalls[0].Function.Name)
	}
}

func TestSessionInitializesJSONFile(t *testing.T) {
	workspace := t.TempDir()
	manager := NewSessionManager(workspace)
	session, err := manager.GetOrCreateSession("telegram:chat-1", "user-1")
	if err != nil {
		t.Fatalf("GetOrCreateSession() error = %v", err)
	}
	if err := session.AppendMessage(openai.ChatCompletionMessage{Role: openai.ChatMessageRoleUser, Content: "hello"}); err != nil {
		t.Fatalf("AppendMessage() error = %v", err)
	}
	if err := session.WriteSessionFile(); err != nil {
		t.Fatalf("WriteSessionFile() error = %v", err)
	}

	content, err := os.ReadFile(filepath.Join(workspace, "sessions", "telegram:chat-1.json"))
	if err != nil {
		t.Fatalf("os.ReadFile() error = %v", err)
	}

	var data SessionFile
	if err := json.Unmarshal(content, &data); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if data.Meta.SessionKey != "telegram:chat-1" {
		t.Fatalf("Meta.SessionKey = %q, want telegram:chat-1", data.Meta.SessionKey)
	}
	if data.Meta.SenderID != "user-1" {
		t.Fatalf("Meta.SenderID = %q, want user-1", data.Meta.SenderID)
	}
	if len(data.Messages) != 1 || data.Messages[0].Content != "hello" {
		t.Fatalf("Messages = %#v, want one hello message", data.Messages)
	}
}

func TestSessionManagerCachesSessionInMemory(t *testing.T) {
	manager := NewSessionManager(t.TempDir())
	first, err := manager.GetOrCreateSession("telegram:chat-1", "user-1")
	if err != nil {
		t.Fatalf("GetOrCreateSession() error = %v", err)
	}
	second, err := manager.GetOrCreateSession("telegram:chat-1", "user-1")
	if err != nil {
		t.Fatalf("GetOrCreateSession() error = %v", err)
	}
	if first != second {
		t.Fatal("expected session manager to return cached session instance")
	}
}

func TestSessionManagerCloseFlushesPendingMessages(t *testing.T) {
	workspace := t.TempDir()
	manager := NewSessionManager(workspace)
	session, err := manager.GetOrCreateSession("telegram:chat-1", "user-1")
	if err != nil {
		t.Fatalf("GetOrCreateSession() error = %v", err)
	}
	if err := session.AppendMessage(openai.ChatCompletionMessage{Role: openai.ChatMessageRoleAssistant, Content: "final reply"}); err != nil {
		t.Fatalf("AppendMessage() error = %v", err)
	}

	if err := manager.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	content, err := os.ReadFile(filepath.Join(workspace, "sessions", "telegram:chat-1.json"))
	if err != nil {
		t.Fatalf("os.ReadFile() error = %v", err)
	}

	var data SessionFile
	if err := json.Unmarshal(content, &data); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if len(data.Messages) != 1 {
		t.Fatalf("len(data.Messages) = %d, want 1", len(data.Messages))
	}
	if data.Messages[0].Content != "final reply" {
		t.Fatalf("data.Messages[0].Content = %q, want final reply", data.Messages[0].Content)
	}
}

func TestSessionArchiveAndReset(t *testing.T) {
	previousNow := sessionNow
	sessionNow = func() time.Time { return time.Unix(1700000000, 0) }
	defer func() { sessionNow = previousNow }()

	workspace := t.TempDir()
	manager := NewSessionManager(workspace)
	currentSession, err := manager.GetOrCreateSession("feishu:chat-1", "user-1")
	if err != nil {
		t.Fatalf("GetOrCreateSession() error = %v", err)
	}
	if err := currentSession.AppendMessages([]openai.ChatCompletionMessage{{Role: openai.ChatMessageRoleUser, Content: "hello"}, {Role: openai.ChatMessageRoleAssistant, Content: "world"}}); err != nil {
		t.Fatalf("AppendMessages() error = %v", err)
	}
	if err := currentSession.WriteSessionFile(); err != nil {
		t.Fatalf("WriteSessionFile() error = %v", err)
	}

	archivePath, err := currentSession.ArchiveAndReset()
	if err != nil {
		t.Fatalf("ArchiveAndReset() error = %v", err)
	}
	if !strings.Contains(archivePath, filepath.Join("sessions", "achrive")) {
		t.Fatalf("archivePath = %q, want achrive directory", archivePath)
	}
	if !strings.HasSuffix(archivePath, ".json_achrive_1700000000") {
		t.Fatalf("archivePath = %q, want suffix .json_achrive_1700000000", archivePath)
	}

	archivedContent, err := os.ReadFile(archivePath)
	if err != nil {
		t.Fatalf("os.ReadFile(archivePath) error = %v", err)
	}
	var archived SessionFile
	if err := json.Unmarshal(archivedContent, &archived); err != nil {
		t.Fatalf("json.Unmarshal(archived) error = %v", err)
	}
	if len(archived.Messages) != 2 {
		t.Fatalf("len(archived.Messages) = %d, want 2", len(archived.Messages))
	}

	if got := currentSession.GetMessages(10); len(got) != 0 {
		t.Fatalf("len(currentSession.GetMessages()) = %d, want 0", len(got))
	}
	currentContent, err := os.ReadFile(currentSession.GetSessionFilePath())
	if err != nil {
		t.Fatalf("os.ReadFile(current session) error = %v", err)
	}
	var cleared SessionFile
	if err := json.Unmarshal(currentContent, &cleared); err != nil {
		t.Fatalf("json.Unmarshal(cleared) error = %v", err)
	}
	if len(cleared.Messages) != 0 {
		t.Fatalf("len(cleared.Messages) = %d, want 0", len(cleared.Messages))
	}
}
