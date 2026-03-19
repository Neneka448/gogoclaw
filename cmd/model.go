package cmd

import (
	"fmt"
	"strings"

	cliauth "github.com/Neneka448/gogoclaw/internal/cli/auth"
	"github.com/Neneka448/gogoclaw/internal/config"
	providerpkg "github.com/Neneka448/gogoclaw/internal/provider"
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

		models, err := providerpkg.ListModels(manager, providerName, codexTokenGetter)
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

func codexTokenGetter() (string, error) {
	token, err := cliauth.GetCodexToken()
	if err != nil {
		return "", err
	}
	return token.Access, nil
}
