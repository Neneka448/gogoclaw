package cmd

import (
	onboardcli "github.com/Neneka448/gogoclaw/internal/cli/onboard"
	"github.com/spf13/cobra"
)

var (
	profilePath string
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
	onboardCmd.Flags().StringVarP(&profilePath, "profile", "f", "~/.gogoclaw", "path to profile file (default: ~/.gogoclaw, and will create a default config ~/.gogoclaw/config.json)")
	onboardCmd.Flags().StringVarP(&provider, "provider", "p", "", "provider name(openrouter, codex), default is not set")
	onboardCmd.Flags().StringVarP(&model, "model", "m", "", "model name to use for the selected provider, default is not set")
	onboardCmd.Flags().StringVarP(&apikey, "apikey", "k", "", "your apikey used to connect the provider, default is not set")
	onboardCmd.Flags().StringVarP(&workspace, "workspace", "w", "~/.gogoclaw/workspace", "workspace path(default:~/.gogoclaw/workspace, if profile is set, it will be <profile>/workspace)")
	onboardCmd.Flags().BoolVarP(&interactive, "interactive", "i", false, "interactive mode")
}
