package agent

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/Neneka448/gogoclaw/internal/context"
	messagebus "github.com/Neneka448/gogoclaw/internal/message_bus"
	"github.com/Neneka448/gogoclaw/internal/provider"
	"github.com/Neneka448/gogoclaw/internal/session"
	toolspkg "github.com/Neneka448/gogoclaw/internal/tools"
	Openai "github.com/sashabaranov/go-openai"
)

type AgentLoop interface {
	ProcessMessage(message messagebus.Message) error
}

type agentLoop struct {
	context context.SystemContext
}

type outboundToolPayload struct {
	Content    string   `json:"content,omitempty"`
	MediaPaths []string `json:"media_paths,omitempty"`
	MediaPath  string   `json:"media_path,omitempty"`
}

const newSessionCommand = "/new"
const newSessionReply = "🎸A new session has started"

func NewAgentLoop(context context.SystemContext) AgentLoop {
	return &agentLoop{
		context: context,
	}
}

func (al *agentLoop) ProcessMessage(message messagebus.Message) error {
	al.prepareToolsForTurn(message)
	return al.loop(message)
}

func (al *agentLoop) buildTools() []Openai.Tool {
	tools := []Openai.Tool{}
	for _, tool := range al.context.ToolRegistry.GetAllTools() {
		tools = append(tools, tool.ToolForLLM)
	}
	return tools
}

func (al *agentLoop) loop(msg messagebus.Message) error {
	config, err := al.context.ConfigManager.GetAgentProfileConfig("default")
	if err != nil {
		return err
	}
	currentSession, err := al.getOrCreateSession(msg, config.Workspace)
	if err != nil {
		return err
	}
	if isNewSessionCommand(msg.Message) {
		if _, err := currentSession.ArchiveAndReset(); err != nil {
			return err
		}
		return al.publishDirectReply(msg, newSessionReply, "new_session")
	}
	if strings.TrimSpace(msg.Message) != "" {
		if err := currentSession.AppendMessage(Openai.ChatCompletionMessage{
			Role:    Openai.ChatMessageRoleUser,
			Content: msg.Message,
		}); err != nil {
			return err
		}
	}

	maxIterations := config.MaxToolIterations
	tools := al.buildTools()
	completed := false

	for i := 0; i < maxIterations; i++ {
		messages := al.buildMessage(currentSession, config.MemoryWindow)
		params := provider.BuildOpenaiRequestParams(provider.ChatCompletionParams{
			Model:               config.Model,
			Messages:            messages,
			MaxCompletionTokens: config.MaxTokens,
			Temperature:         config.Temperature,
			Tools:               tools,
		})
		response, err := al.context.Provider.ChatCompletion(params)
		if err != nil {
			return err
		}

		assistantMessage := provider.BuildAssistantMessage(response)
		if err := currentSession.AppendMessage(assistantMessage); err != nil {
			return err
		}
		if !(al.sentMessageInTurn() && !response.IsToolCall()) {
			if err := al.publishOutboundMessage(msg, assistantMessage, response.GetFinishReason()); err != nil {
				return err
			}
		}

		if !response.IsToolCall() {
			completed = true
			break
		}

		if err := al.publishToolCallMessages(msg, response.GetToolCalls()); err != nil {
			return err
		}

		toolResponses := al.executeToolCalls(response.GetToolCalls())
		if err := currentSession.AppendMessages(executedMessagesToChatMessages(toolResponses)); err != nil {
			return err
		}
		if err := al.publishOutboundMessages(msg, toolResponses); err != nil {
			return err
		}

	}

	if completed {
		return nil
	}

	maxIterationsMessage := al.buildMaxIterationsExceededMessage(maxIterations)
	if err := currentSession.AppendMessage(maxIterationsMessage); err != nil {
		return err
	}
	if err := al.publishOutboundMessage(msg, maxIterationsMessage, "max_iterations"); err != nil {
		return err
	}

	return nil
}

func isNewSessionCommand(message string) bool {
	return strings.TrimSpace(message) == newSessionCommand
}

func (al *agentLoop) buildMessage(currentSession session.Session, memoryWindow int) []Openai.ChatCompletionMessage {
	sessionMessages := currentSession.GetMessages(memoryWindow)
	systemPrompt := al.buildSystemPrompt()
	if strings.TrimSpace(systemPrompt) == "" {
		return sessionMessages
	}

	messages := make([]Openai.ChatCompletionMessage, 0, len(sessionMessages)+1)
	messages = append(messages, Openai.ChatCompletionMessage{
		Role:    Openai.ChatMessageRoleSystem,
		Content: systemPrompt,
	})
	messages = append(messages, sessionMessages...)
	return messages
}

func (al *agentLoop) buildSystemPrompt() string {
	if al.context.SystemPrompt == nil {
		return ""
	}
	return al.context.SystemPrompt.Build(al.context.Skills)
}

type executedToolMessage struct {
	Message                Openai.ChatCompletionMessage
	SuppressOutboundResult bool
}

func (al *agentLoop) executeToolCalls(toolCalls []provider.LLMToolCall) []executedToolMessage {
	messages := make([]executedToolMessage, 0, len(toolCalls))
	for _, toolCall := range toolCalls {
		toolDescriptor, err := al.context.ToolRegistry.GetTool(toolCall.Name)
		if err != nil {
			messages = append(messages, executedToolMessage{Message: buildToolCallOutputMessage(toolCall, buildToolExecutionErrorOutput(toolCall.Name, err))})
			continue
		}

		toolResponse, err := toolDescriptor.Tool.Execute(toolCall.Arguments)
		if err != nil {
			messages = append(messages, executedToolMessage{Message: buildToolCallOutputMessage(toolCall, buildToolExecutionErrorOutput(toolCall.Name, err))})
			continue
		}

		executed := executedToolMessage{Message: buildToolCallOutputMessage(toolCall, toolResponse)}
		if suppressor, ok := toolDescriptor.Tool.(toolspkg.OutboundSuppressionTool); ok {
			executed.SuppressOutboundResult = suppressor.SuppressToolResultOutbound()
		}
		messages = append(messages, executed)
	}

	return messages
}

func buildToolCallOutputMessage(toolCall provider.LLMToolCall, content string) Openai.ChatCompletionMessage {
	message := Openai.ChatCompletionMessage{Content: content}
	if toolCall.ID == "" {
		message.Role = Openai.ChatMessageRoleFunction
		message.Name = toolCall.Name
		return message
	}

	message.Role = Openai.ChatMessageRoleTool
	message.ToolCallID = toolCall.ID
	return message
}

func buildToolExecutionErrorOutput(toolName string, err error) string {
	payload := outboundToolPayload{
		Content: "Tool " + toolName + " failed: " + err.Error(),
	}
	raw, marshalErr := json.Marshal(map[string]string{
		"content": payload.Content,
		"error":   err.Error(),
	})
	if marshalErr != nil {
		return fmt.Sprintf("{\"content\":%q,\"error\":%q}", payload.Content, err.Error())
	}
	return string(raw)
}

func executedMessagesToChatMessages(executed []executedToolMessage) []Openai.ChatCompletionMessage {
	messages := make([]Openai.ChatCompletionMessage, 0, len(executed))
	for _, item := range executed {
		messages = append(messages, item.Message)
	}
	return messages
}

func (al *agentLoop) publishOutboundMessage(source messagebus.Message, message Openai.ChatCompletionMessage, finishReason string) error {
	if al.context.MessageBus == nil {
		return nil
	}
	if strings.TrimSpace(message.Content) == "" {
		return nil
	}

	return al.context.MessageBus.Put(messagebus.Message{
		ChannelID:    source.ChannelID,
		Message:      message.Content,
		MessageID:    source.MessageID,
		MessageType:  source.MessageType,
		ChatID:       source.ChatID,
		SenderID:     source.SenderID,
		MediaPaths:   cloneMediaPaths(source.MediaPaths),
		ReplyTo:      source.ReplyTo,
		Metadata:     cloneMetadata(source.Metadata),
		FinishReason: finishReason,
	}, messagebus.OutboundQueue)
}

func (al *agentLoop) publishDirectReply(source messagebus.Message, content string, finishReason string) error {
	if al.context.MessageBus == nil || strings.TrimSpace(content) == "" {
		return nil
	}
	return al.context.MessageBus.Put(messagebus.Message{
		ChannelID:    source.ChannelID,
		Message:      content,
		MessageID:    source.MessageID,
		MessageType:  source.MessageType,
		ChatID:       source.ChatID,
		SenderID:     source.SenderID,
		MediaPaths:   cloneMediaPaths(source.MediaPaths),
		ReplyTo:      source.ReplyTo,
		Metadata:     cloneMetadata(source.Metadata),
		FinishReason: finishReason,
	}, messagebus.OutboundQueue)
}

func (al *agentLoop) publishOutboundMessages(source messagebus.Message, messages []executedToolMessage) error {
	for _, executed := range messages {
		if executed.SuppressOutboundResult {
			continue
		}
		message := executed.Message
		outbound := cloneMetadata(source.Metadata)
		if outbound == nil {
			outbound = make(map[string]string, 1)
		}
		outbound["message_kind"] = "tool_result"
		content, mediaPaths := extractOutboundToolPayload(message.Content)
		if err := al.context.MessageBus.Put(messagebus.Message{
			ChannelID:    source.ChannelID,
			Message:      content,
			MessageID:    source.MessageID,
			MessageType:  source.MessageType,
			ChatID:       source.ChatID,
			SenderID:     source.SenderID,
			MediaPaths:   mergeMediaPaths(source.MediaPaths, mediaPaths),
			ReplyTo:      source.ReplyTo,
			Metadata:     outbound,
			FinishReason: "",
		}, messagebus.OutboundQueue); err != nil {
			return err
		}
	}

	return nil
}

func (al *agentLoop) prepareToolsForTurn(message messagebus.Message) {
	for _, descriptor := range al.context.ToolRegistry.GetAllTools() {
		if turnTool, ok := descriptor.Tool.(toolspkg.TurnLifecycleTool); ok {
			turnTool.StartTurn()
		}
		if contextTool, ok := descriptor.Tool.(toolspkg.MessageContextTool); ok {
			contextTool.SetMessageContext(message)
		}
	}
}

func (al *agentLoop) sentMessageInTurn() bool {
	for _, descriptor := range al.context.ToolRegistry.GetAllTools() {
		if sender, ok := descriptor.Tool.(toolspkg.SentMessageTool); ok && sender.SentInTurn() {
			return true
		}
	}
	return false
}

func (al *agentLoop) publishToolCallMessages(source messagebus.Message, toolCalls []provider.LLMToolCall) error {
	if al.context.MessageBus == nil {
		return nil
	}

	for _, toolCall := range toolCalls {
		if err := al.context.MessageBus.Put(messagebus.Message{
			ChannelID:    source.ChannelID,
			Message:      formatToolCallMessage(toolCall),
			MessageID:    source.MessageID,
			MessageType:  source.MessageType,
			ChatID:       source.ChatID,
			SenderID:     source.SenderID,
			MediaPaths:   cloneMediaPaths(source.MediaPaths),
			ReplyTo:      source.ReplyTo,
			Metadata:     cloneMetadata(source.Metadata),
			FinishReason: "tool_calls",
		}, messagebus.OutboundQueue); err != nil {
			return err
		}
	}

	return nil
}

func cloneMediaPaths(paths []string) []string {
	if len(paths) == 0 {
		return nil
	}
	cloned := make([]string, len(paths))
	copy(cloned, paths)
	return cloned
}

func mergeMediaPaths(base []string, extra []string) []string {
	if len(base) == 0 && len(extra) == 0 {
		return nil
	}
	merged := make([]string, 0, len(base)+len(extra))
	merged = append(merged, base...)
	merged = append(merged, extra...)
	return merged
}

func cloneMetadata(metadata map[string]string) map[string]string {
	if len(metadata) == 0 {
		return nil
	}
	cloned := make(map[string]string, len(metadata))
	for key, value := range metadata {
		cloned[key] = value
	}
	return cloned
}

func formatToolCallMessage(toolCall provider.LLMToolCall) string {
	return toolCall.Name + "(" + toolCall.Arguments + ")"
}

func extractOutboundToolPayload(raw string) (string, []string) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", nil
	}

	var payload outboundToolPayload
	if err := json.Unmarshal([]byte(trimmed), &payload); err != nil {
		return raw, nil
	}

	if strings.TrimSpace(payload.MediaPath) != "" {
		payload.MediaPaths = append(payload.MediaPaths, payload.MediaPath)
	}
	if strings.TrimSpace(payload.Content) == "" && len(payload.MediaPaths) == 0 {
		return raw, nil
	}

	return payload.Content, cloneMediaPaths(payload.MediaPaths)
}

func (al *agentLoop) buildMaxIterationsExceededMessage(maxIterations int) Openai.ChatCompletionMessage {
	return Openai.ChatCompletionMessage{
		Role:    Openai.ChatMessageRoleAssistant,
		Content: "I reached the maximum number of tool call iterations (" + strconv.Itoa(maxIterations) + ") without finishing. If you want me to continue, please reply \"continue\".",
	}
}

func (al *agentLoop) getOrCreateSession(msg messagebus.Message, workspace string) (session.Session, error) {
	if al.context.SessionManager == nil {
		al.context.SessionManager = session.NewSessionManager(workspace)
	}

	return al.context.SessionManager.GetOrCreateSession(session.MakeSessionID(msg.ChannelID, msg.ChatID), msg.SenderID)
}
