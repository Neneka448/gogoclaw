package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

var (
	message       string
	interactAgent bool
)

var agentCmd = &cobra.Command{
	Use:   "agent",
	Short: "Run the agent command flow",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		if cmd.Flags().Changed("message") && strings.TrimSpace(message) == "" {
			return fmt.Errorf("flag --message requires a non-empty message")
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(agentCmd)
	agentCmd.Flags().StringVarP(&message, "message", "m", "", "single message to send to the agent; must be non-empty when provided")
	agentCmd.Flags().BoolVarP(&interactAgent, "interactive", "i", false, "run the agent in interactive mode")
}
