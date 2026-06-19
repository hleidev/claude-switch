package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var version = "dev"

// SetVersion records the build-injected version for `cs version`.
func SetVersion(v string) {
	if v != "" {
		version = v
	}
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the claude-switch version",
	Args:  cobra.NoArgs,
	Run: func(c *cobra.Command, _ []string) {
		fmt.Fprintln(c.OutOrStdout(), version)
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
