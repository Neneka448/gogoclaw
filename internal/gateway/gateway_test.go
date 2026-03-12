package gateway

import (
	"bytes"
	"errors"
	"testing"
	"time"

	"github.com/Neneka448/gogoclaw/internal/channels"
	appcontext "github.com/Neneka448/gogoclaw/internal/context"
	messagebus "github.com/Neneka448/gogoclaw/internal/message_bus"
)

func TestGatewayStartDispatchesOutboundMessages(t *testing.T) {
	bus := messagebus.NewMessageBus()
	registry := channels.NewRegistry()
	fake := &channelsTestChannel{name: "cli", enabled: true}
	if err := registry.Register(fake); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	gw := NewGateway(appcontext.SystemContext{
		MessageBus:      bus,
		ChannelRegistry: registry,
	})
	if err := gw.Start(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	t.Cleanup(func() {
		if err := gw.Stop(); err != nil {
			t.Fatalf("Stop() error = %v", err)
		}
	})

	if err := bus.Put(messagebus.Message{ChannelID: "cli", Message: "hello", FinishReason: "stop"}, messagebus.OutboundQueue); err != nil {
		t.Fatalf("Put() error = %v", err)
	}

	deadline := time.After(2 * time.Second)
	for len(fake.received) == 0 {
		select {
		case <-deadline:
			t.Fatal("timed out waiting for outbound dispatch")
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}

	if fake.received[0].Message != "hello" {
		t.Fatalf("fake.received[0].Message = %q, want hello", fake.received[0].Message)
	}
}

func TestGatewayStopIsIdempotent(t *testing.T) {
	bus := messagebus.NewMessageBus()
	registry := channels.NewRegistry()
	if err := registry.Register(&channelsTestChannel{name: "cli", enabled: true}); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	gw := NewGateway(appcontext.SystemContext{
		MessageBus:      bus,
		ChannelRegistry: registry,
	})
	if err := gw.Start(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if err := gw.Stop(); err != nil {
		t.Fatalf("Stop() first error = %v", err)
	}
	if err := gw.Stop(); err != nil {
		t.Fatalf("Stop() second error = %v", err)
	}
}

func TestGatewayLogsBackgroundDispatchErrors(t *testing.T) {
	bus := messagebus.NewMessageBus()
	buffer := &bytes.Buffer{}
	gw := &gateway{
		context: appcontext.SystemContext{},
	}
	_ = bus
	stderr := osStderrSwap(buffer)
	defer stderr()

	gw.logBackgroundError("outbound", messagebus.Message{ChannelID: "feishu", ChatID: "oc_1", MessageID: "om_1"}, errors.New("boom"))
	if got := buffer.String(); got != "gateway outbound error: channel=feishu chat=oc_1 message_id=om_1 err=boom\n" {
		t.Fatalf("logBackgroundError() = %q", got)
	}
}

func TestGatewayCanRestartAfterStop(t *testing.T) {
	bus := messagebus.NewMessageBus()
	registry := channels.NewRegistry()
	fake := &channelsTestChannel{name: "cli", enabled: true}
	if err := registry.Register(fake); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	gw := NewGateway(appcontext.SystemContext{
		MessageBus:      bus,
		ChannelRegistry: registry,
	})
	if err := gw.Start(); err != nil {
		t.Fatalf("Start() first error = %v", err)
	}
	if err := gw.Stop(); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}

	bus = messagebus.NewMessageBus()
	gw = NewGateway(appcontext.SystemContext{
		MessageBus:      bus,
		ChannelRegistry: registry,
	})
	if err := gw.Start(); err != nil {
		t.Fatalf("Start() second error = %v", err)
	}
	t.Cleanup(func() {
		_ = gw.Stop()
	})

	if err := bus.Put(messagebus.Message{ChannelID: "cli", Message: "restart", FinishReason: "stop"}, messagebus.OutboundQueue); err != nil {
		t.Fatalf("Put() error = %v", err)
	}

	deadline := time.After(2 * time.Second)
	for len(fake.received) == 0 || fake.received[len(fake.received)-1].Message != "restart" {
		select {
		case <-deadline:
			t.Fatal("timed out waiting for restarted gateway dispatch")
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}
}

type channelsTestChannel struct {
	name     string
	enabled  bool
	received []messagebus.Message
}

func (c *channelsTestChannel) Name() string {
	return c.name
}

func (c *channelsTestChannel) Enabled() bool {
	return c.enabled
}

func (c *channelsTestChannel) Start() error {
	return nil
}

func (c *channelsTestChannel) Stop() error {
	return nil
}

func (c *channelsTestChannel) Send(message messagebus.Message) error {
	c.received = append(c.received, message)
	return nil
}

func osStderrSwap(buffer *bytes.Buffer) func() {
	original := stderrWriter
	stderrWriter = buffer
	return func() {
		stderrWriter = original
	}
}
