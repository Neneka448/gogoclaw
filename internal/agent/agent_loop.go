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
	Openai "github.com/sashabaranov/go-openai"
)

type AgentLoop interface {
	ProcessMessage(message messagebus.Message) error
}

type agentLoop struct {
	context context.SystemContext
}

const newSessionCommand = "/new"
const newSessionReply = "🎸新会话已启动"

func NewAgentLoop(context context.SystemContext) AgentLoop {
	return &agentLoop{
		context: context,
	}
}

func (al *agentLoop) ProcessMessage(message messagebus.Message) error {
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
		if err := al.publishOutboundMessage(msg, assistantMessage, response.GetFinishReason()); err != nil {
			return err
		}

		if !response.IsToolCall() {
			completed = true
			break
		}

		if err := al.publishToolCallMessages(msg, response.GetToolCalls()); err != nil {
			return err
		}

		toolResponses, err := al.executeToolCalls(response.GetToolCalls())
		if err != nil {
			return err
		}
		if err := currentSession.AppendMessages(toolResponses); err != nil {
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
	if al.context.Skills == nil || al.context.Skills.Len() == 0 {
		return ""
	}

	var builder strings.Builder
	builder.WriteString("You have access to reusable workspace skills. Each skill lives under workspace/skills/<skill-name>/SKILL.md.\n")
	builder.WriteString("Before solving the current request, compare the request against the available skill metadata below and judge whether the scenario matches a skill.\n")
	builder.WriteString("If a skill appears relevant, call get_skill with the skill name before continuing so you can read the full SKILL.md instructions.\n")
	builder.WriteString("If no skill matches, continue normally without calling get_skill.\n")
	builder.WriteString("Available skills:\n")

	for _, skill := range al.context.Skills.GetAll() {
		metadata := "{}"
		if len(skill.FrontMatter) > 0 {
			encoded, err := json.Marshal(skill.FrontMatter)
			if err == nil {
				metadata = string(encoded)
			}
		}
		builder.WriteString(fmt.Sprintf("- %s: %s\n", skill.Name, metadata))
	}

	return strings.TrimSpace(builder.String())
}

func (al *agentLoop) executeToolCalls(toolCalls []provider.LLMToolCall) ([]Openai.ChatCompletionMessage, error) {
	messages := make([]Openai.ChatCompletionMessage, 0, len(toolCalls))
	for _, toolCall := range toolCalls {
		toolDescriptor, err := al.context.ToolRegistry.GetTool(toolCall.Name)
		if err != nil {
			return nil, err
		}

		toolResponse, err := toolDescriptor.Tool.Execute(toolCall.Arguments)
		if err != nil {
			return nil, err
		}

		message := Openai.ChatCompletionMessage{Content: toolResponse}
		if toolCall.ID == "" {
			message.Role = Openai.ChatMessageRoleFunction
			message.Name = toolCall.Name
		} else {
			message.Role = Openai.ChatMessageRoleTool
			message.ToolCallID = toolCall.ID
		}

		messages = append(messages, message)
	}

	return messages, nil
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

func (al *agentLoop) publishOutboundMessages(source messagebus.Message, messages []Openai.ChatCompletionMessage) error {
	for _, message := range messages {
		if err := al.publishOutboundMessage(source, message, ""); err != nil {
			return err
		}
	}

	return nil
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
