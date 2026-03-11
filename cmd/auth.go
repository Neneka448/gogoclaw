package cmd

import (
	"fmt"
	"strings"

	cliauth "github.com/Neneka448/gogoclaw/internal/cli/auth"
	"github.com/spf13/cobra"
)

var authProvider string

var authCmd = &cobra.Command{
	Use:   "auth",
	Short: "Authenticate an OAuth-backed provider",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		switch strings.ToLower(strings.TrimSpace(authProvider)) {
		case "codex", "openai-codex", "openai_codex":
			_, err := cliauth.AuthCodex()
			if err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), "Authenticated with OpenAI Codex.")
			return nil
		default:
			return fmt.Errorf("unsupported provider: %s", authProvider)
		}
	},
}

func init() {
	rootCmd.AddCommand(authCmd)
	authCmd.Flags().StringVarP(&authProvider, "provider", "p", "codex", "OAuth provider to authenticate")
}
