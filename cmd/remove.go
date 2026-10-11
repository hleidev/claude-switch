package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/hleidev/claude-switch/internal/config"
	"github.com/hleidev/claude-switch/internal/secret"
)

var removeYes bool

var removeCmd = &cobra.Command{
	Use:               "remove <provider>",
	Aliases:           []string{"rm"},
	Short:             "Remove a provider",
	Args:              cobra.ExactArgs(1),
	ValidArgsFunction: providerNames,
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		if name == "claude" {
			return fmt.Errorf("claude is the built-in OAuth fallback and cannot be removed")
		}
		cfg, err := config.Load()
		if err != nil {
			return err
		}
		if _, ok := cfg.Providers[name]; !ok {
			return fmt.Errorf("provider %q not found", name)
		}
		if !removeYes {
			if !isInteractive() {
				return fmt.Errorf("refusing to remove %q without confirmation; pass --yes", name)
			}
			ok, err := confirm(fmt.Sprintf("Remove provider %q (its key will be deleted)?", name))
			if err != nil {
				return err
			}
			if !ok {
				fmt.Fprintln(cmd.ErrOrStderr(), "aborted")
				return nil
			}
		}
		cleanupKeychainOnRemove(cfg, name)
		delete(cfg.Providers, name)
		if cfg.DefaultProvider == name {
			cfg.DefaultProvider = "claude"
		}
		if err := config.Save(cfg); err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "✓ removed %s\n", name)
		return nil
	},
}

func cleanupKeychainOnRemove(cfg *config.Config, name string) {
	p, ok := cfg.Providers[name]
	if !ok {
		return
	}
	raw := p.AuthTokenRaw()
	if secret.DetectKind(raw) != secret.KindKeychain {
		return
	}
	service, account := secret.ParseKeychainRef(raw)
	if service != secret.DefaultService || account != name {
		return
	}
	for otherName, otherP := range cfg.Providers {
		if otherName == name {
			continue
		}
		if otherRaw := otherP.AuthTokenRaw(); otherRaw != "" {
			if oService, oAccount := secret.ParseKeychainRef(otherRaw); oService == service && oAccount == account {
				return
			}
		}
	}
	_ = secret.DefaultBackend.Delete(service, account)
}

func init() {
	removeCmd.Flags().BoolVar(&removeYes, "yes", false, "skip confirmation")
	rootCmd.AddCommand(removeCmd)
}
