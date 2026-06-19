package cmd

import (
	"fmt"
	"os"
	"os/exec"

	toml "github.com/pelletier/go-toml/v2"
	"github.com/spf13/cobra"

	"github.com/hleidev/claude-switch/internal/config"
)

var editCmd = &cobra.Command{
	Use:               "edit [provider]",
	Short:             "Open config.toml in $EDITOR (whole file)",
	Args:              cobra.MaximumNArgs(1),
	ValidArgsFunction: providerNames,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return err
		}
		if len(args) == 1 {
			name := args[0]
			if name == "claude" {
				return fmt.Errorf("claude is the built-in OAuth fallback; nothing to edit")
			}
			if _, ok := cfg.Providers[name]; !ok {
				return fmt.Errorf("provider %q not found", name)
			}
		}

		path, err := config.ConfigPath()
		if err != nil {
			return err
		}

		// Create the file only if it doesn't exist yet; never rewrite (and thus
		// normalize/strip comments from) a file the user may have hand-authored.
		original, err := os.ReadFile(path)
		if os.IsNotExist(err) {
			if err := config.Save(cfg); err != nil {
				return err
			}
			original, err = os.ReadFile(path)
		}
		if err != nil {
			return err
		}

		editor := os.Getenv("EDITOR")
		if editor == "" {
			editor = "vi"
		}
		ed := exec.Command(editor, path)
		ed.Stdin, ed.Stdout, ed.Stderr = os.Stdin, os.Stdout, os.Stderr
		if err := ed.Run(); err != nil {
			return fmt.Errorf("editor exited with error: %w", err)
		}

		// Validate the result. On a missing/unreadable file or parse failure,
		// restore the original so a broken config can't poison every subsequent
		// command.
		data, err := os.ReadFile(path)
		if err != nil {
			if writeErr := os.WriteFile(path, original, 0o600); writeErr != nil {
				return fmt.Errorf("config unreadable after editing (%v) and the original could not be restored: %w", err, writeErr)
			}
			return fmt.Errorf("config was removed or unreadable after editing; the previous config was restored: %w", err)
		}
		var check config.Config
		if err := toml.Unmarshal(data, &check); err != nil {
			if writeErr := os.WriteFile(path, original, 0o600); writeErr != nil {
				return fmt.Errorf("config has invalid TOML and the original could not be restored (%v): %w", writeErr, err)
			}
			return fmt.Errorf("config has invalid TOML; your edits were discarded and the previous config restored: %w", err)
		}
		fmt.Fprintln(cmd.OutOrStdout(), "✓ config saved")
		return nil
	},
}

func init() {
	rootCmd.AddCommand(editCmd)
}
