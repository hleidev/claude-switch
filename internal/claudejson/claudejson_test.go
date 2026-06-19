package claudejson

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

const longKey = "sk-0123456789abcdefghijklmnop" // > 20 chars
var wantSuffix = longKey[len(longKey)-20:]

func writeJSON(t *testing.T, path string, v any) {
	t.Helper()
	b, _ := json.Marshal(v)
	if err := os.WriteFile(path, b, 0o600); err != nil {
		t.Fatal(err)
	}
}

func readDoc(t *testing.T, path string) map[string]any {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(b, &doc); err != nil {
		t.Fatal(err)
	}
	return doc
}

func approvedList(doc map[string]any) []any {
	resp, _ := doc["customApiKeyResponses"].(map[string]any)
	list, _ := resp["approved"].([]any)
	return list
}

func TestRegisterAppendsSuffixAndPreservesUnknown(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".claude.json")
	writeJSON(t, path, map[string]any{
		"someUnknownField": "keep-me",
		"nested":           map[string]any{"a": 1.0},
	})

	registered, err := RegisterKey(path, longKey)
	if err != nil {
		t.Fatalf("RegisterKey: %v", err)
	}
	if !registered {
		t.Error("expected registered=true on first write")
	}
	doc := readDoc(t, path)
	if doc["someUnknownField"] != "keep-me" {
		t.Errorf("unknown field lost: %+v", doc)
	}
	list := approvedList(doc)
	if len(list) != 1 || list[0] != wantSuffix {
		t.Errorf("approved = %v, want [%s]", list, wantSuffix)
	}
}

func TestRegisterIdempotent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".claude.json")
	writeJSON(t, path, map[string]any{})
	for i := 0; i < 3; i++ {
		registered, err := RegisterKey(path, longKey)
		if err != nil {
			t.Fatal(err)
		}
		if i == 0 && !registered {
			t.Error("first call should register")
		}
		if i > 0 && registered {
			t.Error("repeat call should report no write (already approved)")
		}
	}
	if list := approvedList(readDoc(t, path)); len(list) != 1 {
		t.Errorf("approved = %v, want exactly one entry", list)
	}
}

func TestRegisterMissingFileNoOp(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".claude.json")
	registered, err := RegisterKey(path, longKey)
	if err != nil {
		t.Fatalf("RegisterKey on missing file should be nil, got %v", err)
	}
	if registered {
		t.Error("missing file must report registered=false")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("file should not have been created")
	}
}

func TestRegisterShortTokenNoOp(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".claude.json")
	writeJSON(t, path, map[string]any{})
	if _, err := RegisterKey(path, "short"); err != nil {
		t.Fatal(err)
	}
	if list := approvedList(readDoc(t, path)); len(list) != 0 {
		t.Errorf("approved = %v, want empty for short token", list)
	}
}

func TestRegisterNoTempLeftover(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".claude.json")
	writeJSON(t, path, map[string]any{})
	if _, err := RegisterKey(path, longKey); err != nil {
		t.Fatal(err)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Errorf("dir has %d entries, want 1 (temp file leaked?)", len(entries))
	}
}
