package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/hleidev/claude-switch/internal/config"
)

var unsetCmd = &cobra.Command{
	Use:               "unset <provider> <field>",
	Short:             "Clear a single provider field",
	Args:              cobra.ExactArgs(2),
	ValidArgsFunction: providerNames,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return err
		}
		provider, field := args[0], args[1]
		if err := cfg.UnsetField(provider, field); err != nil {
			return err
		}
		if err := config.Save(cfg); err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "✓ cleared %s.%s\n", provider, field)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(unsetCmd)
}
