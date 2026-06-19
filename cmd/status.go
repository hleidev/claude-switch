package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/hleidev/claude-switch/internal/config"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show the current terminal's provider and config summary",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		cfg, err := config.Load()
		if err != nil {
			return err
		}
		out := cmd.OutOrStdout()
		path, _ := config.ConfigPath()

		current := os.Getenv("CLAUDE_SWITCH_PROVIDER")
		if current == "" {
			fmt.Fprintln(out, "This terminal:   claude (OAuth) — no provider injected")
		} else {
			fmt.Fprintf(out, "This terminal:   %s\n", current)
			fmt.Fprintf(out, "  base_url:      %s\n", os.Getenv("ANTHROPIC_BASE_URL"))
			fmt.Fprintf(out, "  model:         %s\n", os.Getenv("ANTHROPIC_MODEL"))
			if os.Getenv("ANTHROPIC_AUTH_TOKEN") != "" {
				fmt.Fprintln(out, "  auth_token:    set (hidden)")
			} else {
				fmt.Fprintln(out, "  auth_token:    not set")
			}
		}
		fmt.Fprintf(out, "New-terminal default: %s\n", cfg.DefaultProvider)
		fmt.Fprintf(out, "Config:          %s\n", path)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(statusCmd)
}
