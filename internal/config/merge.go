package config

import (
	"fmt"
	"sort"
)

// EnvVar is a single environment-variable assignment.
type EnvVar struct {
	Key   string
	Value string
}

// orderedTypedFields maps typed provider fields to their Claude Code env vars,
// in deterministic output order.
var orderedTypedFields = []struct {
	env string
	get func(Provider) string
}{
	{"ANTHROPIC_BASE_URL", func(p Provider) string { return p.BaseURL }},
	{"ANTHROPIC_AUTH_TOKEN", func(p Provider) string { return p.AuthToken }},
	{"ANTHROPIC_MODEL", func(p Provider) string { return p.Model }},
	{"ANTHROPIC_SMALL_FAST_MODEL", func(p Provider) string { return p.SmallFastModel }},
	{"ANTHROPIC_DEFAULT_SONNET_MODEL", func(p Provider) string { return p.SonnetModel }},
	{"ANTHROPIC_DEFAULT_OPUS_MODEL", func(p Provider) string { return p.OpusModel }},
	{"ANTHROPIC_DEFAULT_HAIKU_MODEL", func(p Provider) string { return p.HaikuModel }},
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// BuildEnv computes the ordered environment assignments for a provider, applying
// merge precedence: [defaults].env -> typed fields -> [providers.X.env]. Empty
// typed fields produce no variable. CLAUDE_SWITCH_PROVIDER is always appended
// last. The returned slice is deterministic.
func (c *Config) BuildEnv(name string) ([]EnvVar, error) {
	p, ok := c.Providers[name]
	if !ok {
		return nil, fmt.Errorf("provider %q not found", name)
	}

	values := map[string]string{}
	var order []string
	put := func(k, v string) {
		if _, seen := values[k]; !seen {
			order = append(order, k)
		}
		values[k] = v
	}

	for _, k := range sortedKeys(c.Defaults.Env) {
		put(k, c.Defaults.Env[k])
	}
	for _, f := range orderedTypedFields {
		if v := f.get(p); v != "" {
			put(f.env, v)
		}
	}
	for _, k := range sortedKeys(p.Env) {
		put(k, p.Env[k])
	}
	put("CLAUDE_SWITCH_PROVIDER", name)

	out := make([]EnvVar, 0, len(order))
	for _, k := range order {
		out = append(out, EnvVar{Key: k, Value: values[k]})
	}
	return out, nil
}
