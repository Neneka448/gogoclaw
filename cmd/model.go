package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	cliauth "github.com/Neneka448/gogoclaw/internal/cli/auth"
	"github.com/Neneka448/gogoclaw/internal/config"
	"github.com/spf13/cobra"
)

var (
	modelListProvider  string
	modelSetProfile    string
	modelSetProvider   string
	modelSetThinkLevel string
)

var modelCmd = &cobra.Command{
	Use:   "model",
	Short: "Manage models",
}

var modelListCmd = &cobra.Command{
	Use:   "list",
	Short: "List available models for a provider",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		configPath, err := resolveConfigPath(cfgFile)
		if err != nil {
			return err
		}
		manager := config.NewConfigManager(configPath)

		providerName := strings.TrimSpace(modelListProvider)
		if providerName == "" {
			// Fall back to the default profile's provider.
			profile, err := manager.GetAgentProfileConfig("default")
			if err == nil {
				providerName = profile.Provider
			}
		}
		if providerName == "" {
			return fmt.Errorf("specify a provider with --provider (openrouter, codex, or an OpenAI-compatible name)")
		}

		models, err := fetchModels(manager, providerName)
		if err != nil {
			return err
		}

		fmt.Fprintf(cmd.OutOrStdout(), "Models for provider %q:\n\n", providerName)
		for _, m := range models {
			if m.Description != "" {
				fmt.Fprintf(cmd.OutOrStdout(), "  %-50s  %s\n", m.ID, m.Description)
			} else {
				fmt.Fprintf(cmd.OutOrStdout(), "  %s\n", m.ID)
			}
		}
		return nil
	},
}

var modelSetCmd = &cobra.Command{
	Use:   "set <model_id>",
	Short: "Set the model for a profile",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		modelID := strings.TrimSpace(args[0])
		if modelID == "" {
			return fmt.Errorf("model_id must not be empty")
		}

		configPath, err := resolveConfigPath(cfgFile)
		if err != nil {
			return err
		}
		manager := config.NewConfigManager(configPath)

		profileName := strings.TrimSpace(modelSetProfile)
		if profileName == "" {
			profileName = "default"
		}

		thinkLevel := strings.TrimSpace(modelSetThinkLevel)
		if thinkLevel != "" && thinkLevel != "low" && thinkLevel != "medium" && thinkLevel != "high" {
			return fmt.Errorf("--think must be one of: low, medium, high")
		}

		if err := manager.SetProfileModel(profileName, modelID, thinkLevel); err != nil {
			return err
		}

		msg := fmt.Sprintf("Profile %q model set to %q.", profileName, modelID)
		if thinkLevel != "" {
			msg += fmt.Sprintf(" Reasoning effort: %s.", thinkLevel)
		}
		fmt.Fprintln(cmd.OutOrStdout(), msg)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(modelCmd)
	modelCmd.AddCommand(modelListCmd)
	modelCmd.AddCommand(modelSetCmd)

	modelListCmd.Flags().StringVar(&modelListProvider, "provider", "", "provider to list models for (openrouter, codex, or custom name)")
	modelSetCmd.Flags().StringVar(&modelSetProfile, "profile", "default", "profile to update")
	modelSetCmd.Flags().StringVar(&modelSetProvider, "provider", "", "provider hint (informational only)")
	modelSetCmd.Flags().StringVar(&modelSetThinkLevel, "think", "", "reasoning effort level: low, medium, or high (codex only)")
}

// modelInfo is a provider-agnostic model descriptor.
type modelInfo struct {
	ID          string
	Name        string
	Description string
}

func fetchModels(manager config.ConfigManager, providerName string) ([]modelInfo, error) {
	switch strings.ToLower(providerName) {
	case "codex":
		return codexModels(), nil
	case "openrouter":
		return fetchOpenRouterModels(manager)
	default:
		return fetchOpenAICompatibleModels(manager, providerName)
	}
}

// codexModels returns the known Codex model IDs (no public list API).
func codexModels() []modelInfo {
	return []modelInfo{
		{ID: "openai-codex/gpt-5.4", Name: "GPT-5.4"},
		{ID: "openai-codex/gpt-5.4-mini", Name: "GPT-5.4-Mini"},
		{ID: "openai-codex/gpt-5.3-codex", Name: "GPT-5.3-Codex"},
		{ID: "openai-codex/gpt-5.2-codex", Name: "GPT-5.2-Codex"},
		{ID: "openai-codex/gpt-5.2", Name: "GPT-5.2"},
		{ID: "openai-codex/gpt-5.1-codex-max", Name: "GPT-5.1-Codex-Max"},
		{ID: "openai-codex/gpt-5.1-codex-mini", Name: "GPT-5.1-Codex-Mini"},
	}
}

// fetchOpenRouterModels calls https://openrouter.ai/api/v1/models.
func fetchOpenRouterModels(manager config.ConfigManager) ([]modelInfo, error) {
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

	models := make([]modelInfo, 0, len(payload.Data))
	for _, m := range payload.Data {
		models = append(models, modelInfo{ID: m.ID, Name: m.Name, Description: m.Description})
	}
	sort.Slice(models, func(i, j int) bool { return models[i].ID < models[j].ID })
	return models, nil
}

// fetchOpenAICompatibleModels calls GET <baseURL>/models for standard providers.
func fetchOpenAICompatibleModels(manager config.ConfigManager, providerName string) ([]modelInfo, error) {
	providerCfg, err := manager.GetProviderConfig(providerName)
	if err != nil {
		return nil, fmt.Errorf("provider %q not found in config: %w", providerName, err)
	}

	// Build base URL.
	baseURL := strings.TrimSpace(providerCfg.BaseURL)
	if baseURL == "" {
		return nil, fmt.Errorf("provider %q has no baseURL configured", providerName)
	}
	baseURL = strings.TrimRight(baseURL, "/")

	// Resolve auth token — for codex-like OAuth providers, try GetCodexToken.
	authToken := providerCfg.Auth.Token
	if authToken == "" {
		if token, err := cliauth.GetCodexToken(); err == nil {
			authToken = token.Access
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
		Object string `json:"object"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("parse models response: %w", err)
	}

	models := make([]modelInfo, 0, len(payload.Data))
	for _, m := range payload.Data {
		models = append(models, modelInfo{ID: m.ID})
	}
	sort.Slice(models, func(i, j int) bool { return models[i].ID < models[j].ID })
	return models, nil
}
