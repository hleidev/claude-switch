// Package config defines the claude-switch configuration schema and its
// load/save logic. The config is a single TOML file at
// $XDG_CONFIG_HOME/claude-switch/config.toml (default ~/.config/...), holding
// provider definitions with inline secrets, protected by 0600 permissions.
package config

import (
	"fmt"
	"os"
	"path/filepath"

	toml "github.com/pelletier/go-toml/v2"
)

// CurrentSchemaVersion is the on-disk config FORMAT version understood by this
// build. It is a monotonic integer, deliberately separate from the application
// (binary) version, which is SemVer (see cmd.version). Bump it only when the
// config.toml layout changes incompatibly, and add a matching entry to
// schemaMigrations. An absent `version` field is treated as version 0 (the
// pre-versioned, bash-era layout).
const CurrentSchemaVersion = 2

// Well-known environment variable names that get special handling. Everything
// else in a provider is treated as an opaque passthrough variable.
const (
	// AuthTokenKey holds the API key. It is the one secret in a provider: it is
	// collected via hidden input, masked in list/status, and registered with
	// Claude Code. Its presence is also what selects API-key mode over OAuth.
	AuthTokenKey = "ANTHROPIC_AUTH_TOKEN"
	// BaseURLKey is the API endpoint, used by the connectivity probe.
	BaseURLKey = "ANTHROPIC_BASE_URL"
	// ProviderVar records the active provider name in the injected environment.
	ProviderVar = "CLAUDE_SWITCH_PROVIDER"
)

// Config is the top-level configuration document.
type Config struct {
	Version         int                 `toml:"version"`
	DefaultProvider string              `toml:"default_provider"`
	Defaults        map[string]string   `toml:"defaults,omitempty"`
	Providers       map[string]Provider `toml:"providers,omitempty"`
}

// Provider is a single backend definition: a flat set of environment variables
// to export, keyed by their real variable names. There are no typed fields —
// base_url, model, timeouts, and any passthrough var are all just entries here.
// Built-in defaults live in the project's presets; a user's config only needs
// to hold the secret (AuthTokenKey) plus any overrides.
type Provider map[string]string

// AuthToken returns the provider's API key (empty if it relies on OAuth).
func (p Provider) AuthToken() string { return p[AuthTokenKey] }

// BaseURL returns the provider's configured endpoint (empty if it comes from a
// preset instead).
func (p Provider) BaseURL() string { return p[BaseURLKey] }

// presetLookup resolves a provider's built-in preset variables. It is injected
// by the cmd layer (via SetPresetLookup) rather than imported, so this package
// stays decoupled from the embedded presets and is easy to test. When unset
// (e.g. in unit tests), preset-based minimization is skipped.
var presetLookup func(name string) (map[string]string, bool)

// SetPresetLookup wires the preset resolver used to minimize stored configs.
func SetPresetLookup(f func(name string) (map[string]string, bool)) {
	presetLookup = f
}

// minimizeProviders removes any provider variable whose stored value already
// equals the built-in preset default (the secret is always kept). It returns
// whether anything was removed. A genuine override — a value that differs from
// the preset — is preserved.
func minimizeProviders(c *Config) bool {
	if presetLookup == nil {
		return false
	}
	removed := false
	for name, p := range c.Providers {
		preset, ok := presetLookup(name)
		if !ok {
			continue
		}
		for k, v := range p {
			if k == AuthTokenKey {
				continue
			}
			if pv, ok := preset[k]; ok && pv == v {
				delete(p, k)
				removed = true
			}
		}
		c.Providers[name] = p
	}
	return removed
}

// ConfigDir returns the directory holding config.toml, honoring XDG_CONFIG_HOME.
func ConfigDir() (string, error) {
	if x := os.Getenv("XDG_CONFIG_HOME"); x != "" {
		return filepath.Join(x, "claude-switch"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "claude-switch"), nil
}

// ConfigPath returns the absolute path to config.toml.
func ConfigPath() (string, error) {
	dir, err := ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.toml"), nil
}

// Load reads the config file. A missing file yields an empty, current-version
// config defaulting to the OAuth ("claude") provider, not an error.
func Load() (*Config, error) {
	path, err := ConfigPath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return &Config{
			Version:         CurrentSchemaVersion,
			DefaultProvider: "claude",
			Providers:       map[string]Provider{},
		}, nil
	}
	if err != nil {
		return nil, err
	}

	// Decode to a generic tree first so schema migrations can transform older
	// layouts before we bind to the current struct. upgradeTree also rejects a
	// config newer than this build understands.
	var tree map[string]any
	if err := toml.Unmarshal(data, &tree); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	changed, err := upgradeTree(tree)
	if err != nil {
		return nil, err
	}
	encoded, err := toml.Marshal(tree)
	if err != nil {
		return nil, err
	}
	var c Config
	if err := toml.Unmarshal(encoded, &c); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if c.Providers == nil {
		c.Providers = map[string]Provider{}
	}
	if c.DefaultProvider == "" {
		c.DefaultProvider = "claude"
	}
	// Drop any stored value that merely duplicates the built-in preset default,
	// so configs shrink to just the secret plus genuine overrides and track
	// future preset changes automatically.
	minimized := minimizeProviders(&c)
	// Persist an in-place change once, keeping a one-time backup of the original
	// whenever the on-disk schema was upgraded.
	if changed || minimized {
		if changed {
			_ = os.WriteFile(path+".bak", data, 0o600)
		}
		if err := Save(&c); err != nil {
			return nil, err
		}
	}
	return &c, nil
}

// Save writes the config atomically (temp file + rename) with 0600 permissions,
// creating the directory at 0700 if needed.
func Save(c *Config) error {
	dir, err := ConfigDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	// Saving always writes the current schema version; Load has already upgraded
	// any older layout to the current struct by this point.
	c.Version = CurrentSchemaVersion
	data, err := toml.Marshal(c)
	if err != nil {
		return err
	}
	path := filepath.Join(dir, "config.toml")
	tmp, err := os.CreateTemp(dir, ".config-*.toml")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}
