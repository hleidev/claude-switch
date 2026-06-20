// Package cmd wires the claude-switch CLI. The PATH binary is named
// claude-switch; users invoke it through the `cs` shell function (see init.go),
// which intercepts env-mutating subcommands and passes the rest through.
package cmd

import (
	"github.com/spf13/cobra"

	"github.com/hleidev/claude-switch/internal/config"
	"github.com/hleidev/claude-switch/internal/presets"
)

func init() {
	// Let config minimization resolve built-in presets without the config
	// package importing them (keeps it decoupled from the embedded data).
	config.SetPresetLookup(presets.Lookup)
}

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
