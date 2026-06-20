package config

import (
	"os"
	"path/filepath"
	"testing"
)

// useTempConfig points config storage at an isolated temp dir for the test.
func useTempConfig(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	return filepath.Join(dir, "claude-switch", "config.toml")
}

func TestLoadMissingReturnsEmptyDefault(t *testing.T) {
	useTempConfig(t)
	c, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.Version != CurrentSchemaVersion {
		t.Errorf("Version = %d, want %d", c.Version, CurrentSchemaVersion)
	}
	if c.DefaultProvider != "claude" {
		t.Errorf("DefaultProvider = %q, want claude", c.DefaultProvider)
	}
	if c.Providers == nil || len(c.Providers) != 0 {
		t.Errorf("Providers = %v, want empty non-nil", c.Providers)
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	useTempConfig(t)
	in := &Config{
		Version:         CurrentSchemaVersion,
		DefaultProvider: "deepseek",
		Defaults:        map[string]string{"FOO": "bar"},
		Providers: map[string]Provider{
			"deepseek": {
				"ANTHROPIC_BASE_URL":            "https://api.deepseek.com/anthropic",
				"ANTHROPIC_AUTH_TOKEN":          "sk-secret",
				"ANTHROPIC_MODEL":               "deepseek-v4-pro",
				"ANTHROPIC_DEFAULT_HAIKU_MODEL": "deepseek-v4-flash",
				"CLAUDE_CODE_EFFORT_LEVEL":      "max",
			},
		},
	}
	if err := Save(in); err != nil {
		t.Fatalf("Save: %v", err)
	}
	out, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if out.DefaultProvider != "deepseek" {
		t.Errorf("DefaultProvider = %q", out.DefaultProvider)
	}
	p := out.Providers["deepseek"]
	if p.AuthToken() != "sk-secret" || p["ANTHROPIC_MODEL"] != "deepseek-v4-pro" || p["ANTHROPIC_DEFAULT_HAIKU_MODEL"] != "deepseek-v4-flash" {
		t.Errorf("provider round-trip mismatch: %+v", p)
	}
	if p["CLAUDE_CODE_EFFORT_LEVEL"] != "max" {
		t.Errorf("env round-trip mismatch: %+v", p)
	}
	if out.Defaults["FOO"] != "bar" {
		t.Errorf("defaults round-trip mismatch: %+v", out.Defaults)
	}
}

func TestLoadMinimizesAgainstPresets(t *testing.T) {
	path := useTempConfig(t)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	SetPresetLookup(func(name string) (map[string]string, bool) {
		if name == "glm" {
			return map[string]string{
				"ANTHROPIC_BASE_URL": "https://x",
				"ANTHROPIC_MODEL":    "glm-5.2",
			}, true
		}
		return nil, false
	})
	defer SetPresetLookup(nil)

	// A flattened v2 config that materialized preset-equal values plus one
	// genuine override and a custom (no-preset) provider.
	content := "version = 2\n" +
		"[providers.glm]\n" +
		"ANTHROPIC_AUTH_TOKEN = 'sk'\n" +
		"ANTHROPIC_BASE_URL = 'https://x'\n" + // equals preset -> dropped
		"ANTHROPIC_MODEL = 'glm-4.7'\n" + // differs from preset -> kept
		"[providers.acme]\n" +
		"ANTHROPIC_BASE_URL = 'https://acme'\n" // no preset -> untouched
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	c, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	glm := c.Providers["glm"]
	if _, ok := glm["ANTHROPIC_BASE_URL"]; ok {
		t.Errorf("preset-equal base_url should have been dropped: %+v", glm)
	}
	if glm["ANTHROPIC_MODEL"] != "glm-4.7" {
		t.Errorf("genuine override should be kept: %+v", glm)
	}
	if glm.AuthToken() != "sk" {
		t.Errorf("secret must always be kept: %+v", glm)
	}
	if c.Providers["acme"]["ANTHROPIC_BASE_URL"] != "https://acme" {
		t.Errorf("custom provider must be untouched: %+v", c.Providers["acme"])
	}

	// The minimization must have been persisted.
	reloaded, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := reloaded.Providers["glm"]["ANTHROPIC_BASE_URL"]; ok {
		t.Errorf("minimization not persisted: %+v", reloaded.Providers["glm"])
	}
}

func TestSaveSets0600(t *testing.T) {
	path := useTempConfig(t)
	if err := Save(&Config{Version: CurrentSchemaVersion, Providers: map[string]Provider{}}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if perm := fi.Mode().Perm(); perm != 0o600 {
		t.Errorf("config perm = %o, want 600", perm)
	}
	di, err := os.Stat(filepath.Dir(path))
	if err != nil {
		t.Fatalf("Stat dir: %v", err)
	}
	if perm := di.Mode().Perm(); perm != 0o700 {
		t.Errorf("dir perm = %o, want 700", perm)
	}
}

func TestLoadRejectsNewerVersion(t *testing.T) {
	path := useTempConfig(t)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	content := "version = 999\ndefault_provider = 'claude'\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(); err == nil {
		t.Error("expected Load to reject a config newer than CurrentSchemaVersion")
	}
}

func TestConfigPathHonorsXDG(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	got, err := ConfigPath()
	if err != nil {
		t.Fatalf("ConfigPath: %v", err)
	}
	want := filepath.Join(dir, "claude-switch", "config.toml")
	if got != want {
		t.Errorf("ConfigPath = %q, want %q", got, want)
	}
}
