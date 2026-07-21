package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

// useCmd only ever runs when the shell function is missing — once installed, it
// shadows this binary and handles `use` itself.
var useCmd = &cobra.Command{
	Use:   "use <provider|claude>",
	Short: "Switch this terminal's provider (requires shell integration)",
	Args:  cobra.ArbitraryArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		return fmt.Errorf("`use` needs shell integration — run `cs setup`, then open a new terminal")
	},
}

func init() {
	rootCmd.AddCommand(useCmd)
}
