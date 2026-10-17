package cmd

import (
	"fmt"
	"sort"

	"github.com/spf13/cobra"

	"github.com/hleidev/claude-switch/internal/claudejson"
	"github.com/hleidev/claude-switch/internal/config"
	"github.com/hleidev/claude-switch/internal/presets"
	"github.com/hleidev/claude-switch/internal/secret"
)

// registerKeyBestEffort resolves a key reference and registers the real key's
// trailing suffix into ~/.claude.json, warning (not failing) on error. It
// returns whether the file was actually updated. If the key reference cannot
// be resolved, it does NOT register the raw reference string.
func registerKeyBestEffort(cmd *cobra.Command, key string) bool {
	realKey, err := secret.Resolve(key)
	if err != nil || realKey == "" {
		return false
	}
	path, err := claudejson.DefaultPath()
	if err != nil {
		return false
	}
	registered, err := claudejson.RegisterKey(path, realKey)
	if err != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "⚠ could not register key in ~/.claude.json: %v\n", err)
		return false
	}
	return registered
}

// registerResolvedKeySilent ensures an already-resolved secret key suffix is
// approved in ~/.claude.json without extra secret resolution or noisy output.
func registerResolvedKeySilent(realKey string) {
	if realKey == "" {
		return
	}
	path, err := claudejson.DefaultPath()
	if err != nil {
		return
	}
	_, _ = claudejson.RegisterKey(path, realKey)
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
	// These commands all take at most one provider; stop offering after it.
	if err != nil || len(args) > 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	return sortedNames(cfg), cobra.ShellCompDirectiveNoFileComp
}

// providerNamesWithClaude adds the built-in "claude" for commands that accept it.
func providerNamesWithClaude(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	names, directive := providerNames(cmd, args, toComplete)
	if names == nil {
		return nil, directive
	}
	return append(names, "claude"), directive
}
