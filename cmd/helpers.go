package cmd

import (
	"sort"

	"github.com/spf13/cobra"

	"github.com/hleidev/claude-switch/internal/config"
)

// sortedNames returns the configured provider names, sorted.
func sortedNames(cfg *config.Config) []string {
	names := make([]string, 0, len(cfg.Providers))
	for k := range cfg.Providers {
		names = append(names, k)
	}
	sort.Strings(names)
	return names
}

// providerNames is the cobra completion callback for a provider-name argument.
func providerNames(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	cfg, err := config.Load()
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	return sortedNames(cfg), cobra.ShellCompDirectiveNoFileComp
}
