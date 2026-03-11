package provider

import (
	"maps"

	openai "github.com/sashabaranov/go-openai"
)

type ChatCompletionParams struct {
	Model               string
	Messages            []openai.ChatCompletionMessage
	MaxCompletionTokens int
	Temperature         float64
	ResponseFormat      *openai.ChatCompletionResponseFormat
	Tools               []openai.Tool
	ToolChoice          *openai.Tool
	ReasoningEffort     string
	Metadata            map[string]string
}

type LLMToolCall struct {
	ID        string
	Name      string
	Arguments string
	Type      string
}

type LLMCommonResponse interface {
	GetContent() string
	GetFinishReason() string
	IsToolCall() bool
	GetToolCalls() []LLMToolCall
}

type NormalizedResponse struct {
	Content      string
	FinishReason string
	ToolCalls    []LLMToolCall
}

func (response NormalizedResponse) GetContent() string {
	return response.Content
}

func (response NormalizedResponse) GetFinishReason() string {
	if response.FinishReason != "" {
		return response.FinishReason
	}
	if len(response.ToolCalls) > 0 {
		return "tool_calls"
	}
	return "stop"
}

func (response NormalizedResponse) IsToolCall() bool {
	return len(response.ToolCalls) > 0
}

func (response NormalizedResponse) GetToolCalls() []LLMToolCall {
	toolCalls := make([]LLMToolCall, len(response.ToolCalls))
	copy(toolCalls, response.ToolCalls)

	return toolCalls
}

type LLMProvider interface {
	ChatCompletion(params ChatCompletionParams) (openai.ChatCompletionResponse, error)
}

type LLMProviderOpenaiCompatible interface {
	ChatCompletion(params openai.ChatCompletionRequest) (LLMCommonResponse, error)
}

func NormalizeOpenaiResponse(response openai.ChatCompletionResponse) LLMCommonResponse {
	if len(response.Choices) == 0 {
		return NormalizedResponse{}
	}

	return NormalizeOpenaiChoice(response.Choices[0])
}

func NormalizeOpenaiChoice(choice openai.ChatCompletionChoice) LLMCommonResponse {
	message := normalizeOpenaiMessage(choice.Message)
	message.FinishReason = string(choice.FinishReason)
	return message
}

func NormalizeOpenaiMessage(message openai.ChatCompletionMessage) LLMCommonResponse {
	return normalizeOpenaiMessage(message)
}

func normalizeOpenaiMessage(message openai.ChatCompletionMessage) NormalizedResponse {
	toolCalls := make([]LLMToolCall, 0, len(message.ToolCalls)+1)
	for _, toolCall := range message.ToolCalls {
		toolCalls = append(toolCalls, LLMToolCall{
			ID:        toolCall.ID,
			Name:      toolCall.Function.Name,
			Arguments: toolCall.Function.Arguments,
			Type:      string(toolCall.Type),
		})
	}

	if message.FunctionCall != nil {
		toolCalls = append(toolCalls, LLMToolCall{
			Name:      message.FunctionCall.Name,
			Arguments: message.FunctionCall.Arguments,
			Type:      string(openai.ToolTypeFunction),
		})
	}

	return NormalizedResponse{
		Content:      message.Content,
		FinishReason: inferFinishReason(toolCalls),
		ToolCalls:    toolCalls,
	}
}

func inferFinishReason(toolCalls []LLMToolCall) string {
	if len(toolCalls) > 0 {
		return "tool_calls"
	}

	return "stop"
}

func FirstToolCallName(response LLMCommonResponse) (string, bool) {
	toolCalls := response.GetToolCalls()
	if len(toolCalls) == 0 {
		return "", false
	}

	return toolCalls[0].Name, true
}

func BuildOpenaiRequestParams(params ChatCompletionParams) openai.ChatCompletionRequest {
	messages := cloneMessages(params.Messages)

	tools := make([]openai.Tool, 0, len(params.Tools))
	for _, tool := range params.Tools {
		copiedTool := tool
		if tool.Function != nil {
			copiedFunction := *tool.Function
			copiedTool.Function = &copiedFunction
		}
		tools = append(tools, copiedTool)
	}

	metadata := make(map[string]string, len(params.Metadata))
	maps.Copy(metadata, params.Metadata)

	var responseFormat *openai.ChatCompletionResponseFormat
	if params.ResponseFormat != nil {
		copied := *params.ResponseFormat
		responseFormat = &copied
	}

	request := openai.ChatCompletionRequest{
		Model:               params.Model,
		Messages:            messages,
		MaxCompletionTokens: params.MaxCompletionTokens,
		Temperature:         float32(params.Temperature),
		ResponseFormat:      responseFormat,
		Tools:               tools,
		ReasoningEffort:     params.ReasoningEffort,
		Metadata:            metadata,
	}

	if params.ToolChoice != nil {
		request.ToolChoice = buildOpenaiToolChoice(*params.ToolChoice)
	}

	return request
}

func BuildAssistantMessage(response LLMCommonResponse) openai.ChatCompletionMessage {
	message := openai.ChatCompletionMessage{
		Role:    openai.ChatMessageRoleAssistant,
		Content: response.GetContent(),
	}

	toolCalls := response.GetToolCalls()
	if len(toolCalls) == 1 && toolCalls[0].ID == "" {
		message.FunctionCall = &openai.FunctionCall{
			Name:      toolCalls[0].Name,
			Arguments: toolCalls[0].Arguments,
		}
		return message
	}

	message.ToolCalls = make([]openai.ToolCall, 0, len(toolCalls))
	for _, toolCall := range toolCalls {
		message.ToolCalls = append(message.ToolCalls, openai.ToolCall{
			ID:   toolCall.ID,
			Type: openai.ToolType(toolCall.Type),
			Function: openai.FunctionCall{
				Name:      toolCall.Name,
				Arguments: toolCall.Arguments,
			},
		})
	}

	return message
}

func cloneMessages(messages []openai.ChatCompletionMessage) []openai.ChatCompletionMessage {
	clonedMessages := make([]openai.ChatCompletionMessage, 0, len(messages))
	for _, message := range messages {
		var functionCall *openai.FunctionCall
		if message.FunctionCall != nil {
			copied := *message.FunctionCall
			functionCall = &copied
		}

		toolCalls := make([]openai.ToolCall, 0, len(message.ToolCalls))
		for _, toolCall := range message.ToolCalls {
			toolCalls = append(toolCalls, toolCall)
		}

		clonedMessages = append(clonedMessages, openai.ChatCompletionMessage{
			Role:         message.Role,
			Content:      message.Content,
			Name:         message.Name,
			Refusal:      message.Refusal,
			FunctionCall: functionCall,
			ToolCalls:    toolCalls,
			ToolCallID:   message.ToolCallID,
		})
	}

	return clonedMessages
}

func buildOpenaiToolChoice(tool openai.Tool) openai.ToolChoice {
	choice := openai.ToolChoice{Type: tool.Type}
	if tool.Function != nil {
		choice.Function = openai.ToolFunction{Name: tool.Function.Name}
	}

	return choice
}
