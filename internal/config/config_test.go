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
		Defaults:        Defaults{Env: map[string]string{"FOO": "bar"}},
		Providers: map[string]Provider{
			"deepseek": {
				BaseURL:    "https://api.deepseek.com/anthropic",
				AuthToken:  "sk-secret",
				Model:      "deepseek-v4-pro",
				HaikuModel: "deepseek-v4-flash",
				Env:        map[string]string{"CLAUDE_CODE_EFFORT_LEVEL": "max"},
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
	if p.AuthToken != "sk-secret" || p.Model != "deepseek-v4-pro" || p.HaikuModel != "deepseek-v4-flash" {
		t.Errorf("provider round-trip mismatch: %+v", p)
	}
	if p.Env["CLAUDE_CODE_EFFORT_LEVEL"] != "max" {
		t.Errorf("env round-trip mismatch: %+v", p.Env)
	}
	if out.Defaults.Env["FOO"] != "bar" {
		t.Errorf("defaults.env round-trip mismatch: %+v", out.Defaults.Env)
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
