package utils

import (
	"testing"

	openai "github.com/sashabaranov/go-openai"
)

func TestCloneMessagesDeepCopiesNestedFields(t *testing.T) {
	original := []openai.ChatCompletionMessage{{
		Role:    openai.ChatMessageRoleAssistant,
		Content: "hello",
		Name:    "assistant",
		Refusal: "none",
		FunctionCall: &openai.FunctionCall{
			Name:      "search_docs",
			Arguments: `{"query":"go"}`,
		},
		ToolCalls: []openai.ToolCall{{
			ID:   "call_1",
			Type: openai.ToolTypeFunction,
			Function: openai.FunctionCall{
				Name:      "read_file",
				Arguments: `{"path":"README.md"}`,
			},
		}},
		ToolCallID: "call_1",
	}}

	cloned := CloneMessages(original)
	cloned[0].Content = "mutated"
	cloned[0].FunctionCall.Name = "mutated_function"
	cloned[0].ToolCalls[0].Function.Name = "mutated_tool"

	if original[0].Content != "hello" {
		t.Fatalf("original content = %q, want hello", original[0].Content)
	}
	if original[0].FunctionCall == nil || original[0].FunctionCall.Name != "search_docs" {
		t.Fatalf("original FunctionCall = %#v, want search_docs", original[0].FunctionCall)
	}
	if len(original[0].ToolCalls) != 1 || original[0].ToolCalls[0].Function.Name != "read_file" {
		t.Fatalf("original ToolCalls = %#v, want read_file", original[0].ToolCalls)
	}
}
