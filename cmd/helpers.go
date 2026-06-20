package cmd

import (
	"fmt"
	"sort"

	"github.com/spf13/cobra"

	"github.com/hleidev/claude-switch/internal/claudejson"
	"github.com/hleidev/claude-switch/internal/config"
	"github.com/hleidev/claude-switch/internal/presets"
)

// registerKeyBestEffort registers a key into ~/.claude.json, warning (not
// failing) on error. It returns whether the file was actually updated.
func registerKeyBestEffort(cmd *cobra.Command, key string) bool {
	path, err := claudejson.DefaultPath()
	if err != nil {
		return false
	}
	registered, err := claudejson.RegisterKey(path, key)
	if err != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "⚠ could not register key in ~/.claude.json: %v\n", err)
		return false
	}
	return registered
}

// resolvedBaseURL returns the effective ANTHROPIC_BASE_URL for a provider,
// preferring the user's own value, then the built-in preset.
func resolvedBaseURL(cfg *config.Config, name string) string {
	if p, ok := cfg.Providers[name]; ok {
		if b := p.BaseURL(); b != "" {
			return b
		}
	}
	if preset, ok := presets.Lookup(name); ok {
		return preset[config.BaseURLKey]
	}
	return ""
}

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
