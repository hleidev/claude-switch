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
// switch can clear them.
func ForProvider(c *config.Config, name string, preset map[string]string) (string, error) {
	code, _, err := ForProviderWithToken(c, name, preset)
	return code, err
}

// ForProviderWithToken is like ForProvider, and also returns the resolved
// ANTHROPIC_AUTH_TOKEN value (if any) so callers do not need to re-resolve
// secrets a second time.
func ForProviderWithToken(c *config.Config, name string, preset map[string]string) (code, authToken string, err error) {
	vars, err := c.BuildEnv(name, preset)
	if err != nil {
		return "", "", err
	}
	var b strings.Builder
	names := make([]string, 0, len(vars)+1)
	for _, kv := range vars {
		// Defense in depth: never emit an unsafe variable name into eval'd code,
		// even if a bad key slipped past write-time validation.
		if !config.ValidEnvKey(kv.Key) {
			continue
		}
		if kv.Key == config.AuthTokenKey {
			authToken = kv.Value
		}
		b.WriteString("export " + kv.Key + "=" + shellQuote(kv.Value) + "\n")
		names = append(names, kv.Key)
	}
	b.WriteString("export ENABLE_CLAUDEAI_MCP_SERVERS=false\n")
	names = append(names, "ENABLE_CLAUDEAI_MCP_SERVERS")
	names = append(names, managedVar)
	b.WriteString("export " + managedVar + "=" + shellQuote(strings.Join(names, " ")) + "\n")
	return b.String(), authToken, nil
}
