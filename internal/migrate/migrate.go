// Package migrate parses the legacy ~/.claude-switch bash-era provider files so
// `cs migrate` can import them into the new config.
package migrate

import (
	"regexp"
	"strings"

	"github.com/hleidev/claude-switch/internal/config"
)

// refPattern matches ${NAME} and $NAME shell variable references.
var refPattern = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)\}|\$([A-Za-z_][A-Za-z0-9_]*)`)

// refName returns the variable name from a ${NAME}/$NAME submatch.
func refName(m []string) string {
	if m[1] != "" {
		return m[1]
	}
	return m[2]
}

// refNames lists the variable names referenced in s.
func refNames(s string) []string {
	var names []string
	for _, m := range refPattern.FindAllStringSubmatch(s, -1) {
		names = append(names, refName(m))
	}
	return names
}

// expand resolves ${NAME}/$NAME references in s using vars (legacy provider
// files commonly set ANTHROPIC_AUTH_TOKEN="${SOME_API_KEY}"). Unknown
// references are left intact. Bounded to avoid reference loops.
func expand(s string, vars map[string]string) string {
	for i := 0; i < 10; i++ {
		next := refPattern.ReplaceAllStringFunc(s, func(tok string) string {
			name := refName(refPattern.FindStringSubmatch(tok))
			if v, ok := vars[name]; ok {
				return v
			}
			return tok
		})
		if next == s {
			break
		}
		s = next
	}
	return s
}

// ToProvider overlays parsed export vars onto a base provider (which may be a
// preset). Each var is a real environment variable name, so it maps directly
// into the flat provider map. Unsafe key names are skipped. Variables referenced
// by another var's value (e.g. SOME_API_KEY in
// ANTHROPIC_AUTH_TOKEN="${SOME_API_KEY}") are pure holders: their reference is
// resolved into the referring var, but the holder itself is not carried over —
// that would leave the secret duplicated under a junk name.
func ToProvider(base config.Provider, vars map[string]string) config.Provider {
	holders := map[string]bool{}
	for _, raw := range vars {
		for _, ref := range refNames(raw) {
			holders[ref] = true
		}
	}

	p := config.Provider{}
	for k, v := range base {
		p[k] = v
	}
	for k, raw := range vars {
		if holders[k] || !config.ValidEnvKey(k) {
			continue
		}
		p[k] = expand(raw, vars)
	}
	return p
}

// ParseExports extracts KEY=VALUE pairs from `export KEY=VALUE` shell lines,
// stripping surrounding single or double quotes. Comments and blank lines are
// ignored. This is best-effort, matching the spec's "尽力导入" guarantee.
func ParseExports(content string) map[string]string {
	out := map[string]string{}
	for _, raw := range strings.Split(content, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		out[key] = parseValue(strings.TrimLeft(val, " \t"))
	}
	return out
}

// parseValue extracts a shell-style value: a quoted string yields its contents
// (trailing text such as an inline comment is ignored); an unquoted value ends
// at the first whitespace, which also drops a trailing ` # comment`.
func parseValue(s string) string {
	if s == "" {
		return ""
	}
	if q := s[0]; q == '"' || q == '\'' {
		if i := strings.IndexByte(s[1:], q); i >= 0 {
			return s[1 : 1+i]
		}
		return s[1:] // unterminated quote: best effort
	}
	if i := strings.IndexAny(s, " \t"); i >= 0 {
		return s[:i]
	}
	return s
}

// ParseLegacyDefault extracts the `default_provider = "x"` value from a legacy
// ~/.claude-switch/config.toml (it may live under a [core] table). Returns ""
// if absent.
func ParseLegacyDefault(content string) string {
	for _, raw := range strings.Split(content, "\n") {
		line := strings.TrimSpace(raw)
		if strings.HasPrefix(line, "#") {
			continue
		}
		rest, ok := strings.CutPrefix(line, "default_provider")
		if !ok {
			continue
		}
		val, ok := strings.CutPrefix(strings.TrimSpace(rest), "=")
		if !ok {
			continue
		}
		return parseValue(strings.TrimLeft(val, " \t"))
	}
	return ""
}
