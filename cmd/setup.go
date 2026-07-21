package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/spf13/cobra"
)

const setupMarker = "command cs init"

// legacySetupMarker is the pre-0.2 line; it must be rewritten, not skipped.
const legacySetupMarker = "command claude-switch init"

// rcFileFor returns the rc file claude-switch should append integration to.
func rcFileFor(shell, home string) string {
	switch shell {
	case "bash":
		// macOS login shells read ~/.bash_profile, not ~/.bashrc.
		if runtime.GOOS == "darwin" {
			return filepath.Join(home, ".bash_profile")
		}
		return filepath.Join(home, ".bashrc")
	default:
		return filepath.Join(home, ".zshrc")
	}
}

// installIntegration idempotently adds the `eval "$(claude-switch init <shell>)"`
// line to the correct rc file. Returns the rc path and whether it added a line.
func installIntegration(home string) (rc string, added bool, err error) {
	shell := detectShell()
	if shell != "zsh" && shell != "bash" {
		return "", false, fmt.Errorf("unsupported shell %q (supported: zsh, bash); add `eval \"$(command cs init zsh)\"` to your rc file manually", shell)
	}
	rc = rcFileFor(shell, home)

	data, rerr := os.ReadFile(rc)
	switch {
	case rerr != nil && !os.IsNotExist(rerr):
		return rc, false, rerr
	case rerr == nil && strings.Contains(string(data), setupMarker):
		return rc, false, nil
	case rerr == nil && strings.Contains(string(data), legacySetupMarker):
		return rc, true, rewriteLegacyLine(rc, string(data), shell)
	}

	line := fmt.Sprintf("\n# claude-switch\neval \"$(command cs init %s)\"\n", shell)
	f, err := os.OpenFile(rc, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return rc, false, err
	}
	defer f.Close()
	if _, err := f.WriteString(line); err != nil {
		return rc, false, err
	}
	return rc, true, nil
}

// rewriteLegacyLine swaps the pre-0.2 activation line for the current one.
func rewriteLegacyLine(rc, data, shell string) error {
	lines := strings.Split(data, "\n")
	for i, line := range lines {
		if strings.Contains(line, legacySetupMarker) {
			lines[i] = fmt.Sprintf("eval \"$(command cs init %s)\"", shell)
		}
	}
	return os.WriteFile(rc, []byte(strings.Join(lines, "\n")), 0o644)
}

var setupCmd = &cobra.Command{
	Use:   "setup",
	Short: "Install shell integration into your rc file (run this once)",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		out := cmd.OutOrStdout()
		home, err := os.UserHomeDir()
		if err != nil {
			return err
		}
		rc, added, err := installIntegration(home)
		if err != nil {
			return err
		}
		if !added {
			fmt.Fprintf(out, "✓ already installed in %s\n", rc)
			return nil
		}
		fmt.Fprintf(out, "✓ shell integration installed in %s\n", rc)
		fmt.Fprintf(out, "  open a new terminal, or run: source %s\n", rc)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(setupCmd)
}
