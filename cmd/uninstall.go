package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/hleidev/claude-switch/internal/config"
)

var uninstallCmd = &cobra.Command{
	Use:   "uninstall",
	Short: "Remove shell integration (optionally delete config)",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		out := cmd.OutOrStdout()
		home, err := os.UserHomeDir()
		if err != nil {
			return err
		}
		shell := detectShell()
		rc := rcFileFor(shell, home)

		removed, err := stripIntegration(rc)
		if err != nil {
			return err
		}
		if removed {
			fmt.Fprintf(out, "✓ removed integration from %s\n", rc)
		} else {
			fmt.Fprintf(out, "  no integration line found in %s\n", rc)
		}

		path, _ := config.ConfigPath()
		if _, err := os.Stat(path); err == nil {
			if isInteractive() {
				ok, err := confirm(fmt.Sprintf("Delete %s (contains your API keys)?", path))
				if err != nil {
					return err
				}
				if ok {
					if err := os.Remove(path); err != nil {
						return err
					}
					fmt.Fprintf(out, "✓ deleted %s\n", path)
				} else {
					fmt.Fprintf(out, "  kept %s\n", path)
				}
			} else {
				fmt.Fprintf(out, "  config kept at %s (delete manually if desired)\n", path)
			}
		}
		fmt.Fprintln(out, "Open a new terminal to finish.")
		return nil
	},
}

// stripIntegration removes claude-switch lines (both the marker line and the
// init.go-delimited block) from an rc file. Returns whether anything changed.
func stripIntegration(rc string) (bool, error) {
	f, err := os.Open(rc)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	var kept []string
	changed := false
	inBlock := false
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		switch {
		case strings.Contains(line, ">>> claude-switch >>>"):
			inBlock, changed = true, true
			continue
		case strings.Contains(line, "<<< claude-switch <<<"):
			inBlock = false
			continue
		case inBlock:
			continue
		case strings.Contains(line, setupMarker), strings.Contains(line, legacySetupMarker), line == "# claude-switch":
			changed = true
			continue
		}
		kept = append(kept, line)
	}
	f.Close()
	if err := sc.Err(); err != nil {
		return false, err
	}
	if !changed {
		return false, nil
	}
	out := strings.Join(kept, "\n")
	if len(kept) > 0 {
		out += "\n"
	}
	return true, os.WriteFile(rc, []byte(out), 0o644)
}

func init() {
	rootCmd.AddCommand(uninstallCmd)
}
