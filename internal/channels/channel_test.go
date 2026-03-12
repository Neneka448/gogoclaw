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

	post := extractFeishuContent("post", `{"zh_cn":{"title":"日报","content":[[{"tag":"text","text":"完成"}]]}}`)
	if post != "日报 完成" {
		t.Fatalf("extractFeishuContent(post) = %q, want 日报 完成", post)
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