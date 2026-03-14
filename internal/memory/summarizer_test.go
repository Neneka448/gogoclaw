package memory

import (
	"testing"

	"github.com/Neneka448/gogoclaw/internal/provider"
	openai "github.com/sashabaranov/go-openai"
)

type fakeSummarizerLLM struct {
	response provider.LLMCommonResponse
	requests []openai.ChatCompletionRequest
}

func (llm *fakeSummarizerLLM) ChatCompletion(request openai.ChatCompletionRequest) (provider.LLMCommonResponse, error) {
	llm.requests = append(llm.requests, request)
	return llm.response, nil
}

func TestSummarizeSessionRequestsJSONObjectAndNormalizesArrays(t *testing.T) {
	llm := &fakeSummarizerLLM{response: provider.NormalizedResponse{Content: `{"who":["user","assistant"],"what":"inspect sessions","when":"unknown","where":"workspace","why":"understand available files","how":"listed the session files","result":["partial","needs follow-up"]}`}}
	summarizer := NewSummarizer(llm, "test-model")

	summary, err := summarizer.SummarizeSession("user: list sessions")
	if err != nil {
		t.Fatalf("SummarizeSession() error = %v", err)
	}
	if summary.Who != "user, assistant" {
		t.Fatalf("summary.Who = %q, want user, assistant", summary.Who)
	}
	if summary.Result != "partial, needs follow-up" {
		t.Fatalf("summary.Result = %q, want partial, needs follow-up", summary.Result)
	}
	if len(llm.requests) != 1 {
		t.Fatalf("len(requests) = %d, want 1", len(llm.requests))
	}
	if llm.requests[0].ResponseFormat == nil || llm.requests[0].ResponseFormat.Type != openai.ChatCompletionResponseFormatTypeJSONObject {
		t.Fatalf("ResponseFormat = %#v, want json_object", llm.requests[0].ResponseFormat)
	}
}

func TestSummarizeCommunityStripsCodeFenceBeforeParsing(t *testing.T) {
	llm := &fakeSummarizerLLM{response: provider.NormalizedResponse{Content: "```json\n{\"who\":\"team\",\"what\":\"merge memory\",\"when\":\"recurring\",\"where\":\"repo\",\"why\":\"capture pattern\",\"how\":\"combine similar cases\",\"result\":\"mostly successful\"}\n```"}}
	summarizer := NewSummarizer(llm, "test-model")

	summary, err := summarizer.SummarizeCommunity([]MemoryNode{{Summary: "one"}})
	if err != nil {
		t.Fatalf("SummarizeCommunity() error = %v", err)
	}
	if summary.What != "merge memory" {
		t.Fatalf("summary.What = %q, want merge memory", summary.What)
	}
}