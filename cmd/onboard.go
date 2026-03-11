package cmd

import (
	"github.com/spf13/cobra"
)

var (
	profilePath string
	provider    string
	apikey      string
	workspace   string
	interactive bool
)

var onboardCmd = &cobra.Command{
	Use:   "onboard",
	Short: "Onboard a new agent",
	Run: func(cmd *cobra.Command, args []string) {

	},
}

func init() {
	rootCmd.AddCommand(onboardCmd)
	onboardCmd.Flags().StringVarP(&profilePath, "profile", "f", "~/.gogoclaw", "path to profile file (default: ~/.gogoclaw, and will create a default config ~/.gogoclaw/config.json)")
	onboardCmd.Flags().StringVarP(&provider, "provider", "p", "", "provider name(openrouter, codex), default is not set")
	onboardCmd.Flags().StringVarP(&apikey, "apikey", "k", "", "your apikey used to connect the provider, default is not set")
	onboardCmd.Flags().StringVarP(&workspace, "workspace", "w", "~/.gogoclaw/workspace", "workspace path(default:~/.gogoclaw/workspace, if profile is set, it will be <profile>/workspace)")
	onboardCmd.Flags().BoolVarP(&interactive, "interactive", "i", false, "interactive mode")
}
