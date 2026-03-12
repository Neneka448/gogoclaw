package channels

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/Neneka448/gogoclaw/internal/config"
	messagebus "github.com/Neneka448/gogoclaw/internal/message_bus"
	lark "github.com/larksuite/oapi-sdk-go/v3"
	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
	"github.com/larksuite/oapi-sdk-go/v3/event/dispatcher"
	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"
	larkws "github.com/larksuite/oapi-sdk-go/v3/ws"
)

type FeishuChannel struct {
	config     config.FeishuChannelConfig
	messageBus messagebus.MessageBus
	client     *lark.Client
	wsClient   *larkws.Client
	startOnce  sync.Once
	started    bool
	mu         sync.Mutex
	messageIDs []string
	seen       map[string]struct{}
}


func NewFeishuChannel(cfg config.FeishuChannelConfig, bus messagebus.MessageBus) *FeishuChannel {
	return &FeishuChannel{
		config:     cfg,
		messageBus: bus,
		seen:       make(map[string]struct{}),
	}
}

func (c *FeishuChannel) SetMessageBus(bus messagebus.MessageBus) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.messageBus = bus
}

func (c *FeishuChannel) Name() string {
	return "feishu"
}

func (c *FeishuChannel) Enabled() bool {
	return c.config.Enabled
}

func (c *FeishuChannel) Start() error {
	if !c.Enabled() {
		return nil
	}
	if strings.TrimSpace(c.config.AppID) == "" || strings.TrimSpace(c.config.AppSecret) == "" {
		return fmt.Errorf("feishu channel requires appId and appSecret")
	}

	var startErr error
	c.startOnce.Do(func() {
		c.client = lark.NewClient(c.config.AppID, c.config.AppSecret)
		eventHandler := dispatcher.NewEventDispatcher(c.config.VerificationToken, c.config.EncryptKey).
			OnP2MessageReceiveV1(c.handleMessageReceive)

		c.wsClient = larkws.NewClient(
			c.config.AppID,
			c.config.AppSecret,
			larkws.WithEventHandler(eventHandler),
			larkws.WithLogLevel(larkcore.LogLevelError),
		)
		c.started = true
		go func() {
			_ = c.wsClient.Start(context.Background())
		}()
	})

	return startErr
}

func (c *FeishuChannel) Stop() error {
	return nil
}

func (c *FeishuChannel) Send(message messagebus.Message) error {
	if !c.Enabled() {
		return fmt.Errorf("feishu channel disabled")
	}
	if strings.TrimSpace(message.Message) == "" {
		return nil
	}
	if c.client == nil {
		c.client = lark.NewClient(c.config.AppID, c.config.AppSecret)
	}

	receiveIDType := receiveIDTypeForChatID(message.ChatID)

	content := larkim.NewTextMsgBuilder().Text(message.Message).Build()
	resp, err := c.client.Im.Message.Create(context.Background(), larkim.NewCreateMessageReqBuilder().
		ReceiveIdType(receiveIDType).
		Body(larkim.NewCreateMessageReqBodyBuilder().
			ReceiveId(message.ChatID).
			MsgType(larkim.MsgTypeText).
			Content(content).
			Build()).
		Build())
	if err != nil {
		return err
	}
	if !resp.Success() {
		return fmt.Errorf("feishu send message failed: code=%d msg=%s request_id=%s", resp.Code, resp.Msg, resp.RequestId())
	}
	return nil
}

func receiveIDTypeForChatID(chatID string) string {
	if strings.HasPrefix(chatID, "oc_") {
		return "chat_id"
	}
	return "open_id"
}

func (c *FeishuChannel) handleMessageReceive(_ context.Context, event *larkim.P2MessageReceiveV1) error {
	payload, err := json.Marshal(event)
	if err != nil {
		return err
	}

	var body map[string]any
	if err := json.Unmarshal(payload, &body); err != nil {
		return err
	}

	messageID := firstString(body,
		"event.message.message_id",
		"event.message.messageId",
		"message.message_id",
		"message.messageId",
	)
	if messageID != "" && !c.markMessage(messageID) {
		return nil
	}

	senderID := firstString(body,
		"event.sender.sender_id.open_id",
		"event.sender.sender_id.user_id",
		"sender.sender_id.open_id",
		"sender.sender_id.user_id",
	)
	if !c.isAllowed(senderID) {
		return nil
	}

	chatID := firstString(body,
		"event.message.chat_id",
		"event.message.chatId",
		"message.chat_id",
		"message.chatId",
	)
	messageType := firstString(body,
		"event.message.message_type",
		"event.message.messageType",
		"message.message_type",
		"message.messageType",
	)
	content := extractFeishuContent(messageType, firstString(body,
		"event.message.content",
		"message.content",
	))
	if strings.TrimSpace(content) == "" {
		return nil
	}

	metadata := map[string]string{}
	if chatType := firstString(body,
		"event.message.chat_type",
		"event.message.chatType",
		"message.chat_type",
		"message.chatType",
	); chatType != "" {
		metadata["chat_type"] = chatType
	}
	if messageID != "" {
		metadata["message_id"] = messageID
	}

	if c.messageBus == nil {
		return nil
	}

	return c.messageBus.Put(messagebus.Message{
		ChannelID:   c.Name(),
		Message:     content,
		MessageID:   messageID,
		MessageType: messageType,
		ChatID:      chatID,
		SenderID:    senderID,
		Metadata:    metadata,
	}, messagebus.InboundQueue)
}

func (c *FeishuChannel) isAllowed(senderID string) bool {
	allowList := c.config.AllowFrom
	if len(allowList) == 0 {
		return false
	}
	if senderID == "" {
		return false
	}
	for _, allow := range allowList {
		if allow == "*" || allow == senderID {
			return true
		}
	}
	return false
}

func (c *FeishuChannel) markMessage(messageID string) bool {
	if messageID == "" {
		return true
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, exists := c.seen[messageID]; exists {
		return false
	}
	c.seen[messageID] = struct{}{}
	c.messageIDs = append(c.messageIDs, messageID)
	if len(c.messageIDs) > 512 {
		delete(c.seen, c.messageIDs[0])
		c.messageIDs = c.messageIDs[1:]
	}
	return true
}

func extractFeishuContent(messageType string, raw string) string {
	if strings.TrimSpace(raw) == "" {
		return ""
	}

	var content map[string]any
	if err := json.Unmarshal([]byte(raw), &content); err != nil {
		return raw
	}

	switch messageType {
	case "text":
		return firstString(content, "text")
	case "post":
		return flattenFeishuPost(content)
	case "interactive":
		if title := firstString(content, "title.content", "header.title.content", "header.title.text"); title != "" {
			return title
		}
		encoded, _ := json.Marshal(content)
		return string(encoded)
	default:
		if text := firstString(content, "text"); text != "" {
			return text
		}
		encoded, _ := json.Marshal(content)
		return string(encoded)
	}
}

func flattenFeishuPost(content map[string]any) string {
	postRoot := content
	if nested, ok := content["post"].(map[string]any); ok {
		postRoot = nested
	}
	for _, locale := range []string{"zh_cn", "en_us", "ja_jp"} {
		if block, ok := postRoot[locale].(map[string]any); ok {
			return flattenFeishuPostBlock(block)
		}
	}
	return flattenFeishuPostBlock(postRoot)
}

func flattenFeishuPostBlock(content map[string]any) string {
	parts := make([]string, 0, 8)
	if title := firstString(content, "title"); title != "" {
		parts = append(parts, title)
	}
	rows, ok := content["content"].([]any)
	if !ok {
		return strings.TrimSpace(strings.Join(parts, "\n"))
	}
	for _, row := range rows {
		columns, ok := row.([]any)
		if !ok {
			continue
		}
		for _, column := range columns {
			item, ok := column.(map[string]any)
			if !ok {
				continue
			}
			if text := firstString(item, "text", "user_name"); text != "" {
				parts = append(parts, text)
			}
		}
	}
	return strings.TrimSpace(strings.Join(parts, " "))
}

func firstString(payload map[string]any, paths ...string) string {
	for _, path := range paths {
		current := any(payload)
		matched := true
		for _, part := range strings.Split(path, ".") {
			next, ok := current.(map[string]any)
			if !ok {
				matched = false
				break
			}
			current, ok = next[part]
			if !ok {
				matched = false
				break
			}
		}
		if matched {
			switch value := current.(type) {
			case string:
				return value
			}
		}
	}
	return ""
}