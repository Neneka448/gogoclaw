package provider

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Neneka448/gogoclaw/internal/config"
	openai "github.com/sashabaranov/go-openai"
)

type stubTokenProvider struct {
	accessToken string
	accountID   string
	err         error
}

func (provider stubTokenProvider) GetToken() (string, string, error) {
	return provider.accessToken, provider.accountID, provider.err
}

func TestNewOpenAICompatibleProviderRejectsCodexWithoutTokenProvider(t *testing.T) {
	_, err := NewOpenAICompatibleProvider(&config.ProviderConfig{Name: "codex"}, nil)
	if err == nil {
		t.Fatal("NewOpenAICompatibleProvider() error = nil, want missing token provider error")
	}
}

func TestCodexProviderUsesInjectedTokenProvider(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Path; got != "/v1/responses" {
			t.Fatalf("r.URL.Path = %q, want /v1/responses", got)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer access-token" {
			t.Fatalf("Authorization = %q, want Bearer access-token", got)
		}
		if got := r.Header.Get("chatgpt-account-id"); got != "account-123" {
			t.Fatalf("chatgpt-account-id = %q, want account-123", got)
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("io.ReadAll() error = %v", err)
		}
		if len(body) == 0 {
			t.Fatal("request body is empty")
		}

		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"type\":\"response.output_text.delta\",\"delta\":\"hello\"}\n\n"))
		_, _ = w.Write([]byte("data: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\"}}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer server.Close()

	rawProvider, err := newCodexProvider(&config.ProviderConfig{Name: "codex", BaseURL: server.URL}, stubTokenProvider{accessToken: "access-token", accountID: "account-123"})
	if err != nil {
		t.Fatalf("newCodexProvider() error = %v", err)
	}
	provider, ok := rawProvider.(*codexProvider)
	if !ok {
		t.Fatalf("provider type = %T, want *codexProvider", rawProvider)
	}
	provider.timeout = time.Second

	response, err := provider.ChatCompletion(openai.ChatCompletionRequest{
		Model: "openai-codex/gpt-5.4",
		Messages: []openai.ChatCompletionMessage{
			{Role: openai.ChatMessageRoleSystem, Content: "You are helpful."},
			{Role: openai.ChatMessageRoleUser, Content: "hello"},
		},
	})
	if err != nil {
		t.Fatalf("ChatCompletion() error = %v", err)
	}
	if response.GetContent() != "hello" {
		t.Fatalf("response.GetContent() = %q, want hello", response.GetContent())
	}
	if response.GetFinishReason() != "stop" {
		t.Fatalf("response.GetFinishReason() = %q, want stop", response.GetFinishReason())
	}
}

func TestResolveCodexEndpointDefaultsToChatGPTBackend(t *testing.T) {
	endpoint, err := resolveCodexEndpoint(&config.ProviderConfig{Name: "codex"})
	if err != nil {
		t.Fatalf("resolveCodexEndpoint() error = %v", err)
	}
	if endpoint != defaultCodexURL {
		t.Fatalf("endpoint = %q, want %q", endpoint, defaultCodexURL)
	}
}
