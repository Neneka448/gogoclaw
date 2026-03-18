package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/Neneka448/gogoclaw/internal/config"
	"github.com/Neneka448/gogoclaw/internal/utils"
	openai "github.com/sashabaranov/go-openai"
)

type openRouterProvider struct {
	client  *openai.Client
	timeout time.Duration
}

func newOpenRouterProvider(providerConfig *config.ProviderConfig) (LLMProviderOpenaiCompatible, error) {
	clientConfig := openai.DefaultConfig(providerConfig.Auth.Token)
	baseURL, err := resolveProviderBaseURL(providerConfig)
	if err != nil {
		return nil, err
	}
	if baseURL != "" {
		clientConfig.BaseURL = baseURL
	}

	clientConfig.HTTPClient = &http.Client{
		Transport: &openRouterTransport{
			base:      http.DefaultTransport,
			headers:   providerConfig.Headers,
			extraBody: providerConfig.ExtraBody,
		},
	}

	timeout := providerTimeout(providerConfig.Timeout)

	return &openRouterProvider{
		client:  openai.NewClientWithConfig(clientConfig),
		timeout: timeout,
	}, nil
}

func (p *openRouterProvider) ChatCompletion(params openai.ChatCompletionRequest) (LLMCommonResponse, error) {
	ctx, cancel := context.WithTimeout(context.Background(), p.timeout)
	defer cancel()

	response, err := p.client.CreateChatCompletion(ctx, params)
	if err != nil {
		return nil, err
	}

	return NormalizeOpenaiResponse(response), nil
}

// openRouterTransport wraps an http.RoundTripper to inject custom headers and
// merge extra fields into JSON request bodies. Configured values take priority.
type openRouterTransport struct {
	base      http.RoundTripper
	headers   map[string]string
	extraBody map[string]any
}

func (t *openRouterTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	for key, value := range t.headers {
		req.Header.Set(key, value)
	}
	if len(t.extraBody) > 0 && req.Body != nil && req.Method == http.MethodPost {
		if err := mergeExtraBody(req, t.extraBody); err != nil {
			return nil, fmt.Errorf("merge extra body: %w", err)
		}
	}

	resp, err := t.base.RoundTrip(req)
	if err != nil {
		return nil, err
	}

	if utils.IsVerbose() && resp.StatusCode == http.StatusOK && resp.Body != nil {
		body, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()
		if readErr == nil {
			logOpenRouterRawUsage(body)
		}
		resp.Body = io.NopCloser(bytes.NewReader(body))
		resp.ContentLength = int64(len(body))
	}

	return resp, nil
}

// openRouterRawResponse captures the OpenRouter-specific fields that go-openai
// does not parse.
type openRouterRawResponse struct {
	Model    string                `json:"model"`
	Provider string                `json:"provider"`
	Usage    *openRouterRawUsage   `json:"usage"`
}

type openRouterRawUsage struct {
	PromptTokens      int                          `json:"prompt_tokens"`
	CompletionTokens  int                          `json:"completion_tokens"`
	TotalTokens       int                          `json:"total_tokens"`
	Cost              float64                      `json:"cost"`
	IsBYOK            bool                         `json:"is_byok"`
	PromptDetails     *openRouterPromptDetails     `json:"prompt_tokens_details"`
	CompletionDetails *openRouterCompletionDetails `json:"completion_tokens_details"`
	CostDetails       *openRouterCostDetails       `json:"cost_details"`
}

type openRouterPromptDetails struct {
	CachedTokens     int `json:"cached_tokens"`
	CacheWriteTokens int `json:"cache_write_tokens"`
}

type openRouterCompletionDetails struct {
	ReasoningTokens int `json:"reasoning_tokens"`
}

type openRouterCostDetails struct {
	InferenceCost       float64 `json:"upstream_inference_cost"`
	InferencePromptCost float64 `json:"upstream_inference_prompt_cost"`
	InferenceCompCost   float64 `json:"upstream_inference_completions_cost"`
}

func logOpenRouterRawUsage(body []byte) {
	var raw openRouterRawResponse
	if err := json.Unmarshal(body, &raw); err != nil || raw.Usage == nil {
		return
	}
	u := raw.Usage
	reasoning := 0
	if u.CompletionDetails != nil {
		reasoning = u.CompletionDetails.ReasoningTokens
	}
	cached := 0
	if u.PromptDetails != nil {
		cached = u.PromptDetails.CachedTokens
	}
	utils.Perf("openrouter: model=%s provider=%s prompt=%d completion=%d reasoning=%d cached=%d total=%d cost=$%.6f",
		raw.Model, raw.Provider, u.PromptTokens, u.CompletionTokens, reasoning, cached, u.TotalTokens, u.Cost)
}

func mergeExtraBody(req *http.Request, extra map[string]any) error {
	body, err := io.ReadAll(req.Body)
	req.Body.Close()
	if err != nil {
		return err
	}

	var parsed map[string]any
	if err := json.Unmarshal(body, &parsed); err != nil {
		// Not JSON — restore original body unchanged.
		req.Body = io.NopCloser(bytes.NewReader(body))
		req.ContentLength = int64(len(body))
		return nil
	}

	for key, value := range extra {
		parsed[key] = value
	}

	merged, err := json.Marshal(parsed)
	if err != nil {
		return err
	}
	req.Body = io.NopCloser(bytes.NewReader(merged))
	req.ContentLength = int64(len(merged))
	return nil
}
