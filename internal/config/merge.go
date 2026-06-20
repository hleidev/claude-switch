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

// BuildEnv computes the ordered environment assignments for a provider by
// merging three flat layers, each overriding the previous on a per-key basis:
//
//	[defaults]  →  preset (built-in template, may be nil)  →  the provider's own entries
//
// A key whose final value is empty is treated as "not exported" (so an override
// can blank out an inherited variable). Unsafe variable names are skipped as
// defense in depth. Output is sorted for determinism, with ProviderVar appended
// last. The preset is injected by the caller (cmd layer) so this package stays
// decoupled from the embedded presets and is easy to test in isolation.
func (c *Config) BuildEnv(name string, preset map[string]string) ([]EnvVar, error) {
	p, ok := c.Providers[name]
	if !ok {
		return nil, fmt.Errorf("provider %q not found", name)
	}

	merged := map[string]string{}
	for k, v := range c.Defaults {
		merged[k] = v
	}
	for k, v := range preset {
		merged[k] = v
	}
	for k, v := range p {
		merged[k] = v
	}

	keys := make([]string, 0, len(merged))
	for k, v := range merged {
		if k == ProviderVar || v == "" || !ValidEnvKey(k) {
			continue
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)

	out := make([]EnvVar, 0, len(keys)+1)
	for _, k := range keys {
		out = append(out, EnvVar{Key: k, Value: merged[k]})
	}
	out = append(out, EnvVar{Key: ProviderVar, Value: name})
	return out, nil
}
