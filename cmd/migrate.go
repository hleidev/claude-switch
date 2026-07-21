package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/hleidev/claude-switch/internal/config"
	"github.com/hleidev/claude-switch/internal/migrate"
	"github.com/hleidev/claude-switch/internal/presets"
)

// legacyHome returns the legacy ~/.claude-switch location, honoring the CS_HOME
// override the bash tool supported.
func legacyHome(home string) string {
	if h := os.Getenv("CS_HOME"); h != "" {
		return h
	}
	return filepath.Join(home, ".claude-switch")
}

var migrateCmd = &cobra.Command{
	Use:   "migrate",
	Short: "Import providers from the legacy ~/.claude-switch layout (schema v0 → current)",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		out := cmd.OutOrStdout()
		home, err := os.UserHomeDir()
		if err != nil {
			return err
		}
		oldDir := legacyHome(home)
		provDir := filepath.Join(oldDir, "providers")
		entries, err := os.ReadDir(provDir)
		if os.IsNotExist(err) {
			fmt.Fprintf(out, "Nothing to migrate: %s not found.\n", provDir)
			return nil
		}
		if err != nil {
			return err
		}

		cfg, err := config.Load()
		if err != nil {
			return err
		}

		// Back up any existing new-location config before we merge into it.
		newPath, _ := config.ConfigPath()
		if data, rerr := os.ReadFile(newPath); rerr == nil {
			_ = os.WriteFile(newPath+".pre-migrate.bak", data, 0o600)
			fmt.Fprintf(out, "  (backed up existing config to %s.pre-migrate.bak)\n", newPath)
		}

		imported := 0
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".zsh") {
				continue
			}
			name := strings.TrimSuffix(e.Name(), ".zsh")
			if name == "claude" {
				continue
			}
			data, err := os.ReadFile(filepath.Join(provDir, e.Name()))
			if err != nil {
				fmt.Fprintf(out, "⚠ skip %s: %v\n", e.Name(), err)
				continue
			}
			vars := migrate.ParseExports(string(data))
			base := config.Provider{}
			known := false
			if preset, ok := presets.Lookup(name); ok {
				base = config.Provider(preset)
				known = true
			}
			p := migrate.ToProvider(base, vars)
			cfg.Providers[name] = p
			imported++

			// Re-register the imported key so Claude Code doesn't re-prompt.
			if p.AuthToken() != "" {
				registerKeyBestEffort(cmd, p.AuthToken())
			}
			if known {
				fmt.Fprintf(out, "✓ imported %s (matched built-in preset)\n", name)
			} else {
				fmt.Fprintf(out, "✓ imported %s — best-effort, please verify\n", name)
			}
		}

		if imported == 0 {
			fmt.Fprintln(out, "No providers found to import.")
			return nil
		}

		// Carry over the old global default if it points at an imported provider.
		if def := readLegacyDefault(oldDir); def != "" {
			if _, ok := cfg.Providers[def]; ok || def == "claude" {
				cfg.DefaultProvider = def
				fmt.Fprintf(out, "✓ default provider set to %s (from old config)\n", def)
			}
		}

		if err := config.Save(cfg); err != nil {
			return err
		}
		fmt.Fprintf(out, "\n✓ migrated %d provider(s) → %s\n", imported, newPath)
		fmt.Fprintf(out, "  Old data left in place at %s (delete it yourself once verified).\n", oldDir)

		fixLegacyIntegration(cmd, home)
		return nil
	},
}

// readLegacyDefault reads default_provider from the legacy config.toml.
func readLegacyDefault(oldDir string) string {
	data, err := os.ReadFile(filepath.Join(oldDir, "config.toml"))
	if err != nil {
		return ""
	}
	return migrate.ParseLegacyDefault(string(data))
}

// fixLegacyIntegration detects the bash-era symlink and rc lines that would
// clash with the new `cs` function and, interactively, offers to remove them
// and install the new integration. Non-interactively it just prints guidance.
func fixLegacyIntegration(cmd *cobra.Command, home string) {
	out := cmd.OutOrStdout()
	oldBin := filepath.Join(home, ".local", "bin", "cs")
	_, binErr := os.Lstat(oldBin)
	hasOldBin := binErr == nil

	oldRCs := []string{}
	for _, rc := range []string{".zshrc", ".bashrc", ".bash_profile"} {
		path := filepath.Join(home, rc)
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		for _, line := range strings.Split(string(data), "\n") {
			if isLegacyActivation(line) {
				oldRCs = append(oldRCs, path)
				break
			}
		}
	}
	if !hasOldBin && len(oldRCs) == 0 {
		return
	}

	fmt.Fprintln(out, "\nLegacy shell integration detected:")
	if hasOldBin {
		fmt.Fprintf(out, "  • old binary/symlink: %s\n", oldBin)
	}
	for _, rc := range oldRCs {
		fmt.Fprintf(out, "  • old activation line in: %s\n", rc)
	}

	if !isInteractive() {
		fmt.Fprintln(out, "Remove these and run `cs setup` to install the new integration.")
		return
	}
	ok, err := confirm("Remove the legacy integration and install the new one now?")
	if err != nil || !ok {
		fmt.Fprintln(out, "Left as-is. Remove them and run `cs setup` when ready.")
		return
	}
	if hasOldBin {
		if err := os.Remove(oldBin); err != nil {
			fmt.Fprintf(out, "⚠ could not remove %s: %v\n", oldBin, err)
		} else {
			fmt.Fprintf(out, "✓ removed %s\n", oldBin)
		}
	}
	for _, rc := range oldRCs {
		if err := stripLegacyRCLines(rc); err != nil {
			fmt.Fprintf(out, "⚠ could not clean %s: %v\n", rc, err)
		} else {
			fmt.Fprintf(out, "✓ cleaned legacy lines from %s\n", rc)
		}
	}
	if rc, added, err := installIntegration(home); err != nil {
		fmt.Fprintf(out, "⚠ could not install new integration: %v\n", err)
	} else if added {
		fmt.Fprintf(out, "✓ installed new integration in %s — open a new terminal\n", rc)
	} else {
		fmt.Fprintf(out, "✓ new integration already present in %s\n", rc)
	}
}

// isLegacyActivation reports whether an rc line is the bash-era activation.
// Current and pre-0.2 lines are excluded first — both would match below.
func isLegacyActivation(line string) bool {
	if strings.Contains(line, setupMarker) || strings.Contains(line, legacySetupMarker) {
		return false
	}
	return strings.Contains(line, ".local/bin/cs") ||
		strings.Contains(line, "cs init zsh") ||
		strings.Contains(line, "cs init bash")
}

// stripLegacyRCLines removes the bash-era activation line(s) from an rc file.
func stripLegacyRCLines(rc string) error {
	data, err := os.ReadFile(rc)
	if err != nil {
		return err
	}
	var kept []string
	for _, line := range strings.Split(string(data), "\n") {
		if isLegacyActivation(line) {
			continue
		}
		kept = append(kept, line)
	}
	return os.WriteFile(rc, []byte(strings.Join(kept, "\n")), 0o644)
}

func init() {
	rootCmd.AddCommand(migrateCmd)
}
