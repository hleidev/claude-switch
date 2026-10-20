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

var bridgeRestartCmd = &cobra.Command{
	Use:   "restart",
	Short: "Gracefully restart the translation bridge daemon",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		out := cmd.OutOrStdout()
		if err := bridge.Restart(bridgeForeground, bridgePort); err != nil {
			return err
		}
		if !bridgeForeground {
			_, pid, _ := bridge.Status(bridgePort)
			fmt.Fprintf(out, "✓ bridge restarted on :%d (pid %d)\n", bridgePort, pid)
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
		if err := bridge.Stop(bridgePort); err != nil {
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
		probe, err := bridge.ExtendedStatus(bridgePort)
		if err != nil {
			return err
		}
		running, pid, err := bridge.Status(bridgePort)
		if err != nil {
			return err
		}
		if running {
			meta := ""
			if probe != nil && probe.Hash != "" {
				h := probe.Hash
				if len(h) > 8 {
					h = h[:8]
				}
				meta = fmt.Sprintf(" [hash: %s]", h)
			}
			fmt.Fprintf(out, "✓ bridge is running on :%d (pid %d)%s\n", bridgePort, pid, meta)
			if probe != nil && probe.Hash != "" && probe.Hash != bridge.ScriptHash() {
				fmt.Fprintln(out, "  ! script version drift detected (auto-reloads on `cs use` or run `cs bridge restart`)")
			}
		} else {
			if probe != nil && probe.State == bridge.StateDraining {
				fmt.Fprintf(out, "… bridge is draining on :%d (pid %d, in-flight: %d)\n", bridgePort, probe.PID, probe.ActiveRequests)
			} else {
				fmt.Fprintf(out, "✗ bridge is not running on :%d\n", bridgePort)
			}
		}
		return nil
	},
}

func init() {
	bridgeStartCmd.Flags().BoolVarP(&bridgeForeground, "foreground", "f", false, "run bridge in the foreground")
	bridgeStartCmd.Flags().IntVarP(&bridgePort, "port", "p", bridge.DefaultPort, "listen port for the bridge")
	bridgeRestartCmd.Flags().BoolVarP(&bridgeForeground, "foreground", "f", false, "run bridge in the foreground")
	bridgeRestartCmd.Flags().IntVarP(&bridgePort, "port", "p", bridge.DefaultPort, "listen port for the bridge")
	bridgeStopCmd.Flags().IntVarP(&bridgePort, "port", "p", bridge.DefaultPort, "listen port for the bridge")
	bridgeStatusCmd.Flags().IntVarP(&bridgePort, "port", "p", bridge.DefaultPort, "listen port for the bridge")

	bridgeCmd.AddCommand(bridgeStartCmd)
	bridgeCmd.AddCommand(bridgeRestartCmd)
	bridgeCmd.AddCommand(bridgeStopCmd)
	bridgeCmd.AddCommand(bridgeStatusCmd)

	rootCmd.AddCommand(bridgeCmd)
}
