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
	if m["ANTHROPIC_BASE_URL"] != "https://api.minimaxi.com/anthropic" || m["ANTHROPIC_DEFAULT_SONNET_MODEL"] != "MiniMax-M3[1m]" {
		t.Errorf("minimax core vars wrong: %+v", m)
	}
	if m["ANTHROPIC_MODEL"] != "" {
		t.Errorf("minimax should not pin ANTHROPIC_MODEL, got %q", m["ANTHROPIC_MODEL"])
	}
	if m["ANTHROPIC_AUTH_TOKEN"] != "" {
		t.Errorf("preset must not carry a secret, got %q", m["ANTHROPIC_AUTH_TOKEN"])
	}

	d, ok := Lookup("deepseek")
	if !ok {
		t.Fatal("deepseek preset missing")
	}
	if d["ANTHROPIC_DEFAULT_HAIKU_MODEL"] != "deepseek-v4-flash[1m]" || d["CLAUDE_CODE_EFFORT_LEVEL"] != "max" {
		t.Errorf("deepseek vars wrong: %+v", d)
	}
	cp, ok := Lookup("coderplan")
	if !ok {
		t.Fatal("coderplan preset missing")
	}
	if cp["ANTHROPIC_BASE_URL"] != "https://api.coderplan.ai" {
		t.Errorf("coderplan base url wrong: %+v", cp)
	}
	if cp["ANTHROPIC_DEFAULT_SONNET_MODEL"] != "claude-sonnet-5-5" || cp["ANTHROPIC_DEFAULT_OPUS_MODEL"] != "claude-opus-5-5" || cp["ANTHROPIC_DEFAULT_HAIKU_MODEL"] != "claude-haiku-5-5" {
		t.Errorf("coderplan model mappings wrong: %+v", cp)
	}

	cx, ok := Lookup("codex")
	if !ok {
		t.Fatal("codex preset missing")
	}
	if cx["ANTHROPIC_BASE_URL"] != "http://127.0.0.1:8787/v1" || cx["CLAUDE_SWITCH_PROTOCOL"] != "openai" {
		t.Errorf("codex vars wrong: %+v", cx)
	}
}

func TestLookupUnknown(t *testing.T) {
	if _, ok := Lookup("does-not-exist"); ok {
		t.Error("expected unknown preset to return false")
	}
}

func TestNamesSorted(t *testing.T) {
	got := Names()
	want := []string{"anthropic", "coderplan", "codex", "deepseek", "glm", "minimax"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Names = %v, want %v", got, want)
	}
}
