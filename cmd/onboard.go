package cmd

import (
	onboardcli "github.com/Neneka448/gogoclaw/internal/cli/onboard"
	"github.com/spf13/cobra"
)

var (
	profilePath string
	profileName string
	provider    string
	model       string
	apikey      string
	workspace   string
	interactive bool
)

var onboardCmd = &cobra.Command{
	Use:   "onboard",
	Short: "Onboard a new agent",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return onboardcli.RunOnboard(onboardcli.OnboardOptions{
			ProfilePath: profilePath,
			ProfileName: profileName,
			Provider:    provider,
			Model:       model,
			APIKey:      apikey,
			Workspace:   workspace,
			Interactive: interactive,
		})
	},
}

func init() {
	rootCmd.AddCommand(onboardCmd)
	onboardCmd.Flags().StringVarP(&profilePath, "profile", "f", "~/.gogoclaw", "config directory path (default: ~/.gogoclaw, stores config.json)")
	onboardCmd.Flags().StringVar(&profileName, "profile-name", "default", "agent profile name to create or update in config.json")
	onboardCmd.Flags().StringVarP(&provider, "provider", "p", "", "provider name(openrouter, codex), default is not set")
	onboardCmd.Flags().StringVarP(&model, "model", "m", "", "model name to use for the selected provider, default is not set")
	onboardCmd.Flags().StringVarP(&apikey, "apikey", "k", "", "your apikey used to connect the provider, default is not set")
	onboardCmd.Flags().StringVarP(&workspace, "workspace", "w", "", "workspace path (default: <config-dir>/workspace)")
	onboardCmd.Flags().BoolVarP(&interactive, "interactive", "i", true, "interactive mode")
}
