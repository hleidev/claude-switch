// Package shellenv generates eval-able POSIX shell code that injects (or clears)
// a provider's environment into the calling shell. It is the only output that
// reaches stdout for the hidden __shellenv command; everything human-readable
// goes to stderr in the command layer.
package shellenv

import (
	"strings"

	"github.com/hleidev/claude-switch/internal/config"
)

const managedVar = "_CS_MANAGED_VARS"

// shellQuote single-quotes a value for POSIX sh, escaping embedded single quotes.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// ForProvider returns eval-able shell code that exports the provider's
// environment and records the variable names in _CS_MANAGED_VARS so the next
// switch can clear them. The preset (built-in template, may be nil) is supplied
// by the caller and merged under the provider's own entries.
//
// Clearing previously-managed variables is intentionally NOT done here: it must
// happen in the shell function (see cmd/init.go), because zsh does not
// word-split an unquoted `$_CS_MANAGED_VARS` in a `for` loop, so a reset loop
// embedded in this eval'd payload would silently no-op under zsh.
func ForProvider(c *config.Config, name string, preset map[string]string) (string, error) {
	vars, err := c.BuildEnv(name, preset)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	names := make([]string, 0, len(vars)+1)
	for _, kv := range vars {
		// Defense in depth: never emit an unsafe variable name into eval'd code,
		// even if a bad key slipped past write-time validation.
		if !config.ValidEnvKey(kv.Key) {
			continue
		}
		b.WriteString("export " + kv.Key + "=" + shellQuote(kv.Value) + "\n")
		names = append(names, kv.Key)
	}
	names = append(names, managedVar)
	b.WriteString("export " + managedVar + "=" + shellQuote(strings.Join(names, " ")) + "\n")
	return b.String(), nil
}
