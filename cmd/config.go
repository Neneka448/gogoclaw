package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
	"github.com/spf13/cobra"
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Read or write config.json fields",
}

var configGetCmd = &cobra.Command{
	Use:   "get <path>",
	Short: "Get a config field value by dot-notation path",
	Long: `Get a config field value by dot-notation path.

Path examples:
  agents.profiles.default.model
  providers.0.auth.token
  agents.profiles.default.maxTokens`,
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cfgPath, err := resolveConfigPath(cfgFile)
		if err != nil {
			return err
		}
		content, err := os.ReadFile(cfgPath)
		if err != nil {
			return fmt.Errorf("read config: %w", err)
		}

		result := gjson.GetBytes(content, args[0])
		if !result.Exists() {
			return fmt.Errorf("path %q not found", args[0])
		}
		fmt.Fprintln(cmd.OutOrStdout(), result.String())
		return nil
	},
}

var configSetCmd = &cobra.Command{
	Use:   "set <path> <value>",
	Short: "Set a config field by dot-notation path",
	Long: `Set a config field by dot-notation path.

Path examples:
  agents.profiles.default.model
  providers.0.auth.token
  agents.profiles.default.maxTokens

Values are parsed as JSON when possible (numbers, booleans, null),
otherwise stored as plain strings.`,
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		cfgPath, err := resolveConfigPath(cfgFile)
		if err != nil {
			return err
		}
		content, err := os.ReadFile(cfgPath)
		if err != nil {
			return fmt.Errorf("read config: %w", err)
		}

		value := parseConfigValue(args[1])

		updated, err := sjson.SetBytesOptions(content, args[0], value, &sjson.Options{ReplaceInPlace: false})
		if err != nil {
			return fmt.Errorf("set %q: %w", args[0], err)
		}

		if err := os.WriteFile(cfgPath, updated, 0644); err != nil {
			return fmt.Errorf("write config: %w", err)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Set %s = %v\n", args[0], args[1])
		return nil
	},
}

func init() {
	rootCmd.AddCommand(configCmd)
	configCmd.AddCommand(configGetCmd)
	configCmd.AddCommand(configSetCmd)
}

// parseConfigValue tries to decode the string as a JSON scalar so that numbers,
// booleans, and null are stored with the correct type. Falls back to plain string.
func parseConfigValue(s string) any {
	s = strings.TrimSpace(s)
	var v any
	if err := json.Unmarshal([]byte(s), &v); err == nil {
		return v
	}
	return s
}
