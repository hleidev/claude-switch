package config

import "fmt"

// schemaMigration upgrades an in-place config tree by exactly one version.
//
// This is the standardized hook for FUTURE schema changes (same file, same
// location). To introduce schema version 2:
//  1. bump CurrentSchemaVersion to 2,
//  2. append {toVersion: 2, apply: migrateV1ToV2} below,
//  3. write migrateV1ToV2 to transform the raw tree from the v1 layout to v2.
//
// Migrations operate on the decoded TOML tree (map[string]any) rather than the
// Config struct, so a step can read fields the current struct no longer has and
// rename/restructure them. Each step must be idempotent-safe and set the tree's
// "version" to toVersion (the runner does this after a successful apply).
//
// The legacy bash-era layout (schema version 0) lives in a DIFFERENT location
// and format (~/.claude-switch/), so its import is handled explicitly by
// `cs migrate`, not by this in-place chain. The chain covers versions >= 1.
type schemaMigration struct {
	toVersion int
	apply     func(tree map[string]any) error
}

// schemaMigrations is the ordered upgrade chain. Empty today (only v1 exists).
var schemaMigrations = []schemaMigration{}

// treeVersion reads the "version" field from a decoded TOML tree; an absent or
// non-integer value means version 0 (pre-versioned).
func treeVersion(tree map[string]any) int {
	switch v := tree["version"].(type) {
	case int64:
		return int(v)
	case int:
		return v
	case float64:
		return int(v)
	default:
		return 0
	}
}

// upgradeTree applies registered migrations in order until the tree reaches
// CurrentSchemaVersion, returning whether anything changed. It refuses a config
// newer than this build understands, since saving it would silently drop
// fields this binary does not know about.
func upgradeTree(tree map[string]any) (bool, error) {
	if from := treeVersion(tree); from > CurrentSchemaVersion {
		return false, fmt.Errorf("config schema version %d is newer than this claude-switch supports (%d); please upgrade claude-switch", from, CurrentSchemaVersion)
	}
	changed := false
	for _, m := range schemaMigrations {
		if treeVersion(tree) < m.toVersion && m.toVersion <= CurrentSchemaVersion {
			if err := m.apply(tree); err != nil {
				return false, fmt.Errorf("migrating config to schema v%d: %w", m.toVersion, err)
			}
			tree["version"] = int64(m.toVersion)
			changed = true
		}
	}
	// Stamp the current version even when no migration ran (e.g. a v1 file that
	// predates the version field).
	if treeVersion(tree) < CurrentSchemaVersion {
		tree["version"] = int64(CurrentSchemaVersion)
		changed = true
	}
	return changed, nil
}
