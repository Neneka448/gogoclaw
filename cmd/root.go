package cmd

import (
	"github.com/Neneka448/gogoclaw/internal/utils"
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "gogoclaw",
	Short: "🎸GogoClaw\n A golang implementation of Openclaw",
}

var (
	cfgFile string
	verbose bool
)

func Execute() error {
	return rootCmd.Execute()
}

func init() {
	rootCmd.PersistentFlags().StringVarP(&cfgFile, "config", "c", "", "config file (default is $HOME/.gogoclaw/config.json)")
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "enable verbose output including performance timing")
	cobra.OnInitialize(func() {
		utils.SetVerbose(verbose)
	})
}
