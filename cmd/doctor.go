package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/sys/unix"

	"github.com/hleidev/claude-switch/internal/bridge"
	"github.com/hleidev/claude-switch/internal/claudejson"
	"github.com/hleidev/claude-switch/internal/config"
	"github.com/hleidev/claude-switch/internal/secret"
)

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Diagnose the claude-switch setup",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		out := cmd.OutOrStdout()
		ok := func(b bool) string {
			if b {
				return "✓"
			}
			return "✗"
		}

		cfg, cfgErr := config.Load()
		fmt.Fprintf(out, "%s config parses\n", ok(cfgErr == nil))
		if cfgErr != nil {
			fmt.Fprintf(out, "    %v\n", cfgErr)
			return nil
		}

		integrated := os.Getenv("_CS_SHELL") != ""
		fmt.Fprintf(out, "%s shell integration active in this terminal\n", ok(integrated))
		if !integrated {
			if rc, present := rcHasIntegration(); present {
				fmt.Fprintf(out, "    installed in %s — open a new terminal, or run: source %s\n", rc, rc)
			} else {
				fmt.Fprintln(out, "    run `cs setup`, then open a new terminal")
			}
		}

		defOK := cfg.DefaultProvider == "claude"
		if !defOK {
			_, defOK = cfg.Providers[cfg.DefaultProvider]
		}
		fmt.Fprintf(out, "%s default provider %q is valid\n", ok(defOK), cfg.DefaultProvider)

		for _, name := range sortedNames(cfg) {
			p := cfg.Providers[name]
			raw := p.AuthTokenRaw()
			kind := secret.DetectKind(raw)
			keyVal := p.AuthToken()
			switch kind {
			case secret.KindKeychain:
				fmt.Fprintf(out, "%s provider %q has key in macOS Keychain\n", ok(keyVal != ""), name)
			case secret.KindEnv:
				fmt.Fprintf(out, "%s provider %q has key via environment (%s)\n", ok(keyVal != ""), name, raw)
			case secret.KindPlaintext:
				fmt.Fprintf(out, "%s provider %q has a key (in config.toml)\n", ok(keyVal != ""), name)
			default:
				fmt.Fprintf(out, "%s provider %q has a key\n", ok(keyVal != ""), name)
			}
			fmt.Fprintf(out, "%s provider %q resolves a base_url\n", ok(resolvedBaseURL(cfg, name) != ""), name)
		}

		hasOpenAI := false
		for _, name := range sortedNames(cfg) {
			if cfg.Providers[name].Protocol() == config.ProtocolOpenAI {
				hasOpenAI = true
				break
			}
		}
		if hasOpenAI {
			_, uvErr := exec.LookPath("uv")
			fmt.Fprintf(out, "%s uv installed (required for OpenAI bridge)\n", ok(uvErr == nil))
			running, pid, _ := bridge.Status(bridge.DefaultPort)
			if running {
				fmt.Fprintf(out, "%s OpenAI bridge running on :%d (pid %d)\n", ok(true), bridge.DefaultPort, pid)
			} else {
				fmt.Fprintf(out, "… OpenAI bridge not running on :%d (auto-starts on `cs use`)\n", bridge.DefaultPort)
			}
		}

		if path, err := claudejson.DefaultPath(); err == nil {
			_, statErr := os.Stat(path)
			switch {
			case statErr == nil:
				fmt.Fprintf(out, "%s ~/.claude.json present and writable\n", ok(isWritable(path)))
			case os.IsNotExist(statErr):
				fmt.Fprintf(out, "%s ~/.claude.json not found (Claude Code creates it on first run)\n", ok(true))
			default:
				fmt.Fprintf(out, "%s ~/.claude.json: %v\n", ok(false), statErr)
			}
		}

		fmt.Fprintln(out, "\nNote: injected env only reaches interactive shells that source your rc file.")
		fmt.Fprintln(out, "IDE-spawned shells (VSCode/JetBrains) and Claude Code's own Bash tool do not")
		fmt.Fprintln(out, "source rc files, so switching has no effect there — this is inherent to env injection.")
		return nil
	},
}

// rcHasIntegration picks the remedy: never installed vs. installed but unloaded.
func rcHasIntegration() (rc string, present bool) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", false
	}
	rc = rcFileFor(detectShell(), home)
	data, err := os.ReadFile(rc)
	if err != nil {
		return rc, false
	}
	return rc, strings.Contains(string(data), setupMarker)
}

// isWritable checks write permission without opening the user's data file for
// writing (which could touch timestamps or contend with Claude Code).
func isWritable(path string) bool {
	return unix.Access(path, unix.W_OK) == nil
}

func init() {
	rootCmd.AddCommand(doctorCmd)
}
