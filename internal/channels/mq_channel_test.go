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

func TestMQChannelAppliesReturnRouteToInboundMessage(t *testing.T) {
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
		Profile:       "front",
		MachineID:     "machine-a",
	}, bus, workspace, func(cfg mqResolvedConfig) (mqBroker, error) {
		return broker, nil
	})
	if err := ch.Start(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer ch.Stop()

	broker.deliveries <- mqDelivery{
		Body: []byte(`{
			"version": 1,
			"message_id": "msg-complete",
			"message_type": "direct",
			"source_profile": "worker",
			"source_instance_id": "worker@machine-b",
			"target_profile": "front",
			"conversation_id": "inv-1",
			"correlation_id": "inv-1",
			"created_at": "2026-03-21T12:10:00Z",
			"body": "SYSTEM EVENT: delegated task completed",
			"metadata": {
				"return_channel_id": "feishu",
				"return_chat_id": "oc_chat_123",
				"return_message_type": "text",
				"return_reply_to": "om_parent",
				"return_sender_id": "user-debug4"
			}
		}`),
		RoutingKey: "profile.front",
		Ack:        func() error { return nil },
		Nack:       func(bool) error { return nil },
	}

	select {
	case inbound := <-inboundQueue:
		if inbound.ChannelID != "feishu" {
			t.Fatalf("ChannelID = %q, want feishu", inbound.ChannelID)
		}
		if inbound.ChatID != "oc_chat_123" {
			t.Fatalf("ChatID = %q, want oc_chat_123", inbound.ChatID)
		}
		if inbound.ReplyTo != "om_parent" {
			t.Fatalf("ReplyTo = %q, want om_parent", inbound.ReplyTo)
		}
		if inbound.Metadata["agent_profile"] != "front" {
			t.Fatalf("agent_profile = %q, want front", inbound.Metadata["agent_profile"])
		}
		if inbound.Metadata["mq_return_route_applied"] != "true" {
			t.Fatalf("mq_return_route_applied = %q, want true", inbound.Metadata["mq_return_route_applied"])
		}
		if inbound.Metadata["target_profile"] != "user-debug4" {
			t.Fatalf("target_profile = %q, want user-debug4", inbound.Metadata["target_profile"])
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for inbound MQ message with return route")
	}
}

func TestMQChannelSendPrefersExplicitTargetProfile(t *testing.T) {
	workspace := t.TempDir()
	bus := messagebus.NewMessageBus()
	broker := newFakeMQBroker()
	ch := newMQChannelWithFactory(config.MQChannelConfig{
		ChannelConfig: config.ChannelConfig{Enabled: true},
		URL:           "amqp://guest:guest@localhost:5672/",
		Exchange:      "agent.bus",
		Profile:       "front",
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
		Message:   "done",
		FinishReason: "stop",
		Metadata: map[string]string{
			"mq_message_id":      "msg-worker",
			"mq_source_profile":  "worker",
			"target_profile":     "user-debug4",
			"mq_conversation_id": "conv-1",
			"mq_correlation_id":  "msg-worker",
		},
	})
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}
	if broker.publishedCount() != 1 {
		t.Fatalf("publishedCount = %d, want 1", broker.publishedCount())
	}
	published := broker.publishedMessage(0)
	if published.routingKey != "profile.user-debug4" {
		t.Fatalf("routingKey = %q, want profile.user-debug4", published.routingKey)
	}
	var envelope mqEnvelope
	if err := json.Unmarshal(published.body, &envelope); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if envelope.TargetProfile != "user-debug4" {
		t.Fatalf("TargetProfile = %q, want user-debug4", envelope.TargetProfile)
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
		FinishReason: "stop",
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

func TestMQChannelSendSkipsToolResultMessages(t *testing.T) {
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
		Message:   "tool output",
		Metadata: map[string]string{
			"target_profile": "planner",
			"message_kind":   "tool_result",
		},
	})
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}
	if broker.publishedCount() != 0 {
		t.Fatalf("publishedCount = %d, want 0", broker.publishedCount())
	}
}

func TestMQChannelSendSkipsToolCallMessages(t *testing.T) {
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
		ChannelID:    mqChannelName,
		ChatID:       "conv-1",
		Message:      "get_skill({\"name\":\"invoke_agent\"})",
		FinishReason: "tool_calls",
		Metadata: map[string]string{
			"target_profile": "planner",
		},
	})
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}
	if broker.publishedCount() != 0 {
		t.Fatalf("publishedCount = %d, want 0", broker.publishedCount())
	}
}

func TestMQChannelSendUsesReturnRouteConversationAndCorrelation(t *testing.T) {
	workspace := t.TempDir()
	bus := messagebus.NewMessageBus()
	broker := newFakeMQBroker()
	ch := newMQChannelWithFactory(config.MQChannelConfig{
		ChannelConfig: config.ChannelConfig{Enabled: true},
		URL:           "amqp://guest:guest@localhost:5672/",
		Exchange:      "agent.bus",
		Profile:       "front",
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
		ChatID:    "user-conv-1",
		MessageID: "root-msg-1",
		ReplyTo:   "completion-msg-1",
		Message:   "final answer",
		Metadata: map[string]string{
			"mq_return_route_applied": "true",
			"mq_conversation_id":      "user-conv-1",
			"mq_correlation_id":       "root-msg-1",
			"target_profile":          "user-debug5",
		},
		FinishReason: "stop",
	})
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}
	if broker.publishedCount() != 1 {
		t.Fatalf("publishedCount = %d, want 1", broker.publishedCount())
	}

	var envelope mqEnvelope
	if err := json.Unmarshal(broker.publishedMessage(0).body, &envelope); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if envelope.ConversationID != "user-conv-1" {
		t.Fatalf("ConversationID = %q, want user-conv-1", envelope.ConversationID)
	}
	if envelope.CorrelationID != "root-msg-1" {
		t.Fatalf("CorrelationID = %q, want root-msg-1", envelope.CorrelationID)
	}
	if envelope.InReplyToMessageID != "root-msg-1" {
		t.Fatalf("InReplyToMessageID = %q, want root-msg-1", envelope.InReplyToMessageID)
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
