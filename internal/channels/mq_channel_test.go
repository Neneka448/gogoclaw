package channels

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/Neneka448/gogoclaw/internal/config"
	messagebus "github.com/Neneka448/gogoclaw/internal/message_bus"
)

type fakeMQBroker struct {
	deliveries chan mqDelivery
	mu         sync.Mutex
	published  []publishedMQMessage
}

type publishedMQMessage struct {
	routingKey string
	body       []byte
}

func newFakeMQBroker() *fakeMQBroker {
	return &fakeMQBroker{deliveries: make(chan mqDelivery, 8)}
}

func (b *fakeMQBroker) Consume() (<-chan mqDelivery, error) {
	return b.deliveries, nil
}

func (b *fakeMQBroker) Publish(routingKey string, body []byte) error {
	cloned := make([]byte, len(body))
	copy(cloned, body)
	b.mu.Lock()
	b.published = append(b.published, publishedMQMessage{routingKey: routingKey, body: cloned})
	b.mu.Unlock()
	return nil
}

func (b *fakeMQBroker) Close() error {
	close(b.deliveries)
	return nil
}

func (b *fakeMQBroker) publishedCount() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.published)
}

func (b *fakeMQBroker) publishedMessage(index int) publishedMQMessage {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.published[index]
}

func TestMQChannelStartCreatesRuntimeConfig(t *testing.T) {
	workspace := t.TempDir()
	bus := messagebus.NewMessageBus()
	broker := newFakeMQBroker()
	ch := newMQChannelWithFactory(config.MQChannelConfig{
		ChannelConfig: config.ChannelConfig{Enabled: true},
		URL:           "amqp://guest:guest@localhost:5672/",
		Exchange:      "agent.bus",
		Profile:       "writer",
		MachineID:     "machine-a",
		Prefetch:      4,
	}, bus, workspace, func(cfg mqResolvedConfig) (mqBroker, error) {
		return broker, nil
	})

	if err := ch.Start(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer ch.Stop()

	configPath := filepath.Join(workspace, mqRuntimeDirName, mqRuntimeConfigFileName)
	content, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("os.ReadFile(%s) error = %v", configPath, err)
	}
	var runtimeCfg mqRuntimeConfigFile
	if err := json.Unmarshal(content, &runtimeCfg); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if runtimeCfg.SourceProfile != "writer" {
		t.Fatalf("SourceProfile = %q, want writer", runtimeCfg.SourceProfile)
	}
	if runtimeCfg.SourceInstanceID != "writer@machine-a" {
		t.Fatalf("SourceInstanceID = %q, want writer@machine-a", runtimeCfg.SourceInstanceID)
	}
}

func TestMQChannelConsumesInboundMessages(t *testing.T) {
	workspace := t.TempDir()
	bus := messagebus.NewMessageBus()
	inboundQueue, err := bus.Get(messagebus.InboundQueue)
	if err != nil {
		t.Fatalf("Get(InboundQueue) error = %v", err)
	}

	broker := newFakeMQBroker()
	ch := newMQChannelWithFactory(config.MQChannelConfig{
		ChannelConfig: config.ChannelConfig{Enabled: true},
		URL:           "amqp://guest:guest@localhost:5672/",
		Exchange:      "agent.bus",
		Profile:       "writer",
		MachineID:     "machine-a",
	}, bus, workspace, func(cfg mqResolvedConfig) (mqBroker, error) {
		return broker, nil
	})
	if err := ch.Start(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer ch.Stop()

	acked := false
	broker.deliveries <- mqDelivery{
		Body: []byte(`{
			"version": 1,
			"message_id": "msg-1",
			"message_type": "direct",
			"source_profile": "planner",
			"source_instance_id": "planner@machine-b",
			"target_profile": "writer",
			"conversation_id": "conv-1",
			"correlation_id": "msg-1",
			"created_at": "2026-03-21T12:00:00Z",
			"body": "hello"
		}`),
		RoutingKey: "profile.writer",
		Ack: func() error {
			acked = true
			return nil
		},
		Nack: func(bool) error { return nil },
	}

	select {
	case inbound := <-inboundQueue:
		if inbound.ChannelID != mqChannelName {
			t.Fatalf("ChannelID = %q, want %q", inbound.ChannelID, mqChannelName)
		}
		if inbound.ChatID != "conv-1" {
			t.Fatalf("ChatID = %q, want conv-1", inbound.ChatID)
		}
		if inbound.SenderID != "planner" {
			t.Fatalf("SenderID = %q, want planner", inbound.SenderID)
		}
		if inbound.Metadata["mq_message_id"] != "msg-1" {
			t.Fatalf("mq_message_id = %q, want msg-1", inbound.Metadata["mq_message_id"])
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for inbound MQ message")
	}

	if !acked {
		t.Fatal("delivery was not acked")
	}
}

func TestMQChannelSendPublishesReplyEnvelope(t *testing.T) {
	workspace := t.TempDir()
	bus := messagebus.NewMessageBus()
	broker := newFakeMQBroker()
	ch := newMQChannelWithFactory(config.MQChannelConfig{
		ChannelConfig: config.ChannelConfig{Enabled: true},
		URL:           "amqp://guest:guest@localhost:5672/",
		Exchange:      "agent.bus",
		Profile:       "writer",
		MachineID:     "machine-a",
	}, bus, workspace, func(cfg mqResolvedConfig) (mqBroker, error) {
		return broker, nil
	})
	if err := ch.Start(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer ch.Stop()

	err := ch.Send(messagebus.Message{
		ChannelID: mqChannelName,
		ChatID:    "conv-1",
		Message:   "ack",
		Metadata: map[string]string{
			"mq_message_id":      "msg-1",
			"mq_source_profile":  "planner",
			"mq_conversation_id": "conv-1",
			"mq_correlation_id":  "msg-root",
		},
	})
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}
	if broker.publishedCount() != 1 {
		t.Fatalf("len(published) = %d, want 1", broker.publishedCount())
	}
	published := broker.publishedMessage(0)
	if published.routingKey != "profile.planner" {
		t.Fatalf("routingKey = %q, want profile.planner", published.routingKey)
	}

	var envelope mqEnvelope
	if err := json.Unmarshal(published.body, &envelope); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if envelope.MessageType != "reply" {
		t.Fatalf("MessageType = %q, want reply", envelope.MessageType)
	}
	if envelope.SourceProfile != "writer" {
		t.Fatalf("SourceProfile = %q, want writer", envelope.SourceProfile)
	}
	if envelope.TargetProfile != "planner" {
		t.Fatalf("TargetProfile = %q, want planner", envelope.TargetProfile)
	}
	if envelope.InReplyToMessageID != "msg-1" {
		t.Fatalf("InReplyToMessageID = %q, want msg-1", envelope.InReplyToMessageID)
	}
}

func TestMQChannelOutboxLoopPublishesQueuedFiles(t *testing.T) {
	workspace := t.TempDir()
	bus := messagebus.NewMessageBus()
	broker := newFakeMQBroker()
	ch := newMQChannelWithFactory(config.MQChannelConfig{
		ChannelConfig: config.ChannelConfig{Enabled: true},
		URL:           "amqp://guest:guest@localhost:5672/",
		Exchange:      "agent.bus",
		Profile:       "writer",
		MachineID:     "machine-a",
	}, bus, workspace, func(cfg mqResolvedConfig) (mqBroker, error) {
		return broker, nil
	})
	if err := ch.Start(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer ch.Stop()

	outboxPath := filepath.Join(workspace, mqRuntimeDirName, mqOutboxDirName, "queued.json")
	if err := os.WriteFile(outboxPath, []byte(`{
		"version": 1,
		"message_id": "msg-2",
		"message_type": "direct",
		"source_profile": "writer",
		"source_instance_id": "writer@machine-a",
		"target_profile": "planner",
		"conversation_id": "conv-2",
		"correlation_id": "msg-2",
		"created_at": "2026-03-21T12:05:00Z",
		"body": "ping"
	}`), 0644); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if broker.publishedCount() == 1 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if broker.publishedCount() != 1 {
		t.Fatalf("len(published) = %d, want 1", broker.publishedCount())
	}
	sentPath := filepath.Join(workspace, mqRuntimeDirName, mqSentDirName, "queued.json")
	if _, err := os.Stat(sentPath); err != nil {
		t.Fatalf("expected sent file at %s: %v", sentPath, err)
	}
}
