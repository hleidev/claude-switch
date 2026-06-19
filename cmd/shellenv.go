package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/hleidev/claude-switch/internal/config"
	"github.com/hleidev/claude-switch/internal/shellenv"
)

var shellenvAnnounce bool

// shellenvCmd is the hidden engine behind `cs use` and new-terminal auto-load.
// Its stdout is ONLY eval-able shell code; all human text goes to stderr.
var shellenvCmd = &cobra.Command{
	Use:    "__shellenv [provider]",
	Hidden: true,
	Args:   cobra.MaximumNArgs(1),
	// Disable usage/error noise: this output is eval'd by the shell.
	SilenceUsage:  true,
	SilenceErrors: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		out := cmd.OutOrStdout()
		errOut := cmd.ErrOrStderr()

		cfg, err := config.Load()
		if err != nil {
			// Startup safety: never emit stdout on error.
			fmt.Fprintln(errOut, "claude-switch: config error:", err)
			return nil
		}

		name := cfg.DefaultProvider
		if len(args) == 1 {
			name = args[0]
		}

		// Reset state: explicit "claude" or empty/default with no provider.
		// The shell function already cleared managed vars before calling us, so
		// there is nothing to emit on stdout here.
		if name == "" || name == "claude" {
			if shellenvAnnounce {
				fmt.Fprintln(errOut, "已切换到 claude (OAuth)")
			}
			return nil
		}

		p, ok := cfg.Providers[name]
		if !ok {
			if shellenvAnnounce {
				return fmt.Errorf("provider %q not found", name)
			}
			// Silent during startup; emit no stdout so nothing is eval'd.
			return nil
		}

		code, err := shellenv.ForProvider(cfg, name)
		if err != nil {
			if shellenvAnnounce {
				return err
			}
			return nil
		}
		fmt.Fprint(out, code)
		if shellenvAnnounce {
			if p.AuthToken == "" {
				fmt.Fprintf(errOut, "⚠ %s 未设置 auth_token；claude 将回落到 OAuth（运行：cs set %s key）\n", name, name)
			}
			fmt.Fprintf(errOut, "✓ 已切换到 %s\n", name)
		}
		return nil
	},
}

func init() {
	shellenvCmd.Flags().BoolVar(&shellenvAnnounce, "announce", false, "print human-readable status to stderr")
	rootCmd.AddCommand(shellenvCmd)
}
