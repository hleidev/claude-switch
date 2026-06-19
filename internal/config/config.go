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
const CurrentSchemaVersion = 1

// Config is the top-level configuration document.
type Config struct {
	Version         int                 `toml:"version"`
	DefaultProvider string              `toml:"default_provider"`
	Defaults        Defaults            `toml:"defaults"`
	Providers       map[string]Provider `toml:"providers"`
}

// Defaults holds preferences applied to every provider before its own values.
type Defaults struct {
	Env map[string]string `toml:"env,omitempty"`
}

// Provider is a single backend definition. Typed fields map to well-known
// Claude Code environment variables; Env carries provider-specific passthrough.
type Provider struct {
	BaseURL        string            `toml:"base_url"`
	AuthToken      string            `toml:"auth_token,omitempty"`
	Model          string            `toml:"model,omitempty"`
	SmallFastModel string            `toml:"small_fast_model,omitempty"`
	SonnetModel    string            `toml:"sonnet_model,omitempty"`
	OpusModel      string            `toml:"opus_model,omitempty"`
	HaikuModel     string            `toml:"haiku_model,omitempty"`
	Env            map[string]string `toml:"env,omitempty"`
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
	// Persist an in-place upgrade once, keeping a one-time backup of the original.
	if changed {
		_ = os.WriteFile(path+".bak", data, 0o600)
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
