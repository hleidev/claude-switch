package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

// detectShell guesses the current shell from environment hints.
func detectShell() string {
	if os.Getenv("ZSH_VERSION") != "" {
		return "zsh"
	}
	if os.Getenv("BASH_VERSION") != "" {
		return "bash"
	}
	if s := os.Getenv("SHELL"); s != "" {
		return filepath.Base(s)
	}
	return "zsh"
}

// shellFragments holds the per-shell pieces injected into integrationTemplate.
type shellFragments struct {
	// split expands _CS_MANAGED_VARS into separate words. zsh does NOT word-split
	// an unquoted parameter expansion, so it needs the ${=VAR} flag; bash splits
	// $VAR natively. This is why the unset loop lives in the shell function and
	// not in the eval'd __shellenv payload.
	split      string
	completion string
}

// shellIntegration returns the source-able snippet for the given shell: an
// unset helper, the `cs` function, new-terminal auto-load, and completion bound
// to `cs`.
func shellIntegration(shell string) (string, error) {
	var f shellFragments
	switch shell {
	case "zsh":
		f.split = "${=_CS_MANAGED_VARS}"
		f.completion = `if command -v compdef >/dev/null 2>&1; then
  source <(command claude-switch completion zsh) 2>/dev/null || true
  compdef _claude-switch cs 2>/dev/null || true
fi`
	case "bash":
		f.split = "$_CS_MANAGED_VARS"
		f.completion = `if type complete >/dev/null 2>&1; then
  source <(command claude-switch completion bash) 2>/dev/null || true
  complete -F __start_claude-switch cs 2>/dev/null || true
fi`
	default:
		return "", fmt.Errorf("unsupported shell %q (supported: zsh, bash)", shell)
	}
	return fmt.Sprintf(integrationTemplate, f.split, f.completion), nil
}

// integrationTemplate is the shared snippet. The first %s is the per-shell
// word-split expression; the second is the completion fragment. Bare `cs use`
// (no provider) reports usage instead of silently switching to the default.
const integrationTemplate = `# >>> claude-switch >>>
_cs_unset_managed() {
  [ -n "${_CS_MANAGED_VARS:-}" ] || return 0
  for v in %s; do unset "$v" 2>/dev/null; done
  return 0
}
cs() {
  if [ "${1:-}" = "use" ]; then
    shift
    if [ -z "${1:-}" ]; then
      echo "usage: cs use <provider|claude>" >&2
      return 2
    fi
    _cs_unset_managed
    eval "$(command claude-switch __shellenv --announce "$@")"
  else
    command claude-switch "$@"
  fi
}
# auto-load the default provider in new shells (must never break startup)
_cs_unset_managed
eval "$(command claude-switch __shellenv 2>/dev/null)" 2>/dev/null || true
# completion (bound to the cs function as well as the binary)
%s
# <<< claude-switch <<<
`

var initCmd = &cobra.Command{
	Use:       "init [zsh|bash]",
	Short:     "Print the integration snippet `claude-switch setup` installs (for manual setup)",
	Args:      cobra.MaximumNArgs(1),
	ValidArgs: []string{"zsh", "bash"},
	RunE: func(cmd *cobra.Command, args []string) error {
		shell := detectShell()
		if len(args) == 1 {
			shell = strings.ToLower(args[0])
		}
		snippet, err := shellIntegration(shell)
		if err != nil {
			return err
		}
		fmt.Fprint(cmd.OutOrStdout(), snippet)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(initCmd)
}
