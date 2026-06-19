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
	if reloaded.Providers["minimax"].BaseURL != "https://x" {
		t.Errorf("provider lost across upgrade: %+v", reloaded.Providers)
	}
}
