package cmd

import (
	"os"
	"os/signal"
	"syscall"

	"github.com/Neneka448/gogoclaw/internal/bootstrap"
	"github.com/spf13/cobra"
)

var gatewayCmd = &cobra.Command{
	Use:   "gateway",
	Short: "Start the message gateway and enabled channels",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		configPath, err := resolveConfigPath(cfgFile)
		if err != nil {
			return err
		}
		gatewayRef, err := bootstrap.Bootstrap(configPath)
		if err != nil {
			return err
		}
		defer (*gatewayRef).Stop()

		if err := (*gatewayRef).Start(); err != nil {
			return err
		}

		signalCh := make(chan os.Signal, 1)
		signal.Notify(signalCh, os.Interrupt, syscall.SIGTERM)
		<-signalCh
		return nil
	},
}

func init() {
	rootCmd.AddCommand(gatewayCmd)
}
