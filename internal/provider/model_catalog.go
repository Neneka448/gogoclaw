package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/Neneka448/gogoclaw/internal/config"
)

// ModelInfo is a provider-agnostic model descriptor.
type ModelInfo struct {
	ID          string
	Name        string
	Description string
}

// ListModels returns available models for the named provider.
// It uses the config manager to resolve provider settings (base URL, auth token).
// A getToken func may be provided to supply an OAuth access token when the config
// auth token is empty (used for codex-style OAuth providers).
func ListModels(manager config.ConfigManager, providerName string, getToken func() (string, error)) ([]ModelInfo, error) {
	switch strings.ToLower(providerName) {
	case "codex":
		return CodexModels(), nil
	case "openrouter":
		return listOpenRouterModels(manager)
	default:
		return listOpenAICompatibleModels(manager, providerName, getToken)
	}
}

// CodexModels returns the known Codex model IDs. Codex has no public list API.
func CodexModels() []ModelInfo {
	return []ModelInfo{
		{ID: "openai-codex/gpt-5.4", Name: "GPT-5.4"},
		{ID: "openai-codex/gpt-5.4-mini", Name: "GPT-5.4-Mini"},
		{ID: "openai-codex/gpt-5.3-codex", Name: "GPT-5.3-Codex"},
		{ID: "openai-codex/gpt-5.2-codex", Name: "GPT-5.2-Codex"},
		{ID: "openai-codex/gpt-5.2", Name: "GPT-5.2"},
		{ID: "openai-codex/gpt-5.1-codex-max", Name: "GPT-5.1-Codex-Max"},
		{ID: "openai-codex/gpt-5.1-codex-mini", Name: "GPT-5.1-Codex-Mini"},
	}
}

// listOpenRouterModels calls https://openrouter.ai/api/v1/models.
func listOpenRouterModels(manager config.ConfigManager) ([]ModelInfo, error) {
	providerCfg, err := manager.GetProviderConfig("openrouter")
	if err != nil {
		return nil, fmt.Errorf("openrouter provider config not found: %w", err)
	}

	baseURL := "https://openrouter.ai/api/v1"
	if strings.TrimSpace(providerCfg.BaseURL) != "" {
		baseURL = strings.TrimRight(providerCfg.BaseURL, "/")
	}

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, baseURL+"/models", nil)
	if err != nil {
		return nil, err
	}
	if providerCfg.Auth.Token != "" {
		req.Header.Set("Authorization", "Bearer "+providerCfg.Auth.Token)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := (&http.Client{Timeout: 20 * time.Second}).Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch OpenRouter models: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("OpenRouter models API returned HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var payload struct {
		Data []struct {
			ID          string `json:"id"`
			Name        string `json:"name"`
			Description string `json:"description"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("parse OpenRouter models response: %w", err)
	}

	models := make([]ModelInfo, 0, len(payload.Data))
	for _, m := range payload.Data {
		models = append(models, ModelInfo{ID: m.ID, Name: m.Name, Description: m.Description})
	}
	sort.Slice(models, func(i, j int) bool { return models[i].ID < models[j].ID })
	return models, nil
}

// listOpenAICompatibleModels calls GET <baseURL>/models for standard OpenAI-compatible providers.
func listOpenAICompatibleModels(manager config.ConfigManager, providerName string, getToken func() (string, error)) ([]ModelInfo, error) {
	providerCfg, err := manager.GetProviderConfig(providerName)
	if err != nil {
		return nil, fmt.Errorf("provider %q not found in config: %w", providerName, err)
	}

	baseURL := strings.TrimRight(strings.TrimSpace(providerCfg.BaseURL), "/")
	if baseURL == "" {
		return nil, fmt.Errorf("provider %q has no baseURL configured", providerName)
	}

	authToken := providerCfg.Auth.Token
	if authToken == "" && getToken != nil {
		if tok, err := getToken(); err == nil {
			authToken = tok
		}
	}

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, baseURL+"/models", nil)
	if err != nil {
		return nil, err
	}
	if authToken != "" {
		req.Header.Set("Authorization", "Bearer "+authToken)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := (&http.Client{Timeout: 20 * time.Second}).Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch models for %q: %w", providerName, err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("models API returned HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var payload struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("parse models response: %w", err)
	}

	models := make([]ModelInfo, 0, len(payload.Data))
	for _, m := range payload.Data {
		models = append(models, ModelInfo{ID: m.ID})
	}
	sort.Slice(models, func(i, j int) bool { return models[i].ID < models[j].ID })
	return models, nil
}
