package cmd

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestShellIntegrationSwitch(t *testing.T) {
	for _, shell := range []string{"bash", "zsh"} {
		t.Run(shell, func(t *testing.T) {
			shellPath, err := exec.LookPath(shell)
			if err != nil {
				t.Skipf("%s is not installed", shell)
			}
			dir := t.TempDir()
			stub := `#!/bin/sh
case "${3:-}" in
  fail) echo 'export ANTHROPIC_MODEL=partial'; exit 7 ;;
  ok)
    echo 'export ANTHROPIC_MODEL=new'
    echo 'export CLAUDE_SWITCH_PROVIDER=new'
    echo "export _CS_MANAGED_VARS='ANTHROPIC_MODEL CLAUDE_SWITCH_PROVIDER _CS_MANAGED_VARS'"
    ;;
esac
`
			if err := os.WriteFile(filepath.Join(dir, "cs"), []byte(stub), 0o700); err != nil {
				t.Fatal(err)
			}
			snippet, err := shellIntegration(shell)
			if err != nil {
				t.Fatal(err)
			}
			for _, tc := range []struct {
				provider string
				want     string
			}{
				{"fail", "7|old|old|old-only"},
				{"ok", "0|new|new|"},
				{"claude", "0|||"},
			} {
				t.Run(tc.provider, func(t *testing.T) {
					script := snippet + `
export ANTHROPIC_MODEL=old CLAUDE_SWITCH_PROVIDER=old OLD_ONLY=old-only
export _CS_MANAGED_VARS='ANTHROPIC_MODEL CLAUDE_SWITCH_PROVIDER OLD_ONLY _CS_MANAGED_VARS'
cs use ` + tc.provider + `
result=$?
printf '%s|%s|%s|%s\n' "$result" "${ANTHROPIC_MODEL:-}" "${CLAUDE_SWITCH_PROVIDER:-}" "${OLD_ONLY:-}"
`
					command := exec.Command(shellPath, "-c", script)
					command.Env = append(os.Environ(), "PATH="+dir+":"+os.Getenv("PATH"))
					output, err := command.CombinedOutput()
					if err != nil {
						t.Fatalf("shell: %v\n%s", err, output)
					}
					if got := strings.TrimSpace(string(output)); got != tc.want {
						t.Fatalf("switch result = %q, want %q", got, tc.want)
					}
				})
			}
		})
	}
}
