package config

import (
	"fmt"
	"sort"
	"strings"
)

// typedSetters maps a bare field path to a setter on *Provider.
var typedSetters = map[string]func(*Provider, string){
	"base_url":         func(p *Provider, v string) { p.BaseURL = v },
	"auth_token":       func(p *Provider, v string) { p.AuthToken = v },
	"model":            func(p *Provider, v string) { p.Model = v },
	"small_fast_model": func(p *Provider, v string) { p.SmallFastModel = v },
	"sonnet_model":     func(p *Provider, v string) { p.SonnetModel = v },
	"opus_model":       func(p *Provider, v string) { p.OpusModel = v },
	"haiku_model":      func(p *Provider, v string) { p.HaikuModel = v },
}

func typedFieldNames() string {
	names := make([]string, 0, len(typedSetters))
	for k := range typedSetters {
		names = append(names, k)
	}
	sort.Strings(names)
	return strings.Join(names, "/")
}

// SetField sets a single provider field. Bare names address typed fields;
// "env.<KEY>" addresses the passthrough env map. Unknown bare names error.
func (c *Config) SetField(provider, path, value string) error {
	p, ok := c.Providers[provider]
	if !ok {
		return fmt.Errorf("provider %q not found", provider)
	}
	if key, isEnv := strings.CutPrefix(path, "env."); isEnv {
		if !ValidEnvKey(key) {
			return fmt.Errorf("invalid env key %q: must match [A-Za-z_][A-Za-z0-9_]*", key)
		}
		if p.Env == nil {
			p.Env = map[string]string{}
		}
		p.Env[key] = value
		c.Providers[provider] = p
		return nil
	}
	set, ok := typedSetters[path]
	if !ok {
		return fmt.Errorf("unknown field %q (use one of %s, or env.<KEY>)", path, typedFieldNames())
	}
	set(&p, value)
	c.Providers[provider] = p
	return nil
}

// UnsetField clears a single provider field (typed field or env.<KEY>).
func (c *Config) UnsetField(provider, path string) error {
	p, ok := c.Providers[provider]
	if !ok {
		return fmt.Errorf("provider %q not found", provider)
	}
	if key, isEnv := strings.CutPrefix(path, "env."); isEnv {
		if !ValidEnvKey(key) {
			return fmt.Errorf("invalid env key %q", key)
		}
		if _, present := p.Env[key]; !present {
			return fmt.Errorf("%s has no env key %q", provider, key)
		}
		delete(p.Env, key)
		c.Providers[provider] = p
		return nil
	}
	set, ok := typedSetters[path]
	if !ok {
		return fmt.Errorf("unknown field %q (use one of %s, or env.<KEY>)", path, typedFieldNames())
	}
	set(&p, "")
	c.Providers[provider] = p
	return nil
}
