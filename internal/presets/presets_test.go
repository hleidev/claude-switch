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
	if m.BaseURL != "https://api.minimaxi.com/anthropic" || m.Model != "MiniMax-M3" {
		t.Errorf("minimax core fields wrong: %+v", m)
	}
	if m.Env["API_TIMEOUT_MS"] != "3000000" {
		t.Errorf("minimax env wrong: %+v", m.Env)
	}
	if m.AuthToken != "" {
		t.Errorf("preset must not carry a secret, got %q", m.AuthToken)
	}

	d, ok := Lookup("deepseek")
	if !ok {
		t.Fatal("deepseek preset missing")
	}
	if d.HaikuModel != "deepseek-v4-flash" || d.Env["CLAUDE_CODE_EFFORT_LEVEL"] != "max" {
		t.Errorf("deepseek fields wrong: %+v", d)
	}
}

func TestLookupUnknown(t *testing.T) {
	if _, ok := Lookup("does-not-exist"); ok {
		t.Error("expected unknown preset to return false")
	}
}

func TestNamesSorted(t *testing.T) {
	got := Names()
	want := []string{"anthropic", "deepseek", "minimax"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Names = %v, want %v", got, want)
	}
}
