package provider

import (
	"testing"

	openai "github.com/sashabaranov/go-openai"
)

func TestBuildRequestParams(t *testing.T) {
	params := ChatCompletionParams{
		Model: "gpt-5",
		Messages: []ChatMessage{
			{Role: openai.ChatMessageRoleSystem, Content: "You are helpful."},
			{Role: openai.ChatMessageRoleUser, Content: "hello", Name: "alice"},
		},
		MaxCompletionTokens: 512,
		Temperature:         0.25,
		ReasoningEffort:     "medium",
		Metadata:            map[string]string{"trace_id": "abc123"},
	}

	got := buildRequestParams(params)

	if got.Model != params.Model {
		t.Fatalf("Model = %q, want %q", got.Model, params.Model)
	}
	if got.MaxCompletionTokens != params.MaxCompletionTokens {
		t.Fatalf("MaxCompletionTokens = %d, want %d", got.MaxCompletionTokens, params.MaxCompletionTokens)
	}
	if got.Temperature != float32(params.Temperature) {
		t.Fatalf("Temperature = %v, want %v", got.Temperature, float32(params.Temperature))
	}
	if got.ReasoningEffort != params.ReasoningEffort {
		t.Fatalf("ReasoningEffort = %q, want %q", got.ReasoningEffort, params.ReasoningEffort)
	}
	if got.Metadata["trace_id"] != "abc123" {
		t.Fatalf("Metadata = %#v, want trace_id", got.Metadata)
	}
	if len(got.Messages) != 2 {
		t.Fatalf("len(Messages) = %d, want 2", len(got.Messages))
	}
	if got.Messages[0].Role != openai.ChatMessageRoleSystem || got.Messages[0].Content != "You are helpful." {
		t.Fatalf("first message = %#v, want system helper message", got.Messages[0])
	}
	if got.Messages[1].Role != openai.ChatMessageRoleUser || got.Messages[1].Content != "hello" || got.Messages[1].Name != "alice" {
		t.Fatalf("second message = %#v, want user message with name", got.Messages[1])
	}
	params.Metadata["trace_id"] = "mutated"
	if got.Metadata["trace_id"] != "abc123" {
		t.Fatalf("Metadata should be copied, got %#v", got.Metadata)
	}
}

func TestBuildRequestParamsIncludesToolCallingFields(t *testing.T) {
	selectedTool := openai.Tool{
		Type: openai.ToolTypeFunction,
		Function: &openai.FunctionDefinition{
			Name:        "search_docs",
			Description: "search project docs",
			Parameters:  map[string]any{"type": "object"},
		},
	}

	params := ChatCompletionParams{
		Model: "gpt-5",
		Messages: []ChatMessage{
			{
				Role: openai.ChatMessageRoleAssistant,
				ToolCalls: []openai.ToolCall{{
					ID:   "call_1",
					Type: openai.ToolTypeFunction,
					Function: openai.FunctionCall{
						Name:      "search_docs",
						Arguments: `{"query":"react"}`,
					},
				}},
			},
			{
				Role:       openai.ChatMessageRoleTool,
				Content:    `{"result":"ok"}`,
				ToolCallID: "call_1",
			},
		},
		Tools:      []openai.Tool{selectedTool},
		ToolChoice: &selectedTool,
		ResponseFormat: &openai.ChatCompletionResponseFormat{
			Type: openai.ChatCompletionResponseFormatTypeJSONObject,
		},
	}

	got := buildRequestParams(params)

	if len(got.Tools) != 1 || got.Tools[0].Function == nil || got.Tools[0].Function.Name != "search_docs" {
		t.Fatalf("Tools = %#v, want search_docs tool", got.Tools)
	}
	toolChoice, ok := got.ToolChoice.(openai.ToolChoice)
	if !ok {
		t.Fatalf("ToolChoice type = %T, want openai.ToolChoice", got.ToolChoice)
	}
	if toolChoice.Function.Name != "search_docs" {
		t.Fatalf("ToolChoice = %#v, want search_docs", toolChoice)
	}
	if got.ResponseFormat == nil || got.ResponseFormat.Type != openai.ChatCompletionResponseFormatTypeJSONObject {
		t.Fatalf("ResponseFormat = %#v, want json_object", got.ResponseFormat)
	}
	if len(got.Messages) != 2 || len(got.Messages[0].ToolCalls) != 1 {
		t.Fatalf("Messages = %#v, want assistant tool call + tool response", got.Messages)
	}
	if got.Messages[0].ToolCalls[0].Function.Name != "search_docs" {
		t.Fatalf("first message tool call = %#v, want search_docs", got.Messages[0].ToolCalls[0])
	}
	if got.Messages[1].ToolCallID != "call_1" {
		t.Fatalf("second message ToolCallID = %q, want call_1", got.Messages[1].ToolCallID)
	}
}
