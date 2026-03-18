package utils

import openai "github.com/sashabaranov/go-openai"

func CloneMessages(messages []openai.ChatCompletionMessage) []openai.ChatCompletionMessage {
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
