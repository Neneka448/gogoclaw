package provider

import (
	"testing"

	openai "github.com/sashabaranov/go-openai"
)

func TestBuildRequestParams(t *testing.T) {
	params := ChatCompletionParams{
		Model: "gpt-5",
		Messages: []openai.ChatCompletionMessage{
			{Role: openai.ChatMessageRoleSystem, Content: "You are helpful."},
			{Role: openai.ChatMessageRoleUser, Content: "hello", Name: "alice"},
		},
		MaxCompletionTokens: 512,
		Temperature:         0.25,
		ReasoningEffort:     "medium",
		Metadata:            map[string]string{"trace_id": "abc123"},
	}

	got := BuildOpenaiRequestParams(params)

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
		Messages: []openai.ChatCompletionMessage{
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

	got := BuildOpenaiRequestParams(params)

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

func TestBuildAssistantMessageReturnsToolCalls(t *testing.T) {
	response := NormalizedResponse{
		Content:      "",
		FinishReason: "tool_calls",
		ToolCalls: []LLMToolCall{{
			ID:        "call_1",
			Name:      "search_docs",
			Arguments: `{"query":"react"}`,
			Type:      string(openai.ToolTypeFunction),
		}},
	}

	message := BuildAssistantMessage(response)

	if message.Role != openai.ChatMessageRoleAssistant {
		t.Fatalf("Role = %q, want assistant", message.Role)
	}
	if len(message.ToolCalls) != 1 {
		t.Fatalf("len(ToolCalls) = %d, want 1", len(message.ToolCalls))
	}
	if message.ToolCalls[0].ID != "call_1" {
		t.Fatalf("ToolCalls[0].ID = %q, want call_1", message.ToolCalls[0].ID)
	}
	if message.FunctionCall != nil {
		t.Fatalf("FunctionCall = %#v, want nil", message.FunctionCall)
	}
}

func TestBuildAssistantMessageReturnsLegacyFunctionCall(t *testing.T) {
	response := NormalizedResponse{
		Content:      "",
		FinishReason: "tool_calls",
		ToolCalls: []LLMToolCall{{
			Name:      "search_docs",
			Arguments: `{"query":"golang"}`,
			Type:      string(openai.ToolTypeFunction),
		}},
	}

	message := BuildAssistantMessage(response)

	if message.FunctionCall == nil {
		t.Fatal("FunctionCall = nil, want non-nil")
	}
	if message.FunctionCall.Name != "search_docs" {
		t.Fatalf("FunctionCall.Name = %q, want search_docs", message.FunctionCall.Name)
	}
	if len(message.ToolCalls) != 0 {
		t.Fatalf("len(ToolCalls) = %d, want 0", len(message.ToolCalls))
	}
}

func TestNormalizeOpenaiResponseReturnsToolCalls(t *testing.T) {
	response := openai.ChatCompletionResponse{
		Choices: []openai.ChatCompletionChoice{{
			FinishReason: "tool_calls",
			Message: openai.ChatCompletionMessage{
				Content: "",
				ToolCalls: []openai.ToolCall{{
					ID:   "call_1",
					Type: openai.ToolTypeFunction,
					Function: openai.FunctionCall{
						Name:      "search_docs",
						Arguments: `{"query":"react"}`,
					},
				}},
			},
		}},
	}

	normalized := NormalizeOpenaiResponse(response)

	if !normalized.IsToolCall() {
		t.Fatal("IsToolCall = false, want true")
	}
	if normalized.GetFinishReason() != "tool_calls" {
		t.Fatalf("GetFinishReason() = %q, want tool_calls", normalized.GetFinishReason())
	}
	toolCalls := normalized.GetToolCalls()
	if len(toolCalls) != 1 {
		t.Fatalf("len(GetToolCalls()) = %d, want 1", len(toolCalls))
	}
	if toolCalls[0].Name != "search_docs" {
		t.Fatalf("toolCalls[0].Name = %q, want search_docs", toolCalls[0].Name)
	}
	toolName, ok := FirstToolCallName(normalized)
	if !ok || toolName != "search_docs" {
		t.Fatalf("FirstToolCallName() = (%q, %v), want (search_docs, true)", toolName, ok)
	}
}

func TestNormalizeOpenaiMessageIncludesLegacyFunctionCall(t *testing.T) {
	message := openai.ChatCompletionMessage{
		Content: "",
		FunctionCall: &openai.FunctionCall{
			Name:      "search_docs",
			Arguments: `{"query":"golang"}`,
		},
	}

	normalized := NormalizeOpenaiMessage(message)

	if !normalized.IsToolCall() {
		t.Fatal("IsToolCall = false, want true")
	}
	if normalized.GetFinishReason() != "tool_calls" {
		t.Fatalf("GetFinishReason() = %q, want tool_calls", normalized.GetFinishReason())
	}
	toolCalls := normalized.GetToolCalls()
	if len(toolCalls) != 1 {
		t.Fatalf("len(GetToolCalls()) = %d, want 1", len(toolCalls))
	}
	if toolCalls[0].Name != "search_docs" {
		t.Fatalf("toolCalls[0].Name = %q, want search_docs", toolCalls[0].Name)
	}
	if toolCalls[0].Type != string(openai.ToolTypeFunction) {
		t.Fatalf("toolCalls[0].Type = %q, want %q", toolCalls[0].Type, string(openai.ToolTypeFunction))
	}
}

func TestNormalizeOpenaiResponseReturnsStopFinishReason(t *testing.T) {
	response := openai.ChatCompletionResponse{
		Choices: []openai.ChatCompletionChoice{{
			FinishReason: "stop",
			Message: openai.ChatCompletionMessage{
				Role:    openai.ChatMessageRoleAssistant,
				Content: "done",
			},
		}},
	}

	normalized := NormalizeOpenaiResponse(response)

	if normalized.GetFinishReason() != "stop" {
		t.Fatalf("GetFinishReason() = %q, want stop", normalized.GetFinishReason())
	}
	if normalized.IsToolCall() {
		t.Fatal("IsToolCall = true, want false")
	}
}
