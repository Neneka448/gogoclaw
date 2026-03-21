package channels

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Neneka448/gogoclaw/internal/config"
	messagebus "github.com/Neneka448/gogoclaw/internal/message_bus"
	amqp "github.com/rabbitmq/amqp091-go"
)

const (
	mqChannelName            = "mq"
	mqExchangeType           = "topic"
	mqBroadcastRoutingKey    = "broadcast"
	mqDirectRoutingKeyPrefix = "profile."
	mqInboxQueuePrefix       = "agent.inbox."
	mqRuntimeDirName         = ".gogoclaw/agent_bus"
	mqRuntimeConfigFileName  = "config.json"
	mqOutboxDirName          = "outbox"
	mqSentDirName            = "sent"
	mqFailedDirName          = "failed"
	mqOutboxPollInterval     = time.Second
	mqEnvelopeVersion        = 1
)

type mqEnvelope struct {
	Version            int               `json:"version"`
	MessageID          string            `json:"message_id"`
	MessageType        string            `json:"message_type"`
	SourceProfile      string            `json:"source_profile"`
	SourceInstanceID   string            `json:"source_instance_id"`
	TargetProfile      string            `json:"target_profile,omitempty"`
	ConversationID     string            `json:"conversation_id"`
	CorrelationID      string            `json:"correlation_id"`
	InReplyToMessageID string            `json:"in_reply_to_message_id,omitempty"`
	CreatedAt          string            `json:"created_at"`
	Body               string            `json:"body"`
	Metadata           map[string]string `json:"metadata,omitempty"`
}

type mqRuntimeConfigFile struct {
	Version          int    `json:"version"`
	SourceProfile    string `json:"source_profile"`
	SourceInstanceID string `json:"source_instance_id"`
	RuntimeDir       string `json:"runtime_dir"`
	OutboxDir        string `json:"outbox_dir"`
	SentDir          string `json:"sent_dir"`
	FailedDir        string `json:"failed_dir"`
}

type mqResolvedConfig struct {
	URL           string
	Exchange      string
	Profile       string
	MachineID     string
	InstanceID    string
	QueueName     string
	Prefetch      int
	ConsumerTag   string
	Workspace     string
	RuntimeDir    string
	OutboxDir     string
	SentDir       string
	FailedDir     string
	RuntimeConfig string
}

type mqDelivery struct {
	Body       []byte
	RoutingKey string
	Ack        func() error
	Nack       func(requeue bool) error
}

type mqBroker interface {
	Consume() (<-chan mqDelivery, error)
	Publish(routingKey string, body []byte) error
	Close() error
}

type mqBrokerFactory func(mqResolvedConfig) (mqBroker, error)

type MQChannel struct {
	config        config.MQChannelConfig
	messageBus    messagebus.MessageBus
	workspace     string
	brokerFactory mqBrokerFactory

	mu       sync.Mutex
	broker   mqBroker
	resolved mqResolvedConfig
	stopCh   chan struct{}
	started  bool
	wg       sync.WaitGroup
}

func NewMQChannel(cfg config.MQChannelConfig, bus messagebus.MessageBus, workspace string) *MQChannel {
	return newMQChannelWithFactory(cfg, bus, workspace, newAMQPBroker)
}

func newMQChannelWithFactory(cfg config.MQChannelConfig, bus messagebus.MessageBus, workspace string, factory mqBrokerFactory) *MQChannel {
	if factory == nil {
		factory = newAMQPBroker
	}
	return &MQChannel{
		config:        cfg,
		messageBus:    bus,
		workspace:     strings.TrimSpace(workspace),
		brokerFactory: factory,
	}
}

func (c *MQChannel) Name() string {
	return mqChannelName
}

func (c *MQChannel) Enabled() bool {
	return c.config.Enabled
}

func (c *MQChannel) Start() error {
	if !c.Enabled() {
		return nil
	}

	c.mu.Lock()
	if c.started {
		c.mu.Unlock()
		return nil
	}

	resolved, err := resolveMQConfig(c.config, c.workspace)
	if err != nil {
		c.mu.Unlock()
		return err
	}
	if err := ensureMQRuntimeFiles(resolved); err != nil {
		c.mu.Unlock()
		return err
	}

	broker, err := c.brokerFactory(resolved)
	if err != nil {
		c.mu.Unlock()
		return err
	}
	deliveries, err := broker.Consume()
	if err != nil {
		c.mu.Unlock()
		_ = broker.Close()
		return err
	}

	c.broker = broker
	c.resolved = resolved
	c.stopCh = make(chan struct{})
	c.started = true
	stopCh := c.stopCh
	c.wg.Add(2)
	go c.consumeLoop(stopCh, deliveries)
	go c.outboxLoop(stopCh)
	c.mu.Unlock()

	return nil
}

func (c *MQChannel) Stop() error {
	c.mu.Lock()
	if !c.started {
		c.mu.Unlock()
		return nil
	}
	stopCh := c.stopCh
	broker := c.broker
	c.stopCh = nil
	c.broker = nil
	c.started = false
	c.mu.Unlock()

	if stopCh != nil {
		close(stopCh)
	}
	if broker != nil {
		_ = broker.Close()
	}
	c.wg.Wait()
	return nil
}

func (c *MQChannel) Send(message messagebus.Message) error {
	if !c.Enabled() {
		return fmt.Errorf("mq channel disabled")
	}
	envelope, err := c.buildOutboundEnvelope(message)
	if err != nil {
		return err
	}
	body, err := json.Marshal(envelope)
	if err != nil {
		return err
	}

	c.mu.Lock()
	broker := c.broker
	c.mu.Unlock()
	if broker == nil {
		return fmt.Errorf("mq channel is not started")
	}
	return broker.Publish(routingKeyForEnvelope(envelope), body)
}

func (c *MQChannel) consumeLoop(stopCh <-chan struct{}, deliveries <-chan mqDelivery) {
	defer c.wg.Done()

	for {
		select {
		case <-stopCh:
			return
		case delivery, ok := <-deliveries:
			if !ok {
				return
			}
			if err := c.handleDelivery(delivery); err != nil {
				_, _ = fmt.Fprintf(os.Stderr, "[mq] inbound error: %v\n", err)
			}
		}
	}
}

func (c *MQChannel) handleDelivery(delivery mqDelivery) error {
	envelope, err := decodeMQEnvelope(delivery.Body)
	if err != nil {
		if delivery.Nack != nil {
			_ = delivery.Nack(false)
		}
		return err
	}
	if envelope.TargetProfile != "" && envelope.TargetProfile != c.resolved.Profile && envelope.MessageType != "broadcast" {
		if delivery.Nack != nil {
			_ = delivery.Nack(false)
		}
		return fmt.Errorf("message target %q does not match local profile %q", envelope.TargetProfile, c.resolved.Profile)
	}

	c.mu.Lock()
	bus := c.messageBus
	c.mu.Unlock()
	if bus == nil {
		if delivery.Nack != nil {
			_ = delivery.Nack(false)
		}
		return fmt.Errorf("message bus is not initialized")
	}

	msg := messagebus.Message{
		ChannelID:   mqChannelName,
		Message:     envelope.Body,
		MessageID:   envelope.MessageID,
		MessageType: envelope.MessageType,
		ChatID:      envelope.ConversationID,
		SenderID:    envelope.SourceProfile,
		ReplyTo:     envelope.InReplyToMessageID,
		Metadata: map[string]string{
			"mq_message_id":             envelope.MessageID,
			"mq_message_type":           envelope.MessageType,
			"mq_source_profile":         envelope.SourceProfile,
			"mq_source_instance_id":     envelope.SourceInstanceID,
			"mq_target_profile":         envelope.TargetProfile,
			"mq_conversation_id":        envelope.ConversationID,
			"mq_correlation_id":         envelope.CorrelationID,
			"mq_in_reply_to_message_id": envelope.InReplyToMessageID,
			"mq_created_at":             envelope.CreatedAt,
			"mq_routing_key":            delivery.RoutingKey,
		},
	}
	for key, value := range envelope.Metadata {
		msg.Metadata["mq_meta_"+key] = value
	}

	if err := bus.Put(msg, messagebus.InboundQueue); err != nil {
		if delivery.Nack != nil {
			_ = delivery.Nack(false)
		}
		return err
	}
	if delivery.Ack != nil {
		return delivery.Ack()
	}
	return nil
}

func (c *MQChannel) outboxLoop(stopCh <-chan struct{}) {
	defer c.wg.Done()

	ticker := time.NewTicker(mqOutboxPollInterval)
	defer ticker.Stop()

	for {
		if err := c.flushOutbox(); err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "[mq] outbox flush error: %v\n", err)
		}
		select {
		case <-stopCh:
			return
		case <-ticker.C:
		}
	}
}

func (c *MQChannel) flushOutbox() error {
	c.mu.Lock()
	broker := c.broker
	resolved := c.resolved
	c.mu.Unlock()

	if broker == nil {
		return nil
	}

	entries, err := os.ReadDir(resolved.OutboxDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(strings.ToLower(entry.Name()), ".json") {
			continue
		}
		names = append(names, entry.Name())
	}
	sort.Strings(names)

	for _, name := range names {
		path := filepath.Join(resolved.OutboxDir, name)
		if err := c.publishOutboxFile(broker, resolved, path, name); err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "[mq] publish outbox %s: %v\n", path, err)
		}
	}
	return nil
}

func (c *MQChannel) publishOutboxFile(broker mqBroker, resolved mqResolvedConfig, path string, name string) error {
	content, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	envelope, err := decodeMQEnvelope(content)
	if err != nil {
		return moveMQRuntimeFile(path, filepath.Join(resolved.FailedDir, name))
	}
	body, err := json.Marshal(envelope)
	if err != nil {
		return moveMQRuntimeFile(path, filepath.Join(resolved.FailedDir, name))
	}
	if err := broker.Publish(routingKeyForEnvelope(envelope), body); err != nil {
		return moveMQRuntimeFile(path, filepath.Join(resolved.FailedDir, name))
	}
	return moveMQRuntimeFile(path, filepath.Join(resolved.SentDir, name))
}

func (c *MQChannel) buildOutboundEnvelope(message messagebus.Message) (mqEnvelope, error) {
	c.mu.Lock()
	resolved := c.resolved
	c.mu.Unlock()
	if strings.TrimSpace(resolved.Profile) == "" {
		return mqEnvelope{}, fmt.Errorf("mq profile is not configured")
	}

	body := strings.TrimSpace(message.Message)
	if body == "" {
		return mqEnvelope{}, fmt.Errorf("mq outbound body is empty")
	}

	metadata := message.Metadata
	messageID := newMQID("msg")
	conversationID := firstMQNonEmpty(metadataValue(metadata, "mq_conversation_id"), message.ChatID, messageID)
	correlationID := firstMQNonEmpty(metadataValue(metadata, "mq_correlation_id"), metadataValue(metadata, "mq_message_id"), messageID)
	targetProfile := firstMQNonEmpty(metadataValue(metadata, "mq_source_profile"), metadataValue(metadata, "target_profile"))
	if targetProfile == "" {
		return mqEnvelope{}, fmt.Errorf("mq outbound target profile is required")
	}
	inReplyTo := firstMQNonEmpty(metadataValue(metadata, "mq_message_id"), message.ReplyTo)
	messageType := "direct"
	if inReplyTo != "" {
		messageType = "reply"
	}

	envelope := mqEnvelope{
		Version:            mqEnvelopeVersion,
		MessageID:          messageID,
		MessageType:        messageType,
		SourceProfile:      resolved.Profile,
		SourceInstanceID:   resolved.InstanceID,
		TargetProfile:      targetProfile,
		ConversationID:     conversationID,
		CorrelationID:      correlationID,
		InReplyToMessageID: inReplyTo,
		CreatedAt:          time.Now().UTC().Format(time.RFC3339),
		Body:               body,
		Metadata:           filterOutboundMetadata(metadata, message),
	}
	return envelope, nil
}

func filterOutboundMetadata(metadata map[string]string, message messagebus.Message) map[string]string {
	filtered := make(map[string]string)
	for key, value := range metadata {
		if strings.HasPrefix(key, "mq_") {
			continue
		}
		if strings.TrimSpace(value) == "" {
			continue
		}
		filtered[key] = value
	}
	if strings.TrimSpace(message.FinishReason) != "" {
		filtered["finish_reason"] = message.FinishReason
	}
	if len(message.MediaPaths) > 0 {
		encoded, err := json.Marshal(message.MediaPaths)
		if err == nil {
			filtered["media_paths"] = string(encoded)
		}
	}
	if len(filtered) == 0 {
		return nil
	}
	return filtered
}

func metadataValue(metadata map[string]string, key string) string {
	if len(metadata) == 0 {
		return ""
	}
	return strings.TrimSpace(metadata[key])
}

func decodeMQEnvelope(content []byte) (mqEnvelope, error) {
	var envelope mqEnvelope
	if err := json.Unmarshal(content, &envelope); err != nil {
		return mqEnvelope{}, err
	}
	if envelope.Version == 0 {
		envelope.Version = mqEnvelopeVersion
	}
	if strings.TrimSpace(envelope.MessageID) == "" {
		return mqEnvelope{}, fmt.Errorf("message_id is required")
	}
	if strings.TrimSpace(envelope.MessageType) == "" {
		return mqEnvelope{}, fmt.Errorf("message_type is required")
	}
	if strings.TrimSpace(envelope.SourceProfile) == "" {
		return mqEnvelope{}, fmt.Errorf("source_profile is required")
	}
	if strings.TrimSpace(envelope.ConversationID) == "" {
		envelope.ConversationID = envelope.MessageID
	}
	if strings.TrimSpace(envelope.CorrelationID) == "" {
		envelope.CorrelationID = envelope.MessageID
	}
	if strings.TrimSpace(envelope.CreatedAt) == "" {
		envelope.CreatedAt = time.Now().UTC().Format(time.RFC3339)
	}
	if envelope.Metadata == nil {
		envelope.Metadata = nil
	}
	return envelope, nil
}

func resolveMQConfig(cfg config.MQChannelConfig, workspace string) (mqResolvedConfig, error) {
	workspace = strings.TrimSpace(workspace)
	if workspace == "" {
		return mqResolvedConfig{}, fmt.Errorf("mq workspace is required")
	}
	profile := strings.TrimSpace(cfg.Profile)
	if profile == "" {
		return mqResolvedConfig{}, fmt.Errorf("mq profile is required")
	}
	machineID := strings.TrimSpace(cfg.MachineID)
	if machineID == "" {
		hostname, err := os.Hostname()
		if err != nil {
			return mqResolvedConfig{}, err
		}
		machineID = hostname
	}
	instanceID := profile + "@" + machineID
	runtimeDir := filepath.Join(workspace, mqRuntimeDirName)
	return mqResolvedConfig{
		URL:           strings.TrimSpace(cfg.URL),
		Exchange:      strings.TrimSpace(cfg.Exchange),
		Profile:       profile,
		MachineID:     machineID,
		InstanceID:    instanceID,
		QueueName:     mqInboxQueuePrefix + profile,
		Prefetch:      cfg.Prefetch,
		ConsumerTag:   instanceID,
		Workspace:     workspace,
		RuntimeDir:    runtimeDir,
		OutboxDir:     filepath.Join(runtimeDir, mqOutboxDirName),
		SentDir:       filepath.Join(runtimeDir, mqSentDirName),
		FailedDir:     filepath.Join(runtimeDir, mqFailedDirName),
		RuntimeConfig: filepath.Join(runtimeDir, mqRuntimeConfigFileName),
	}, nil
}

func ensureMQRuntimeFiles(cfg mqResolvedConfig) error {
	for _, dir := range []string{cfg.RuntimeDir, cfg.OutboxDir, cfg.SentDir, cfg.FailedDir} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
	}
	content, err := json.MarshalIndent(mqRuntimeConfigFile{
		Version:          mqEnvelopeVersion,
		SourceProfile:    cfg.Profile,
		SourceInstanceID: cfg.InstanceID,
		RuntimeDir:       cfg.RuntimeDir,
		OutboxDir:        cfg.OutboxDir,
		SentDir:          cfg.SentDir,
		FailedDir:        cfg.FailedDir,
	}, "", "  ")
	if err != nil {
		return err
	}
	content = append(content, '\n')
	return os.WriteFile(cfg.RuntimeConfig, content, 0644)
}

func routingKeyForEnvelope(envelope mqEnvelope) string {
	if strings.EqualFold(strings.TrimSpace(envelope.MessageType), "broadcast") || strings.TrimSpace(envelope.TargetProfile) == "" {
		return mqBroadcastRoutingKey
	}
	return mqDirectRoutingKeyPrefix + strings.TrimSpace(envelope.TargetProfile)
}

func moveMQRuntimeFile(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}
	if _, err := os.Stat(dst); err == nil {
		dst = filepath.Join(filepath.Dir(dst), newMQID("dup")+"-"+filepath.Base(dst))
	}
	return os.Rename(src, dst)
}

func firstMQNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func newMQID(prefix string) string {
	buf := make([]byte, 6)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano())
	}
	return fmt.Sprintf("%s-%d-%s", prefix, time.Now().UTC().UnixMilli(), hex.EncodeToString(buf))
}

type amqpBroker struct {
	exchange   string
	conn       *amqp.Connection
	consumeCh  *amqp.Channel
	publishCh  *amqp.Channel
	deliveries chan mqDelivery
	mu         sync.Mutex
}

func newAMQPBroker(cfg mqResolvedConfig) (mqBroker, error) {
	conn, err := amqp.Dial(cfg.URL)
	if err != nil {
		return nil, err
	}
	consumeCh, err := conn.Channel()
	if err != nil {
		_ = conn.Close()
		return nil, err
	}
	publishCh, err := conn.Channel()
	if err != nil {
		_ = consumeCh.Close()
		_ = conn.Close()
		return nil, err
	}

	for _, ch := range []*amqp.Channel{consumeCh, publishCh} {
		if err := ch.ExchangeDeclare(cfg.Exchange, mqExchangeType, true, false, false, false, nil); err != nil {
			_ = publishCh.Close()
			_ = consumeCh.Close()
			_ = conn.Close()
			return nil, err
		}
	}
	if cfg.Prefetch > 0 {
		if err := consumeCh.Qos(cfg.Prefetch, 0, false); err != nil {
			_ = publishCh.Close()
			_ = consumeCh.Close()
			_ = conn.Close()
			return nil, err
		}
	}
	queue, err := consumeCh.QueueDeclare(cfg.QueueName, true, false, false, false, nil)
	if err != nil {
		_ = publishCh.Close()
		_ = consumeCh.Close()
		_ = conn.Close()
		return nil, err
	}
	for _, key := range []string{mqDirectRoutingKeyPrefix + cfg.Profile, mqBroadcastRoutingKey} {
		if err := consumeCh.QueueBind(queue.Name, key, cfg.Exchange, false, nil); err != nil {
			_ = publishCh.Close()
			_ = consumeCh.Close()
			_ = conn.Close()
			return nil, err
		}
	}
	rawDeliveries, err := consumeCh.Consume(queue.Name, cfg.ConsumerTag, false, false, false, false, nil)
	if err != nil {
		_ = publishCh.Close()
		_ = consumeCh.Close()
		_ = conn.Close()
		return nil, err
	}

	broker := &amqpBroker{
		exchange:   cfg.Exchange,
		conn:       conn,
		consumeCh:  consumeCh,
		publishCh:  publishCh,
		deliveries: make(chan mqDelivery),
	}
	go func() {
		defer close(broker.deliveries)
		for delivery := range rawDeliveries {
			raw := delivery
			broker.deliveries <- mqDelivery{
				Body:       raw.Body,
				RoutingKey: raw.RoutingKey,
				Ack: func() error {
					return raw.Ack(false)
				},
				Nack: func(requeue bool) error {
					return raw.Nack(false, requeue)
				},
			}
		}
	}()
	return broker, nil
}

func (b *amqpBroker) Consume() (<-chan mqDelivery, error) {
	return b.deliveries, nil
}

func (b *amqpBroker) Publish(routingKey string, body []byte) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.publishCh == nil {
		return fmt.Errorf("mq publisher is closed")
	}
	return b.publishCh.Publish(b.exchange, routingKey, false, false, amqp.Publishing{
		Headers:         amqp.Table{},
		ContentType:     "application/json",
		ContentEncoding: "utf-8",
		DeliveryMode:    amqp.Persistent,
		Timestamp:       time.Now().UTC(),
		Body:            body,
	})
}

func (b *amqpBroker) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	var firstErr error
	if b.publishCh != nil {
		if err := b.publishCh.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
		b.publishCh = nil
	}
	if b.consumeCh != nil {
		if err := b.consumeCh.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
		b.consumeCh = nil
	}
	if b.conn != nil {
		if err := b.conn.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
		b.conn = nil
	}
	return firstErr
}
