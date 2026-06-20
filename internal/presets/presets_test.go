package presets

import (
	"reflect"
	"testing"
)

func TestLookupKnown(t *testing.T) {
	m, ok := Lookup("minimax")
	if !ok {
		t.Fatal("minimax preset missing")
	}
	if m["ANTHROPIC_BASE_URL"] != "https://api.minimaxi.com/anthropic" || m["ANTHROPIC_DEFAULT_SONNET_MODEL"] != "MiniMax-M3" {
		t.Errorf("minimax core vars wrong: %+v", m)
	}
	if m["ANTHROPIC_MODEL"] != "" {
		t.Errorf("minimax should not pin ANTHROPIC_MODEL, got %q", m["ANTHROPIC_MODEL"])
	}
	if m["API_TIMEOUT_MS"] != "3000000" {
		t.Errorf("minimax timeout wrong: %+v", m)
	}
	if m["ANTHROPIC_AUTH_TOKEN"] != "" {
		t.Errorf("preset must not carry a secret, got %q", m["ANTHROPIC_AUTH_TOKEN"])
	}

	d, ok := Lookup("deepseek")
	if !ok {
		t.Fatal("deepseek preset missing")
	}
	if d["ANTHROPIC_DEFAULT_HAIKU_MODEL"] != "deepseek-v4-flash" || d["CLAUDE_CODE_EFFORT_LEVEL"] != "max" {
		t.Errorf("deepseek vars wrong: %+v", d)
	}
}

func TestLookupUnknown(t *testing.T) {
	if _, ok := Lookup("does-not-exist"); ok {
		t.Error("expected unknown preset to return false")
	}
}

func TestNamesSorted(t *testing.T) {
	got := Names()
	want := []string{"anthropic", "deepseek", "glm", "minimax"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Names = %v, want %v", got, want)
	}
}
