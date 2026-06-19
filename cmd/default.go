package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/hleidev/claude-switch/internal/config"
)

var defaultCmd = &cobra.Command{
	Use:               "default [provider]",
	Short:             "Show or set the provider loaded in new terminals",
	Args:              cobra.MaximumNArgs(1),
	ValidArgsFunction: providerNames,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return err
		}
		out := cmd.OutOrStdout()

		if len(args) == 0 {
			fmt.Fprintln(out, cfg.DefaultProvider)
			return nil
		}
		name := args[0]
		if name != "claude" {
			if _, ok := cfg.Providers[name]; !ok {
				return fmt.Errorf("provider %q not found (add it with: cs add %s)", name, name)
			}
		}
		cfg.DefaultProvider = name
		if err := config.Save(cfg); err != nil {
			return err
		}
		if name == "claude" {
			fmt.Fprintln(out, "✓ New terminals will use claude (OAuth)")
		} else {
			fmt.Fprintf(out, "✓ New terminals will default to %s\n", name)
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(defaultCmd)
}
