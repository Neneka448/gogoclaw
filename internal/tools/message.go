package tools

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	messagebus "github.com/Neneka448/gogoclaw/internal/message_bus"
	"github.com/Neneka448/gogoclaw/internal/utils"
	openai "github.com/sashabaranov/go-openai"
)

type MessageTool struct {
	sink messagebus.OutputSink

	mu         sync.Mutex
	context    messagebus.Message
	sentInTurn bool
}

type messageToolArgs struct {
	Content       string   `json:"content,omitempty"`
	MediaPaths    []string `json:"media_paths,omitempty"`
	MediaPath     string   `json:"media_path,omitempty"`
	TargetProfile string   `json:"target_profile,omitempty"`
}

type messageToolResult struct {
	Status string `json:"status,omitempty"`
	Error  string `json:"error,omitempty"`
}

func NewMessageTool(sink messagebus.OutputSink) ToolDescriptor {
	return ToolDescriptor{
		Name: "message",
		Tool: &MessageTool{sink: sink},
		ToolForLLM: openai.Tool{
			Type: openai.ToolTypeFunction,
			Function: &openai.FunctionDefinition{
				Name:        "message",
				Description: "Send a message to the user. Use this when you want to communicate something.",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"content": map[string]any{
							"type":        "string",
							"description": "The message content to send.",
						},
						"media_paths": map[string]any{
							"type":        "array",
							"items":       map[string]any{"type": "string"},
							"description": "Optional list of local file paths to attach, such as images, audio, or documents.",
						},
						"media_path": map[string]any{
							"type":        "string",
							"description": "Optional single local file path to attach to the message.",
						},
						"target_profile": map[string]any{
							"type":        "string",
							"description": "Optional target agent profile name. When set, the message is routed via MQ to the specified remote profile instead of replying to the current channel.",
						},
					},
					"required": []string{"content"},
				},
			},
		},
	}
}

func (tool *MessageTool) SetMessageContext(message messagebus.Message) {
	tool.mu.Lock()
	defer tool.mu.Unlock()
	tool.context = message
}

func (tool *MessageTool) StartTurn() {
	tool.mu.Lock()
	defer tool.mu.Unlock()
	tool.sentInTurn = false
}

func (tool *MessageTool) SentInTurn() bool {
	tool.mu.Lock()
	defer tool.mu.Unlock()
	return tool.sentInTurn
}

func (tool *MessageTool) SuppressToolResultOutbound() bool {
	return true
}

func (tool *MessageTool) Execute(args string) (string, error) {
	var input messageToolArgs
	if err := json.Unmarshal([]byte(args), &input); err != nil {
		return encodeMessageToolResult(messageToolResult{Error: fmt.Sprintf("parse message args: %v", err)})
	}

	input.Content = strings.TrimSpace(input.Content)
	mediaPaths := normalizeMessageMediaPaths(input.MediaPaths, input.MediaPath)
	if input.Content == "" && len(mediaPaths) == 0 {
		return encodeMessageToolResult(messageToolResult{Error: "message requires content or media_paths"})
	}
	if tool.sink == nil {
		return encodeMessageToolResult(messageToolResult{Error: "output sink is not initialized"})
	}

	tool.mu.Lock()
	ctx := tool.context
	tool.mu.Unlock()

	targetProfile := strings.TrimSpace(input.TargetProfile)
	if targetProfile != "" {
		return tool.executeMQDirect(ctx, input.Content, mediaPaths, targetProfile)
	}

	if strings.TrimSpace(ctx.ChannelID) == "" || strings.TrimSpace(ctx.ChatID) == "" {
		return encodeMessageToolResult(messageToolResult{Error: "message context is not set"})
	}

	outbound := messagebus.Message{
		ChannelID:    ctx.ChannelID,
		Message:      input.Content,
		MessageID:    ctx.MessageID,
		MessageType:  ctx.MessageType,
		ChatID:       ctx.ChatID,
		SenderID:     ctx.SenderID,
		MediaPaths:   mediaPaths,
		ReplyTo:      ctx.ReplyTo,
		Metadata:     cloneMessageMetadata(ctx.Metadata),
		FinishReason: "",
	}
	if outbound.Metadata == nil {
		outbound.Metadata = make(map[string]string, 1)
	}
	outbound.Metadata["message_kind"] = "active_message"

	if err := tool.sink.Emit(outbound); err != nil {
		return encodeMessageToolResult(messageToolResult{Error: err.Error()})
	}

	tool.mu.Lock()
	tool.sentInTurn = true
	tool.mu.Unlock()

	return encodeMessageToolResult(messageToolResult{Status: "sent"})
}

func encodeMessageToolResult(result messageToolResult) (string, error) {
	return utils.EncodeJSON(result)
}

func normalizeMessageMediaPaths(paths []string, single string) []string {
	normalized := make([]string, 0, len(paths)+1)
	for _, path := range paths {
		trimmed := strings.TrimSpace(path)
		if trimmed == "" {
			continue
		}
		normalized = append(normalized, trimmed)
	}
	if trimmed := strings.TrimSpace(single); trimmed != "" {
		normalized = append(normalized, trimmed)
	}
	if len(normalized) == 0 {
		return nil
	}
	return normalized
}

func cloneMessageMetadata(metadata map[string]string) map[string]string {
	if len(metadata) == 0 {
		return nil
	}
	cloned := make(map[string]string, len(metadata))
	for key, value := range metadata {
		cloned[key] = value
	}
	return cloned
}

const mqChannelID = "mq"

func (tool *MessageTool) executeMQDirect(ctx messagebus.Message, content string, mediaPaths []string, targetProfile string) (string, error) {
	metadata := cloneMessageMetadata(ctx.Metadata)
	if metadata == nil {
		metadata = make(map[string]string, 8)
	}
	metadata["target_profile"] = targetProfile
	metadata["message_kind"] = "active_message"

	// Propagate return routing so the remote agent's reply
	// routes back to the original caller (e.g. the MQ source or feishu chat).
	if ctx.ChannelID != "" {
		metadata["return_channel_id"] = ctx.ChannelID
	}
	if ctx.ChatID != "" {
		metadata["return_chat_id"] = ctx.ChatID
	}
	if ctx.SenderID != "" {
		metadata["return_sender_id"] = ctx.SenderID
	}
	if ctx.ReplyTo != "" {
		metadata["return_reply_to"] = ctx.ReplyTo
	}
	if ctx.MessageID != "" {
		metadata["return_message_id"] = ctx.MessageID
	}
	if ctx.MessageType != "" {
		metadata["return_message_type"] = ctx.MessageType
	}

	outbound := messagebus.Message{
		ChannelID:    mqChannelID,
		Message:      content,
		MessageID:    ctx.MessageID,
		MessageType:  "direct",
		ChatID:       ctx.ChatID,
		SenderID:     ctx.SenderID,
		MediaPaths:   mediaPaths,
		Metadata:     metadata,
		FinishReason: "",
	}
	if err := tool.sink.Emit(outbound); err != nil {
		return encodeMessageToolResult(messageToolResult{Error: err.Error()})
	}
	tool.mu.Lock()
	tool.sentInTurn = true
	tool.mu.Unlock()
	return encodeMessageToolResult(messageToolResult{Status: "sent_to_profile:" + targetProfile})
}
