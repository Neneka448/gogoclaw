package channels

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

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
	toolMu     sync.Mutex
	toolCalls  map[string]*pendingToolCallBatch
	messageIDs []string
	seen       map[string]struct{}
}

type pendingToolCallBatch struct {
	messages []messagebus.Message
	timer    *time.Timer
}

var (
	feishuComplexMarkdownRe = regexp.MustCompile("(?m)```|^\\s*#{1,6}\\s|\\|.+\\||^\\s*>|^\\s*---+$")
	feishuSimpleMarkdownRe  = regexp.MustCompile(`\*\*[^*]+\*\*|__[^_]+__|~~[^~]+~~|` + "`[^`]+`" + `|\*[^*]+\*|_[^_]+_`)
	feishuListRe            = regexp.MustCompile(`(?m)^\s*[-*+]\s+`)
	feishuOrderedListRe     = regexp.MustCompile(`(?m)^\s*\d+\.\s+`)
	feishuMarkdownLinkRe    = regexp.MustCompile(`\[([^\]]+)\]\(([^)]+)\)`)
	feishuWhitespaceRe      = regexp.MustCompile(`\s+`)
	feishuTextMaxLen        = 200
	feishuPostMaxLen        = 2000
	feishuToolPreviewMaxLen = 120
	feishuToolDetailMaxLen  = 12000
	feishuToolCallDebounce  = 2 * time.Second
	feishuImageExts         = map[string]struct{}{
		".png": {}, ".jpg": {}, ".jpeg": {}, ".gif": {}, ".bmp": {}, ".webp": {}, ".ico": {}, ".tiff": {}, ".tif": {},
	}
	feishuAudioExts = map[string]struct{}{
		".opus": {},
	}
	feishuVideoExts = map[string]struct{}{
		".mp4": {}, ".mov": {}, ".avi": {},
	}
	feishuFileTypeMap = map[string]string{
		".opus": "opus",
		".mp4":  "mp4",
		".pdf":  "pdf",
		".doc":  "doc",
		".docx": "doc",
		".xls":  "xls",
		".xlsx": "xls",
		".ppt":  "ppt",
		".pptx": "ppt",
	}
)

func NewFeishuChannel(cfg config.FeishuChannelConfig, bus messagebus.MessageBus) *FeishuChannel {
	return &FeishuChannel{
		config:     cfg,
		messageBus: bus,
		toolCalls:  make(map[string]*pendingToolCallBatch),
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
	if err := c.flushAllToolCallBatches(); err != nil {
		return err
	}
	return nil
}

func (c *FeishuChannel) Send(message messagebus.Message) error {
	if !c.Enabled() {
		return fmt.Errorf("feishu channel disabled")
	}
	if message.FinishReason == "tool_calls" {
		return c.enqueueToolCall(message)
	}
	if err := c.flushToolCallBatch(toolCallBatchKey(message)); err != nil {
		return err
	}
	mediaPaths := outboundMediaPaths(message)
	if strings.TrimSpace(message.Message) == "" && len(mediaPaths) == 0 {
		return nil
	}
	if c.client == nil {
		c.client = lark.NewClient(c.config.AppID, c.config.AppSecret)
	}

	receiveIDType := receiveIDTypeForChatID(message.ChatID)
	for _, mediaPath := range mediaPaths {
		if err := c.sendMediaMessage(receiveIDType, message.ChatID, mediaPath); err != nil {
			return err
		}
	}
	if strings.TrimSpace(message.Message) == "" {
		return nil
	}
	if isToolResultMessage(message) {
		content, err := buildFeishuToolResultCard(message.Message)
		if err != nil {
			return err
		}
		return c.sendMessage(receiveIDType, message.ChatID, larkim.MsgTypeInteractive, content)
	}

	messageType, content, err := buildFeishuOutboundContent(message.Message)
	if err != nil {
		return err
	}
	return c.sendMessage(receiveIDType, message.ChatID, messageType, content)
}

func (c *FeishuChannel) enqueueToolCall(message messagebus.Message) error {
	key := toolCallBatchKey(message)
	c.toolMu.Lock()
	batch, ok := c.toolCalls[key]
	if !ok {
		batch = &pendingToolCallBatch{}
		c.toolCalls[key] = batch
	}
	batch.messages = append(batch.messages, message)
	if batch.timer != nil {
		batch.timer.Stop()
	}
	batch.timer = time.AfterFunc(feishuToolCallDebounce, func() {
		_ = c.flushToolCallBatch(key)
	})
	c.toolMu.Unlock()
	return nil
}

func (c *FeishuChannel) flushToolCallBatch(key string) error {
	batch := c.takeToolCallBatch(key)
	if batch == nil || len(batch.messages) == 0 {
		return nil
	}
	messageType, content, err := buildFeishuOutboundContent(formatToolCallBatch(batch.messages))
	if err != nil {
		return err
	}
	last := batch.messages[len(batch.messages)-1]
	return c.sendMessage(receiveIDTypeForChatID(last.ChatID), last.ChatID, messageType, content)
}

func (c *FeishuChannel) flushAllToolCallBatches() error {
	c.toolMu.Lock()
	keys := make([]string, 0, len(c.toolCalls))
	for key := range c.toolCalls {
		keys = append(keys, key)
	}
	c.toolMu.Unlock()
	for _, key := range keys {
		if err := c.flushToolCallBatch(key); err != nil {
			return err
		}
	}
	return nil
}

func (c *FeishuChannel) takeToolCallBatch(key string) *pendingToolCallBatch {
	c.toolMu.Lock()
	defer c.toolMu.Unlock()
	batch, ok := c.toolCalls[key]
	if !ok {
		return nil
	}
	delete(c.toolCalls, key)
	if batch.timer != nil {
		batch.timer.Stop()
	}
	return batch
}

func toolCallBatchKey(message messagebus.Message) string {
	if strings.TrimSpace(message.ReplyTo) != "" {
		return message.ChatID + "|" + message.ReplyTo
	}
	return message.ChatID
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

	message, ok := parseFeishuInboundMessage(body)
	if !ok {
		return nil
	}
	if message.MessageID != "" && !c.markMessage(message.MessageID) {
		return nil
	}
	if isBotSender(body) {
		return nil
	}
	if !c.isAllowed(message.SenderID) {
		return nil
	}
	if err := c.addReaction(message.MessageID); err != nil {
		return err
	}
	if c.messageBus == nil {
		return nil
	}

	return c.messageBus.Put(message, messagebus.InboundQueue)
}

func parseFeishuInboundMessage(body map[string]any) (messagebus.Message, bool) {
	messageID := firstString(body,
		"event.message.message_id",
		"event.message.messageId",
		"message.message_id",
		"message.messageId",
	)
	senderID := firstString(body,
		"event.sender.sender_id.open_id",
		"event.sender.sender_id.user_id",
		"sender.sender_id.open_id",
		"sender.sender_id.user_id",
	)
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
		return messagebus.Message{}, false
	}

	metadata := collectFeishuMetadata(body)
	return messagebus.Message{
		ChannelID:   "feishu",
		Message:     content,
		MessageID:   messageID,
		MessageType: messageType,
		ChatID:      chatID,
		SenderID:    senderID,
		ReplyTo:     firstNonEmpty(metadata["parent_id"], metadata["root_id"], metadata["thread_id"]),
		Metadata:    metadata,
	}, true
}

func collectFeishuMetadata(body map[string]any) map[string]string {
	metadata := map[string]string{}
	for key, value := range map[string]string{
		"message_id": firstString(body,
			"event.message.message_id",
			"event.message.messageId",
			"message.message_id",
			"message.messageId",
		),
		"chat_type": firstString(body,
			"event.message.chat_type",
			"event.message.chatType",
			"message.chat_type",
			"message.chatType",
		),
		"parent_id": firstString(body,
			"event.message.parent_id",
			"event.message.parentId",
			"message.parent_id",
			"message.parentId",
		),
		"root_id": firstString(body,
			"event.message.root_id",
			"event.message.rootId",
			"message.root_id",
			"message.rootId",
		),
		"thread_id": firstString(body,
			"event.message.thread_id",
			"event.message.threadId",
			"message.thread_id",
			"message.threadId",
		),
		"sender_type": firstString(body,
			"event.sender.sender_type",
			"event.sender.senderType",
			"sender.sender_type",
			"sender.senderType",
		),
	} {
		if value != "" {
			metadata[key] = value
		}
	}
	return metadata
}

func isBotSender(body map[string]any) bool {
	return strings.EqualFold(firstString(body,
		"event.sender.sender_type",
		"event.sender.senderType",
		"sender.sender_type",
		"sender.senderType",
	), "bot")
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
		return firstNonEmpty(
			firstString(content, "text_without_at_bot"),
			firstString(content, "textWithoutAtBot"),
			firstString(content, "text"),
		)
	case "post":
		return flattenFeishuPost(content)
	case "interactive":
		if flattened := flattenFeishuInteractive(content); flattened != "" {
			return flattened
		}
		encoded, _ := json.Marshal(content)
		return string(encoded)
	default:
		return firstNonEmpty(
			firstString(content, "text_without_at_bot"),
			firstString(content, "textWithoutAtBot"),
			firstString(content, "text"),
		)
	}
}

func flattenFeishuInteractive(content map[string]any) string {
	parts := make([]string, 0, 8)
	for _, candidate := range []string{
		firstString(content, "title.content"),
		firstString(content, "header.title.content"),
		firstString(content, "header.title.text"),
	} {
		if candidate != "" {
			parts = append(parts, candidate)
		}
	}

	for _, key := range []string{"elements", "body.elements", "card.elements"} {
		for _, element := range getAnySliceByPath(content, key) {
			parts = append(parts, flattenInteractiveElement(element)...)
		}
	}

	return strings.TrimSpace(strings.Join(uniqueNonEmpty(parts), " "))
}

func flattenInteractiveElement(value any) []string {
	item, ok := value.(map[string]any)
	if !ok {
		return nil
	}
	parts := make([]string, 0, 4)
	if text := firstString(item, "content", "text.content", "text.text", "title.content", "title.text"); text != "" {
		parts = append(parts, text)
	}
	for _, key := range []string{"elements", "fields", "columns", "actions"} {
		for _, nested := range getAnySliceByPath(item, key) {
			parts = append(parts, flattenInteractiveElement(nested)...)
		}
	}
	return parts
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

func getAnySliceByPath(payload map[string]any, path string) []any {
	current := any(payload)
	for _, part := range strings.Split(path, ".") {
		next, ok := current.(map[string]any)
		if !ok {
			return nil
		}
		current, ok = next[part]
		if !ok {
			return nil
		}
	}
	values, ok := current.([]any)
	if !ok {
		return nil
	}
	return values
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func uniqueNonEmpty(values []string) []string {
	result := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		result = append(result, trimmed)
	}
	sort.Strings(result)
	return result
}

func buildFeishuOutboundContent(content string) (string, string, error) {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return "", "", fmt.Errorf("feishu outbound content is empty")
	}

	switch detectFeishuMessageFormat(trimmed) {
	case larkim.MsgTypePost:
		post, err := markdownToFeishuPost(trimmed)
		if err != nil {
			return "", "", err
		}
		return larkim.MsgTypePost, post, nil
	case larkim.MsgTypeInteractive:
		interactive, err := markdownToFeishuInteractive(trimmed)
		if err != nil {
			return "", "", err
		}
		return larkim.MsgTypeInteractive, interactive, nil
	default:
		textPayload, err := json.Marshal(map[string]string{"text": trimmed})
		if err != nil {
			return "", "", err
		}
		return larkim.MsgTypeText, string(textPayload), nil
	}
}

func isToolResultMessage(message messagebus.Message) bool {
	if message.Metadata == nil {
		return false
	}
	return message.Metadata["message_kind"] == "tool_result"
}

func buildFeishuToolResultCard(content string) (string, error) {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return "", fmt.Errorf("feishu tool result content is empty")
	}

	detail := trimmed
	if len(detail) > feishuToolDetailMaxLen {
		detail = detail[:feishuToolDetailMaxLen] + "\n\n... output truncated ..."
	}

	preview := singleLinePreview(trimmed, feishuToolPreviewMaxLen)
	card := map[string]any{
		"config": map[string]any{
			"wide_screen_mode": true,
		},
		"header": map[string]any{
			"title": map[string]any{
				"tag":     "plain_text",
				"content": "Tool Output",
			},
			"template": "blue",
		},
		"elements": []any{
			map[string]any{
				"tag":     "markdown",
				"content": preview,
			},
			map[string]any{
				"tag":      "collapsible_panel",
				"expanded": false,
				"header": map[string]any{
					"title": map[string]any{
						"tag":     "plain_text",
						"content": "Click to view the full output",
					},
				},
				"elements": []any{
					map[string]any{
						"tag":     "markdown",
						"content": "```json\n" + detail + "\n```",
					},
				},
			},
		},
	}

	encoded, err := json.Marshal(card)
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}

func singleLinePreview(content string, maxLen int) string {
	collapsed := strings.TrimSpace(feishuWhitespaceRe.ReplaceAllString(content, " "))
	if maxLen <= 0 || len(collapsed) <= maxLen {
		return collapsed
	}
	if maxLen <= 3 {
		return collapsed[:maxLen]
	}
	return collapsed[:maxLen-3] + "..."
}

func formatToolCallBatch(messages []messagebus.Message) string {
	lines := []string{"Tool calls in progress"}
	for index, message := range messages {
		name, arguments := splitToolCallMessage(message.Message)
		if name == "" {
			name = message.Message
		}
		lines = append(lines, strconv.Itoa(index+1)+". "+name)
		if strings.TrimSpace(arguments) != "" {
			lines = append(lines, "```json")
			lines = append(lines, prettyToolCallArguments(arguments))
			lines = append(lines, "```")
		}
	}
	return strings.Join(lines, "\n")
}

func splitToolCallMessage(content string) (string, string) {
	trimmed := strings.TrimSpace(content)
	open := strings.Index(trimmed, "(")
	close := strings.LastIndex(trimmed, ")")
	if open <= 0 || close <= open {
		return trimmed, ""
	}
	return strings.TrimSpace(trimmed[:open]), strings.TrimSpace(trimmed[open+1 : close])
}

func prettyToolCallArguments(arguments string) string {
	trimmed := strings.TrimSpace(arguments)
	if trimmed == "" {
		return "{}"
	}
	var payload any
	if err := json.Unmarshal([]byte(trimmed), &payload); err != nil {
		return trimmed
	}
	formatted, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return trimmed
	}
	return string(formatted)
}

func detectFeishuMessageFormat(content string) string {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return larkim.MsgTypeText
	}
	if feishuComplexMarkdownRe.MatchString(trimmed) {
		return larkim.MsgTypeInteractive
	}
	if len(trimmed) > feishuPostMaxLen {
		return larkim.MsgTypeInteractive
	}
	if feishuSimpleMarkdownRe.MatchString(trimmed) {
		return larkim.MsgTypeInteractive
	}
	if feishuListRe.MatchString(trimmed) || feishuOrderedListRe.MatchString(trimmed) {
		return larkim.MsgTypeInteractive
	}
	if feishuMarkdownLinkRe.MatchString(trimmed) {
		return larkim.MsgTypePost
	}
	if len(trimmed) <= feishuTextMaxLen {
		return larkim.MsgTypeText
	}
	return larkim.MsgTypePost
}

func markdownToFeishuPost(content string) (string, error) {
	lines := strings.Split(strings.TrimSpace(content), "\n")
	paragraphs := make([][]map[string]string, 0, len(lines))
	for _, line := range lines {
		elements := make([]map[string]string, 0, 4)
		lastEnd := 0
		for _, match := range feishuMarkdownLinkRe.FindAllStringSubmatchIndex(line, -1) {
			before := line[lastEnd:match[0]]
			if before != "" {
				elements = append(elements, map[string]string{"tag": "text", "text": before})
			}
			elements = append(elements, map[string]string{
				"tag":  "a",
				"text": line[match[2]:match[3]],
				"href": line[match[4]:match[5]],
			})
			lastEnd = match[1]
		}
		remaining := line[lastEnd:]
		if remaining != "" {
			elements = append(elements, map[string]string{"tag": "text", "text": remaining})
		}
		if len(elements) == 0 {
			elements = append(elements, map[string]string{"tag": "text", "text": ""})
		}
		paragraphs = append(paragraphs, elements)
	}
	body := map[string]any{
		"zh_cn": map[string]any{
			"content": paragraphs,
		},
	}
	encoded, err := json.Marshal(body)
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}

func markdownToFeishuInteractive(content string) (string, error) {
	body := map[string]any{
		"config": map[string]any{
			"wide_screen_mode": true,
		},
		"elements": []map[string]string{{
			"tag":     "markdown",
			"content": content,
		}},
	}
	encoded, err := json.Marshal(body)
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}

func outboundMediaPaths(message messagebus.Message) []string {
	paths := cloneStringSlice(message.MediaPaths)
	if len(paths) > 0 {
		return paths
	}
	if message.Metadata != nil {
		if raw := strings.TrimSpace(message.Metadata["media_paths"]); raw != "" {
			var decoded []string
			if err := json.Unmarshal([]byte(raw), &decoded); err == nil {
				return decoded
			}
		}
		if raw := strings.TrimSpace(message.Metadata["media_path"]); raw != "" {
			return []string{raw}
		}
	}
	if isLocalFilePath(message.Message) && strings.TrimSpace(message.MessageType) != "text" {
		return []string{message.Message}
	}
	return nil
}

func cloneStringSlice(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	cloned := make([]string, len(values))
	copy(cloned, values)
	return cloned
}

func isLocalFilePath(path string) bool {
	if strings.TrimSpace(path) == "" {
		return false
	}
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func (c *FeishuChannel) sendMediaMessage(receiveIDType string, receiveID string, mediaPath string) error {
	if !isLocalFilePath(mediaPath) {
		return fmt.Errorf("feishu media path not found: %s", mediaPath)
	}
	ext := strings.ToLower(filepath.Ext(mediaPath))
	if _, ok := feishuImageExts[ext]; ok {
		imageKey, err := c.uploadImage(mediaPath)
		if err != nil {
			return err
		}
		payload, err := json.Marshal(map[string]string{"image_key": imageKey})
		if err != nil {
			return err
		}
		return c.sendMessage(receiveIDType, receiveID, larkim.MsgTypeImage, string(payload))
	}

	fileKey, messageType, err := c.uploadFile(mediaPath)
	if err != nil {
		return err
	}
	payload, err := json.Marshal(map[string]string{"file_key": fileKey})
	if err != nil {
		return err
	}
	return c.sendMessage(receiveIDType, receiveID, messageType, string(payload))
}

func (c *FeishuChannel) uploadImage(mediaPath string) (string, error) {
	body, err := larkim.NewCreateImagePathReqBodyBuilder().
		ImagePath(mediaPath).
		ImageType("message").
		Build()
	if err != nil {
		return "", err
	}
	resp, err := c.client.Im.Image.Create(context.Background(), larkim.NewCreateImageReqBuilder().Body(body).Build())
	if err != nil {
		return "", err
	}
	if !resp.Success() || resp.Data == nil || resp.Data.ImageKey == nil {
		return "", fmt.Errorf("feishu upload image failed: code=%d msg=%s request_id=%s", resp.Code, resp.Msg, resp.RequestId())
	}
	return *resp.Data.ImageKey, nil
}

func (c *FeishuChannel) uploadFile(mediaPath string) (string, string, error) {
	ext := strings.ToLower(filepath.Ext(mediaPath))
	body, err := larkim.NewCreateFilePathReqBodyBuilder().
		FilePath(mediaPath).
		FileName(filepath.Base(mediaPath)).
		FileType(feishuUploadFileType(ext)).
		Build()
	if err != nil {
		return "", "", err
	}
	resp, err := c.client.Im.File.Create(context.Background(), larkim.NewCreateFileReqBuilder().Body(body).Build())
	if err != nil {
		return "", "", err
	}
	if !resp.Success() || resp.Data == nil || resp.Data.FileKey == nil {
		return "", "", fmt.Errorf("feishu upload file failed: code=%d msg=%s request_id=%s", resp.Code, resp.Msg, resp.RequestId())
	}
	return *resp.Data.FileKey, feishuMessageTypeForFileExt(ext), nil
}

func feishuUploadFileType(ext string) string {
	if fileType, ok := feishuFileTypeMap[ext]; ok {
		return fileType
	}
	return "stream"
}

func feishuMessageTypeForFileExt(ext string) string {
	if _, ok := feishuAudioExts[ext]; ok {
		return "media"
	}
	if _, ok := feishuVideoExts[ext]; ok {
		return "media"
	}
	return "file"
}

func (c *FeishuChannel) sendMessage(receiveIDType string, receiveID string, messageType string, content string) error {
	resp, err := c.client.Im.Message.Create(context.Background(), larkim.NewCreateMessageReqBuilder().
		ReceiveIdType(receiveIDType).
		Body(larkim.NewCreateMessageReqBodyBuilder().
			ReceiveId(receiveID).
			MsgType(messageType).
			Content(content).
			Build()).
		Build())
	if err != nil {
		return err
	}
	if !resp.Success() {
		return fmt.Errorf("feishu send %s message failed: code=%d msg=%s request_id=%s", messageType, resp.Code, resp.Msg, resp.RequestId())
	}
	return nil
}

func (c *FeishuChannel) addReaction(messageID string) error {
	if strings.TrimSpace(messageID) == "" || c.client == nil {
		return nil
	}
	reactionEmoji := strings.TrimSpace(c.config.ReactEmoji)
	if reactionEmoji == "" {
		reactionEmoji = "THUMBSUP"
	}
	resp, err := c.client.Im.V1.MessageReaction.Create(context.Background(), larkim.NewCreateMessageReactionReqBuilder().
		MessageId(messageID).
		Body(larkim.NewCreateMessageReactionReqBodyBuilder().
			ReactionType(larkim.NewEmojiBuilder().EmojiType(reactionEmoji).Build()).
			Build()).
		Build())
	if err != nil {
		return err
	}
	if !resp.Success() {
		return fmt.Errorf("feishu add reaction failed: code=%d msg=%s request_id=%s", resp.Code, resp.Msg, resp.RequestId())
	}
	return nil
}
