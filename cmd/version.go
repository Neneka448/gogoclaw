package cmd

import (
	"fmt"

	"github.com/Neneka448/gogoclaw/internal/version"
	"github.com/spf13/cobra"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the version of gogoclaw",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("Version:", version.Version)
		fmt.Println("Build time:", version.BuildTime)
		fmt.Println("Git commit:", version.GitCommit)
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
