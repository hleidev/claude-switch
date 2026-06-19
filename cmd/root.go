// Package cmd wires the claude-switch CLI. The PATH binary is named
// claude-switch; users invoke it through the `cs` shell function (see init.go),
// which intercepts env-mutating subcommands and passes the rest through.
package cmd

import (
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "claude-switch",
	Short: "Per-terminal Claude Code provider switcher",
	Long: "claude-switch (cs) switches Claude Code between Claude.ai (OAuth) and\n" +
		"third-party API providers per terminal, via shell environment injection.",
	SilenceUsage: true,
}

// Execute runs the root command.
func Execute() error {
	return rootCmd.Execute()
}
