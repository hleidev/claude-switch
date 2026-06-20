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

// schemaMigrations is the ordered upgrade chain.
var schemaMigrations = []schemaMigration{
	{toVersion: 2, apply: migrateV1ToV2},
}

// v1TypedToEnv maps the v1 typed provider fields to the real environment
// variable names used in the v2 flat layout.
var v1TypedToEnv = map[string]string{
	"base_url":         "ANTHROPIC_BASE_URL",
	"auth_token":       "ANTHROPIC_AUTH_TOKEN",
	"model":            "ANTHROPIC_MODEL",
	"small_fast_model": "ANTHROPIC_SMALL_FAST_MODEL",
	"sonnet_model":     "ANTHROPIC_DEFAULT_SONNET_MODEL",
	"opus_model":       "ANTHROPIC_DEFAULT_OPUS_MODEL",
	"haiku_model":      "ANTHROPIC_DEFAULT_HAIKU_MODEL",
}

// migrateV1ToV2 flattens the v1 layout (typed fields + a nested [env] table)
// into v2, where each provider — and the top-level [defaults] — is a single
// flat map of environment variables. Typed fields are renamed to their real
// variable names; nested env entries are lifted up; empty values are dropped.
// Existing materialized values survive as overrides, so v1 configs keep working.
func migrateV1ToV2(tree map[string]any) error {
	if provs, ok := tree["providers"].(map[string]any); ok {
		for name, raw := range provs {
			if old, ok := raw.(map[string]any); ok {
				provs[name] = flattenV1Provider(old)
			}
		}
	}
	if def, ok := tree["defaults"].(map[string]any); ok {
		tree["defaults"] = flattenV1EnvHolder(def)
	}
	return nil
}

// flattenV1Provider converts one v1 provider table to a flat v2 variable map.
func flattenV1Provider(old map[string]any) map[string]any {
	out := map[string]any{}
	for k, v := range old {
		if k == "env" {
			if env, ok := v.(map[string]any); ok {
				mergeNonEmpty(out, env)
			}
			continue
		}
		if envName, ok := v1TypedToEnv[k]; ok {
			if s, _ := v.(string); s != "" {
				out[envName] = v
			}
			continue
		}
		// Unknown key (already a real variable name, or something hand-added):
		// keep it as-is so nothing is silently dropped.
		out[k] = v
	}
	return out
}

// flattenV1EnvHolder lifts a v1 [defaults] table (which held only a nested env
// map) into a flat variable map.
func flattenV1EnvHolder(def map[string]any) map[string]any {
	out := map[string]any{}
	if env, ok := def["env"].(map[string]any); ok {
		mergeNonEmpty(out, env)
	}
	for k, v := range def {
		if k != "env" {
			out[k] = v
		}
	}
	return out
}

// mergeNonEmpty copies non-empty string values from src into dst.
func mergeNonEmpty(dst, src map[string]any) {
	for k, v := range src {
		if s, _ := v.(string); s != "" {
			dst[k] = v
		}
	}
}

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
