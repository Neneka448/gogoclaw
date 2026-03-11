package provider

import (
	"maps"

	openai "github.com/sashabaranov/go-openai"
)

type ChatMessage struct {
	Role         string
	Content      string
	Name         string
	FunctionCall *openai.FunctionCall
	ToolCalls    []openai.ToolCall
	ToolCallID   string
}

type ChatCompletionParams struct {
	Model               string
	Messages            []ChatMessage
	MaxCompletionTokens int
	Temperature         float64
	ResponseFormat      *openai.ChatCompletionResponseFormat
	Tools               []openai.Tool
	ToolChoice          *openai.Tool
	ReasoningEffort     string
	Metadata            map[string]string
}

type LLMCommonResponse interface {
	GetContent() string
	IsToolCall() bool
}

type LLMProvider interface {
	ChatCompletion(params ChatCompletionParams) (openai.ChatCompletionResponse, error)
}

type LLMProviderOpenaiCompatible interface {
	ChatCompletion(params ChatCompletionParams) (openai.ChatCompletionResponse, error)
}

func buildRequestParams(params ChatCompletionParams) openai.ChatCompletionRequest {
	messages := make([]openai.ChatCompletionMessage, 0, len(params.Messages))
	for _, message := range params.Messages {
		var functionCall *openai.FunctionCall
		if message.FunctionCall != nil {
			copied := *message.FunctionCall
			functionCall = &copied
		}

		toolCalls := make([]openai.ToolCall, 0, len(message.ToolCalls))
		for _, toolCall := range message.ToolCalls {
			toolCalls = append(toolCalls, toolCall)
		}

		messages = append(messages, openai.ChatCompletionMessage{
			Role:         message.Role,
			Content:      message.Content,
			Name:         message.Name,
			FunctionCall: functionCall,
			ToolCalls:    toolCalls,
			ToolCallID:   message.ToolCallID,
		})
	}

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
		request.ToolChoice = buildToolChoice(*params.ToolChoice)
	}

	return request
}

func buildToolChoice(tool openai.Tool) openai.ToolChoice {
	choice := openai.ToolChoice{Type: tool.Type}
	if tool.Function != nil {
		choice.Function = openai.ToolFunction{Name: tool.Function.Name}
	}

	return choice
}
