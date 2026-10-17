package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/hleidev/claude-switch/internal/bridge"
)

var (
	bridgeForeground bool
	bridgePort       int
)

var bridgeCmd = &cobra.Command{
	Use:   "bridge",
	Short: "Manage the local OpenAI-to-Anthropic translation bridge daemon",
}

var bridgeStartCmd = &cobra.Command{
	Use:   "start",
	Short: "Start the translation bridge daemon",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		out := cmd.OutOrStdout()
		running, pid, _ := bridge.Status(bridgePort)
		if running {
			fmt.Fprintf(out, "✓ bridge is already running on :%d (pid %d)\n", bridgePort, pid)
			return nil
		}
		if err := bridge.Start(bridgeForeground, bridgePort); err != nil {
			return err
		}
		if !bridgeForeground {
			_, pid, _ = bridge.Status(bridgePort)
			fmt.Fprintf(out, "✓ bridge started on :%d (pid %d)\n", bridgePort, pid)
		}
		return nil
	},
}

var bridgeStopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop the translation bridge daemon",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		out := cmd.OutOrStdout()
		if err := bridge.Stop(); err != nil {
			return err
		}
		fmt.Fprintln(out, "✓ bridge stopped")
		return nil
	},
}

var bridgeStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Check the status of the translation bridge daemon",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		out := cmd.OutOrStdout()
		running, pid, err := bridge.Status(bridgePort)
		if err != nil {
			return err
		}
		if running {
			fmt.Fprintf(out, "✓ bridge is running on :%d (pid %d)\n", bridgePort, pid)
		} else {
			fmt.Fprintf(out, "✗ bridge is not running on :%d\n", bridgePort)
		}
		return nil
	},
}

func init() {
	bridgeStartCmd.Flags().BoolVarP(&bridgeForeground, "foreground", "f", false, "run bridge in the foreground")
	bridgeStartCmd.Flags().IntVarP(&bridgePort, "port", "p", bridge.DefaultPort, "listen port for the bridge")
	bridgeStatusCmd.Flags().IntVarP(&bridgePort, "port", "p", bridge.DefaultPort, "listen port for the bridge")

	bridgeCmd.AddCommand(bridgeStartCmd)
	bridgeCmd.AddCommand(bridgeStopCmd)
	bridgeCmd.AddCommand(bridgeStatusCmd)

	rootCmd.AddCommand(bridgeCmd)
}
