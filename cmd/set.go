package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/hleidev/claude-switch/internal/claudejson"
	"github.com/hleidev/claude-switch/internal/config"
)

var setCmd = &cobra.Command{
	Use:   "set <provider> <VAR> [value]",
	Short: "Set a single provider variable",
	Long: "Set a provider variable by its real environment variable name\n" +
		"(e.g. `cs set glm ANTHROPIC_MODEL glm-4.7`). Use `cs set <provider> key`\n" +
		"(no value) to set the API key via hidden input.",
	Args:              cobra.RangeArgs(2, 3),
	ValidArgsFunction: providerNames,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return err
		}
		provider, field := args[0], args[1]
		if _, ok := cfg.Providers[provider]; !ok {
			return fmt.Errorf("provider %q not found", provider)
		}

		// Secret fields must never take a value on the command line.
		if field == "key" || field == "auth_token" {
			if len(args) == 3 {
				return fmt.Errorf("refusing plaintext secret on command line; run `cs set %s key` and enter it at the hidden prompt", provider)
			}
			if !isInteractive() {
				return fmt.Errorf("setting a key requires an interactive terminal")
			}
			secret, err := readSecret(fmt.Sprintf("Paste API key for %s", provider))
			if err != nil {
				return err
			}
			if secret == "" {
				return fmt.Errorf("no key entered")
			}
			if err := cfg.SetField(provider, "auth_token", secret); err != nil {
				return err
			}
			if err := config.Save(cfg); err != nil {
				return err
			}
			registerKeyBestEffort(cmd, secret)
			fmt.Fprintf(cmd.OutOrStdout(), "✓ updated %s key\n", provider)
			return nil
		}

		if len(args) != 3 {
			return fmt.Errorf("field %q requires a value", field)
		}
		if err := cfg.SetField(provider, field, args[2]); err != nil {
			return err
		}
		if err := config.Save(cfg); err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "✓ %s.%s = %s\n", provider, field, args[2])
		return nil
	},
}

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

func init() {
	rootCmd.AddCommand(setCmd)
}
