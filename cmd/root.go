package cmd

import "github.com/spf13/cobra"

var rootCmd = &cobra.Command{
	Use:   "gogoclaw",
	Short: "🎸GogoClaw\n A golang implementation of Openclaw",
}

var cfgFile string

func Execute() error {
	return rootCmd.Execute()
}

func init() {
	rootCmd.PersistentFlags().StringVarP(&cfgFile, "config", "c", "", "config file (default is $HOME/.gogoclaw/config.json)")
}
