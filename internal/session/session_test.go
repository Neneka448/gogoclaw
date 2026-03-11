package session

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

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
