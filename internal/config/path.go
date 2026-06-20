package config

import "fmt"

// resolveFieldKey maps the friendly secret aliases to the real variable name;
// every other field is already a real environment variable name.
func resolveFieldKey(field string) string {
	if field == "key" || field == "auth_token" {
		return AuthTokenKey
	}
	return field
}

// SetField sets a single provider variable. The field is the real environment
// variable name (e.g. ANTHROPIC_MODEL), except "key"/"auth_token" which alias
// the secret. The name must be a valid environment variable identifier.
func (c *Config) SetField(provider, field, value string) error {
	p, ok := c.Providers[provider]
	if !ok {
		return fmt.Errorf("provider %q not found", provider)
	}
	key := resolveFieldKey(field)
	if !ValidEnvKey(key) {
		return fmt.Errorf("invalid variable name %q: must match [A-Za-z_][A-Za-z0-9_]*", field)
	}
	if p == nil {
		p = Provider{}
	}
	p[key] = value
	c.Providers[provider] = p
	return nil
}

// UnsetField clears a single provider variable by its real name (or the
// "key"/"auth_token" alias).
func (c *Config) UnsetField(provider, field string) error {
	p, ok := c.Providers[provider]
	if !ok {
		return fmt.Errorf("provider %q not found", provider)
	}
	key := resolveFieldKey(field)
	if !ValidEnvKey(key) {
		return fmt.Errorf("invalid variable name %q", field)
	}
	if _, present := p[key]; !present {
		return fmt.Errorf("%s has no variable %q", provider, key)
	}
	delete(p, key)
	c.Providers[provider] = p
	return nil
}
