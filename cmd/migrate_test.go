package cmd

import "testing"

func TestIsLegacyActivation(t *testing.T) {
	legacy := []string{
		`[[ -x "${HOME}/.local/bin/cs" ]] && eval "$("${HOME}/.local/bin/cs" init zsh)"`,
		`command -v cs &>/dev/null && eval "$(cs init zsh)"`,
		`eval "$(cs init bash)"`,
	}
	for _, l := range legacy {
		if !isLegacyActivation(l) {
			t.Errorf("expected legacy: %q", l)
		}
	}
	keep := []string{
		`eval "$(command claude-switch init zsh)"`, // the new line
		`export PATH="${HOME}/.local/bin:${PATH}"`, // PATH export must survive
		`# Claude Code provider switcher (cs)`,     // a comment
		`alias g=git`,
	}
	for _, l := range keep {
		if isLegacyActivation(l) {
			t.Errorf("should NOT be treated as legacy: %q", l)
		}
	}
}
