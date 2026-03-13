package cmd

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/Neneka448/gogoclaw/internal/bootstrap"
	mcppkg "github.com/Neneka448/gogoclaw/internal/mcp"
	"github.com/spf13/cobra"
)

var mcpRestartName string

var mcpCmd = &cobra.Command{
	Use:   "mcp",
	Short: "Inspect and manage configured MCP servers",
	Args:  cobra.NoArgs,
}

var mcpListCmd = &cobra.Command{
	Use:   "list",
	Short: "List configured MCP servers and their status",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		service, err := loadMCPService(false)
		if err != nil {
			return err
		}
		defer service.Close()

		return printMCPStatuses(service.Statuses())
	},
}

var mcpRestartCmd = &cobra.Command{
	Use:   "restart",
	Short: "Reconnect one configured MCP server",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		if mcpRestartName == "" {
			return fmt.Errorf("flag --name requires a non-empty value")
		}
		service, err := loadMCPService(false)
		if err != nil {
			return err
		}
		defer service.Close()

		status, err := service.Restart(mcpRestartName)
		if err != nil {
			_ = printMCPStatuses([]mcppkg.Status{status})
			return err
		}
		return printMCPStatuses([]mcppkg.Status{status})
	},
}

func init() {
	rootCmd.AddCommand(mcpCmd)
	mcpCmd.AddCommand(mcpListCmd)
	mcpCmd.AddCommand(mcpRestartCmd)
	mcpRestartCmd.Flags().StringVar(&mcpRestartName, "name", "", "configured MCP server name")
}

func loadMCPService(failFast bool) (mcppkg.Service, error) {
	configPath, err := resolveConfigPath(cfgFile)
	if err != nil {
		return nil, err
	}
	return bootstrap.BootstrapMCPService(configPath, failFast)
}

func printMCPStatuses(statuses []mcppkg.Status) error {
	writer := tabwriter.NewWriter(os.Stdout, 0, 8, 2, ' ', 0)
	if _, err := fmt.Fprintln(writer, "NAME\tENABLED\tTRANSPORT\tSTATE\tTOOLS\tERROR"); err != nil {
		return err
	}
	for _, status := range statuses {
		if _, err := fmt.Fprintf(
			writer,
			"%s\t%t\t%s\t%s\t%d\t%s\n",
			status.Name,
			status.Enabled,
			status.Transport,
			status.State,
			status.ToolCount,
			status.LastError,
		); err != nil {
			return err
		}
	}
	return writer.Flush()
}
