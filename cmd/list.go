package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/hleidev/claude-switch/internal/config"
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List configured providers",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		cfg, err := config.Load()
		if err != nil {
			return err
		}
		out := cmd.OutOrStdout()
		// An empty CLAUDE_SWITCH_PROVIDER means this terminal is on OAuth ("claude").
		current := os.Getenv("CLAUDE_SWITCH_PROVIDER")
		if current == "" {
			current = "claude"
		}

		fmt.Fprintln(out, "  ✓ = new-terminal default   ● = this terminal")
		for _, name := range sortedNames(cfg) {
			p := cfg.Providers[name]
			key := "no key"
			if p.AuthToken() != "" {
				key = "key set"
			}
			fmt.Fprintf(out, "%s %s  %-14s %s  (%s)\n",
				glyph(name == cfg.DefaultProvider, "✓"), glyph(name == current, "●"),
				name, resolvedBaseURL(cfg, name), key)
		}
		// claude is always an available target (OAuth fallback), not a config entry.
		fmt.Fprintf(out, "%s %s  %-14s %s\n",
			glyph(cfg.DefaultProvider == "claude", "✓"), glyph(current == "claude", "●"),
			"claude", "(OAuth — native subscription)")
		return nil
	},
}

func glyph(on bool, g string) string {
	if on {
		return g
	}
	return " "
}

func init() {
	rootCmd.AddCommand(listCmd)
}
