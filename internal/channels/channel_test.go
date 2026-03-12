package channels

import (
	"bytes"
	"testing"

	"github.com/Neneka448/gogoclaw/internal/config"
	messagebus "github.com/Neneka448/gogoclaw/internal/message_bus"
)

type fakeChannel struct {
	name     string
	enabled  bool
	started  bool
	stopped  bool
	received []messagebus.Message
}

func (f *fakeChannel) Name() string {
	return f.name
}

func (f *fakeChannel) Enabled() bool {
	return f.enabled
}

func (f *fakeChannel) Start() error {
	f.started = true
	return nil
}

func (f *fakeChannel) Stop() error {
	f.stopped = true
	return nil
}

func (f *fakeChannel) Send(message messagebus.Message) error {
	f.received = append(f.received, message)
	return nil
}

func TestRegistryDispatchesToTargetChannel(t *testing.T) {
	registry := NewRegistry()
	channel := &fakeChannel{name: "feishu", enabled: true}
	if err := registry.Register(channel); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	msg := messagebus.Message{ChannelID: "feishu", Message: "hello"}
	if err := registry.Dispatch(msg); err != nil {
		t.Fatalf("Dispatch() error = %v", err)
	}
	if len(channel.received) != 1 {
		t.Fatalf("len(channel.received) = %d, want 1", len(channel.received))
	}
	if channel.received[0].Message != "hello" {
		t.Fatalf("channel.received[0].Message = %q, want hello", channel.received[0].Message)
	}
}

func TestCLIChannelFormatsMessages(t *testing.T) {
	buffer := &bytes.Buffer{}
	channel := NewCLIChannel(config.CLIChannelConfig{ChannelConfig: config.ChannelConfig{Enabled: true}}, buffer)

	if err := channel.Send(messagebus.Message{Message: "search()", FinishReason: "tool_calls"}); err != nil {
		t.Fatalf("Send(tool call) error = %v", err)
	}
	if err := channel.Send(messagebus.Message{Message: "done", FinishReason: "stop"}); err != nil {
		t.Fatalf("Send(message) error = %v", err)
	}

	want := "[tool call]: search()\n[message]:\ndone\n"
	if buffer.String() != want {
		t.Fatalf("buffer.String() = %q, want %q", buffer.String(), want)
	}
}

func TestExtractFeishuContent(t *testing.T) {
	text := extractFeishuContent("text", `{"text":"hello"}`)
	if text != "hello" {
		t.Fatalf("extractFeishuContent(text) = %q, want hello", text)
	}

	textWithoutAt := extractFeishuContent("text", `{"text":"@bot hello","text_without_at_bot":"hello"}`)
	if textWithoutAt != "hello" {
		t.Fatalf("extractFeishuContent(text_without_at_bot) = %q, want hello", textWithoutAt)
	}

	post := extractFeishuContent("post", `{"zh_cn":{"title":"日报","content":[[{"tag":"text","text":"完成"}]]}}`)
	if post != "日报 完成" {
		t.Fatalf("extractFeishuContent(post) = %q, want 日报 完成", post)
	}

	interactive := extractFeishuContent("interactive", `{"header":{"title":{"content":"告警"}},"elements":[{"tag":"markdown","content":"请处理"}]}`)
	if interactive != "告警 请处理" {
		t.Fatalf("extractFeishuContent(interactive) = %q, want 告警 请处理", interactive)
	}
}

func TestReceiveIDTypeForChatID(t *testing.T) {
	if got := receiveIDTypeForChatID("oc_123"); got != "chat_id" {
		t.Fatalf("receiveIDTypeForChatID(group) = %q, want chat_id", got)
	}
	if got := receiveIDTypeForChatID("ou_123"); got != "open_id" {
		t.Fatalf("receiveIDTypeForChatID(user) = %q, want open_id", got)
	}
}

func TestParseFeishuInboundMessage(t *testing.T) {
	body := map[string]any{
		"event": map[string]any{
			"sender": map[string]any{
				"sender_id": map[string]any{
					"open_id": "ou_user_1",
				},
			},
			"message": map[string]any{
				"message_id":   "om_1",
				"message_type": "text",
				"chat_id":      "oc_chat_1",
				"chat_type":    "group",
				"parent_id":    "om_parent",
				"thread_id":    "omt_thread",
				"content":      `{"text_without_at_bot":"hello"}`,
			},
		},
	}

	message, ok := parseFeishuInboundMessage(body)
	if !ok {
		t.Fatal("parseFeishuInboundMessage() ok = false, want true")
	}
	if message.ChannelID != "feishu" {
		t.Fatalf("message.ChannelID = %q, want feishu", message.ChannelID)
	}
	if message.Message != "hello" {
		t.Fatalf("message.Message = %q, want hello", message.Message)
	}
	if message.ChatID != "oc_chat_1" || message.SenderID != "ou_user_1" {
		t.Fatalf("message ids = (%q,%q), want (oc_chat_1,ou_user_1)", message.ChatID, message.SenderID)
	}
	if message.ReplyTo != "om_parent" {
		t.Fatalf("message.ReplyTo = %q, want om_parent", message.ReplyTo)
	}
	if message.Metadata["thread_id"] != "omt_thread" {
		t.Fatalf("message.Metadata[thread_id] = %q, want omt_thread", message.Metadata["thread_id"])
	}
}

func TestParseFeishuInboundMessageSkipsEmptyContent(t *testing.T) {
	body := map[string]any{
		"event": map[string]any{
			"message": map[string]any{
				"message_type": "image",
				"content":      `{}`,
			},
		},
	}

	if _, ok := parseFeishuInboundMessage(body); ok {
		t.Fatal("parseFeishuInboundMessage() ok = true, want false")
	}
}
