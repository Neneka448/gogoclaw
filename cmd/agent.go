package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Neneka448/gogoclaw/internal/bootstrap"
	messagebus "github.com/Neneka448/gogoclaw/internal/message_bus"
	"github.com/spf13/cobra"
)

var (
	message       string
	interactAgent bool
	sessionID     string
)

var agentCmd = &cobra.Command{
	Use:   "agent",
	Short: "Run the agent command flow",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		if cmd.Flags().Changed("message") && strings.TrimSpace(message) == "" {
			return fmt.Errorf("flag --message requires a non-empty message")
		}
		if strings.TrimSpace(message) == "" {
			return fmt.Errorf("flag --message requires a non-empty message")
		}
		if strings.TrimSpace(sessionID) == "" {
			return fmt.Errorf("flag --session requires a non-empty value")
		}

		configPath, err := resolveConfigPath(cfgFile)
		if err != nil {
			return err
		}
		gatewayRef, err := bootstrap.Bootstrap(configPath)
		if err != nil {
			return err
		}
		defer (*gatewayRef).Stop()

		_, err = (*gatewayRef).DirectProcessAndReturn(messagebus.Message{
			ChannelID: "cli",
			ChatID:    strings.TrimSpace(sessionID),
			Message:   message,
		})
		if err != nil {
			return err
		}
		// for _, response := range responses {
		// 	if strings.TrimSpace(response.Message) == "" {
		// 		continue
		// 	}
		// 	fmt.Fprintln(cmd.OutOrStdout(), response.Message)
		// }

		return nil
	},
}

func init() {
	rootCmd.AddCommand(agentCmd)
	agentCmd.Flags().StringVarP(&message, "message", "m", "", "single message to send to the agent; must be non-empty when provided")
	agentCmd.Flags().BoolVarP(&interactAgent, "interactive", "i", false, "run the agent in interactive mode")
	agentCmd.Flags().StringVar(&sessionID, "session", "default", "session id to reuse for CLI conversation state")
}

func resolveConfigPath(configPath string) (string, error) {
	if strings.TrimSpace(configPath) != "" {
		return configPath, nil
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	return filepath.Join(homeDir, ".gogoclaw", "config.json"), nil
}
