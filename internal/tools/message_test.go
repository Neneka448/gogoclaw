package tools

import (
	"encoding/json"
	"testing"

	messagebus "github.com/Neneka448/gogoclaw/internal/message_bus"
)

func TestMessageToolPublishesOutboundMessage(t *testing.T) {
	bus := messagebus.NewMessageBus()
	descriptor := NewMessageTool(bus)
	messageTool, ok := descriptor.Tool.(*MessageTool)
	if !ok {
		t.Fatal("descriptor.Tool is not *MessageTool")
	}
	messageTool.SetMessageContext(messagebus.Message{ChannelID: "feishu", ChatID: "chat-1", MessageID: "msg-1", MessageType: "group", SenderID: "user-1"})
	messageTool.StartTurn()

	result, err := descriptor.Tool.Execute(`{"content":"hello","media_paths":["/tmp/a.png"]}`)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	var parsed messageToolResult
	if err := json.Unmarshal([]byte(result), &parsed); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if parsed.Status != "sent" {
		t.Fatalf("parsed.Status = %q, want sent", parsed.Status)
	}
	if !messageTool.SentInTurn() {
		t.Fatal("SentInTurn() = false, want true")
	}

	outboundQueue, err := bus.Get(messagebus.OutboundQueue)
	if err != nil {
		t.Fatalf("Get(OutboundQueue) error = %v", err)
	}
	message := <-outboundQueue
	if message.ChannelID != "feishu" || message.ChatID != "chat-1" {
		t.Fatalf("outbound = %#v, want feishu/chat-1", message)
	}
	if message.Message != "hello" {
		t.Fatalf("message.Message = %q, want hello", message.Message)
	}
	if len(message.MediaPaths) != 1 || message.MediaPaths[0] != "/tmp/a.png" {
		t.Fatalf("message.MediaPaths = %#v, want [/tmp/a.png]", message.MediaPaths)
	}
	if message.Metadata["message_kind"] != "active_message" {
		t.Fatalf("message.Metadata[message_kind] = %q, want active_message", message.Metadata["message_kind"])
	}
}

func TestMessageToolRequiresContentOrMedia(t *testing.T) {
	bus := messagebus.NewMessageBus()
	descriptor := NewMessageTool(bus)
	messageTool := descriptor.Tool.(*MessageTool)
	messageTool.SetMessageContext(messagebus.Message{ChannelID: "feishu", ChatID: "chat-1"})

	result, err := descriptor.Tool.Execute(`{"content":"   "}`)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	var parsed messageToolResult
	if err := json.Unmarshal([]byte(result), &parsed); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if parsed.Error != "message requires content or media_paths" {
		t.Fatalf("parsed.Error = %q, want validation error", parsed.Error)
	}
}
