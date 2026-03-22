package tools

import (
	"encoding/json"
	"testing"

	messagebus "github.com/Neneka448/gogoclaw/internal/message_bus"
)

func TestMessageToolPublishesOutboundMessage(t *testing.T) {
	bus := messagebus.NewMessageBus()
	descriptor := NewMessageTool(messagebus.NewMessageBusOutputSink(bus))
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
	descriptor := NewMessageTool(messagebus.NewMessageBusOutputSink(bus))
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

func TestMessageToolTargetProfileRoutesMQ(t *testing.T) {
	bus := messagebus.NewMessageBus()
	descriptor := NewMessageTool(messagebus.NewMessageBusOutputSink(bus))
	messageTool := descriptor.Tool.(*MessageTool)
	messageTool.SetMessageContext(messagebus.Message{
		ChannelID: "feishu", ChatID: "chat-1", MessageID: "msg-1",
		SenderID: "user-1",
		Metadata: map[string]string{"agent_profile": "default"},
	})
	messageTool.StartTurn()

	result, err := descriptor.Tool.Execute(`{"content":"hello remote","target_profile":"remote"}`)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	var parsed messageToolResult
	if err := json.Unmarshal([]byte(result), &parsed); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if parsed.Status != "sent_to_profile:remote" {
		t.Fatalf("Status = %q, want sent_to_profile:remote", parsed.Status)
	}
	if !messageTool.SentInTurn() {
		t.Fatal("SentInTurn() = false, want true")
	}

	outboundQueue, err := bus.Get(messagebus.OutboundQueue)
	if err != nil {
		t.Fatalf("Get(OutboundQueue) error = %v", err)
	}
	message := <-outboundQueue
	if message.ChannelID != "mq" {
		t.Fatalf("ChannelID = %q, want mq", message.ChannelID)
	}
	if message.Metadata["target_profile"] != "remote" {
		t.Fatalf("target_profile = %q, want remote", message.Metadata["target_profile"])
	}
	if message.Metadata["message_kind"] != "active_message" {
		t.Fatalf("message_kind = %q, want active_message", message.Metadata["message_kind"])
	}
	if message.Metadata["return_channel_id"] != "feishu" {
		t.Fatalf("return_channel_id = %q, want feishu", message.Metadata["return_channel_id"])
	}
	if message.Metadata["return_chat_id"] != "chat-1" {
		t.Fatalf("return_chat_id = %q, want chat-1", message.Metadata["return_chat_id"])
	}
	if message.Metadata["return_sender_id"] != "user-1" {
		t.Fatalf("return_sender_id = %q, want user-1", message.Metadata["return_sender_id"])
	}
	if message.Message != "hello remote" {
		t.Fatalf("Message = %q, want hello remote", message.Message)
	}
	if message.MessageType != "direct" {
		t.Fatalf("MessageType = %q, want direct", message.MessageType)
	}
}

func TestMessageToolTargetProfileEmptyFallsBackToChannel(t *testing.T) {
	bus := messagebus.NewMessageBus()
	descriptor := NewMessageTool(messagebus.NewMessageBusOutputSink(bus))
	messageTool := descriptor.Tool.(*MessageTool)
	messageTool.SetMessageContext(messagebus.Message{ChannelID: "feishu", ChatID: "chat-1"})
	messageTool.StartTurn()

	result, err := descriptor.Tool.Execute(`{"content":"hello user","target_profile":""}`)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	var parsed messageToolResult
	if err := json.Unmarshal([]byte(result), &parsed); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if parsed.Status != "sent" {
		t.Fatalf("Status = %q, want sent (channel fallback)", parsed.Status)
	}
	outboundQueue, err := bus.Get(messagebus.OutboundQueue)
	if err != nil {
		t.Fatalf("Get(OutboundQueue) error = %v", err)
	}
	message := <-outboundQueue
	if message.ChannelID != "feishu" {
		t.Fatalf("ChannelID = %q, want feishu (original channel)", message.ChannelID)
	}
}
