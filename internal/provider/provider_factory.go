package provider

import (
	"context"
	"fmt"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/Neneka448/gogoclaw/internal/config"
	openai "github.com/sashabaranov/go-openai"
)

type openAICompatibleProvider struct {
	client  *openai.Client
	timeout time.Duration
}

func NewOpenAICompatibleProvider(providerConfig *config.ProviderConfig, tokenProvider TokenProvider) (LLMProviderOpenaiCompatible, error) {
	if providerConfig == nil {
		return nil, fmt.Errorf("provider config is nil")
	}
	if providerConfig.Name == "codex" {
		return newCodexProvider(providerConfig, tokenProvider)
	}
	if providerConfig.Name == "openrouter" || len(providerConfig.Headers) > 0 || len(providerConfig.ExtraBody) > 0 {
		return newOpenRouterProvider(providerConfig)
	}

	clientConfig := openai.DefaultConfig(providerConfig.Auth.Token)
	baseURL, err := resolveProviderBaseURL(providerConfig)
	if err != nil {
		return nil, err
	}
	if baseURL != "" {
		clientConfig.BaseURL = baseURL
	}

	timeout := providerTimeout(providerConfig.Timeout)

	return &openAICompatibleProvider{
		client:  openai.NewClientWithConfig(clientConfig),
		timeout: timeout,
	}, nil
}

func (provider *openAICompatibleProvider) ChatCompletion(params openai.ChatCompletionRequest) (LLMCommonResponse, error) {
	ctx, cancel := context.WithTimeout(context.Background(), provider.timeout)
	defer cancel()

	response, err := provider.client.CreateChatCompletion(ctx, params)
	if err != nil {
		return nil, err
	}

	return NormalizeOpenaiResponse(response), nil
}

func resolveProviderBaseURL(providerConfig *config.ProviderConfig) (string, error) {
	baseURL := strings.TrimSpace(providerConfig.BaseURL)
	if baseURL == "" {
		switch providerConfig.Name {
		case "openrouter":
			baseURL = "https://openrouter.ai/api/v1"
		case "codex":
			baseURL = "https://api.openai.com/v1"
		case "voyageai":
			baseURL = "https://api.voyageai.com/v1"
		}
	}

	if strings.TrimSpace(providerConfig.Path) == "" {
		return baseURL, nil
	}
	if baseURL == "" {
		return "", fmt.Errorf("provider %s path configured without base url", providerConfig.Name)
	}

	parsed, err := url.Parse(baseURL)
	if err != nil {
		return "", fmt.Errorf("parse provider base url: %w", err)
	}
	parsed.Path = path.Join(parsed.Path, providerConfig.Path)
	return parsed.String(), nil
}

func providerTimeout(timeoutSeconds int) time.Duration {
	timeout := time.Duration(timeoutSeconds) * time.Second
	if timeout <= 0 {
		return 60 * time.Second
	}
	return timeout
}
