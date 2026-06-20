package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestUpgradeTreeStampsAbsentVersion(t *testing.T) {
	tree := map[string]any{"default_provider": "claude"}
	changed, err := upgradeTree(tree)
	if err != nil {
		t.Fatalf("upgradeTree: %v", err)
	}
	if !changed {
		t.Error("expected changed=true for a version-less tree")
	}
	if treeVersion(tree) != CurrentSchemaVersion {
		t.Errorf("version stamped to %d, want %d", treeVersion(tree), CurrentSchemaVersion)
	}
}

func TestUpgradeTreeCurrentIsNoop(t *testing.T) {
	tree := map[string]any{"version": int64(CurrentSchemaVersion)}
	changed, err := upgradeTree(tree)
	if err != nil {
		t.Fatalf("upgradeTree: %v", err)
	}
	if changed {
		t.Error("a current-version tree should not change")
	}
}

func TestUpgradeTreeRejectsNewer(t *testing.T) {
	tree := map[string]any{"version": int64(CurrentSchemaVersion + 1)}
	if _, err := upgradeTree(tree); err == nil {
		t.Error("expected error for a newer-than-supported schema version")
	}
}

func TestLoadStampsAndBacksUpVersionlessFile(t *testing.T) {
	path := useTempConfig(t)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	// A config that predates the version field.
	content := "default_provider = 'claude'\n[providers.minimax]\nbase_url = 'https://x'\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	c, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.Version != CurrentSchemaVersion {
		t.Errorf("loaded version = %d, want %d", c.Version, CurrentSchemaVersion)
	}
	// The upgrade should have been persisted with a one-time backup.
	if _, err := os.Stat(path + ".bak"); err != nil {
		t.Errorf("expected backup at %s.bak: %v", path, err)
	}
	reloaded, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.Providers["minimax"].BaseURL() != "https://x" {
		t.Errorf("provider lost across upgrade: %+v", reloaded.Providers)
	}
}

func TestMigrateV1ToV2FlattensProvider(t *testing.T) {
	// A v1 provider with typed fields and a nested env table becomes a flat
	// variable map; empty typed fields are dropped.
	tree := map[string]any{
		"version": int64(1),
		"providers": map[string]any{
			"glm": map[string]any{
				"base_url":     "https://open.bigmodel.cn/api/anthropic",
				"auth_token":   "sk-x",
				"model":        "glm-5.2",
				"sonnet_model": "glm-5.2[1m]",
				"opus_model":   "", // empty typed field must be dropped
				"env": map[string]any{
					"API_TIMEOUT_MS": "3000000",
					"EMPTY":          "", // empty env entry must be dropped
				},
			},
		},
		"defaults": map[string]any{
			"env": map[string]any{"FOO": "bar"},
		},
	}
	if _, err := upgradeTree(tree); err != nil {
		t.Fatalf("upgradeTree: %v", err)
	}
	if treeVersion(tree) != 2 {
		t.Fatalf("version = %d, want 2", treeVersion(tree))
	}
	glm := tree["providers"].(map[string]any)["glm"].(map[string]any)
	want := map[string]string{
		"ANTHROPIC_BASE_URL":             "https://open.bigmodel.cn/api/anthropic",
		"ANTHROPIC_AUTH_TOKEN":           "sk-x",
		"ANTHROPIC_MODEL":                "glm-5.2",
		"ANTHROPIC_DEFAULT_SONNET_MODEL": "glm-5.2[1m]",
		"API_TIMEOUT_MS":                 "3000000",
	}
	for k, v := range want {
		if glm[k] != v {
			t.Errorf("glm[%q] = %v, want %q", k, glm[k], v)
		}
	}
	for _, gone := range []string{"base_url", "model", "env", "ANTHROPIC_DEFAULT_OPUS_MODEL", "EMPTY"} {
		if _, ok := glm[gone]; ok {
			t.Errorf("key %q should have been dropped/renamed: %+v", gone, glm)
		}
	}
	defaults := tree["defaults"].(map[string]any)
	if defaults["FOO"] != "bar" {
		t.Errorf("defaults not flattened: %+v", defaults)
	}
	if _, ok := defaults["env"]; ok {
		t.Errorf("defaults.env should have been lifted: %+v", defaults)
	}
}
