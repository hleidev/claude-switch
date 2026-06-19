package shellenv

import (
	"strings"
	"testing"

	"github.com/hleidev/claude-switch/internal/config"
)

func TestForProviderGolden(t *testing.T) {
	c := &config.Config{Providers: map[string]config.Provider{
		"deepseek": {
			BaseURL:   "https://api.deepseek.com/anthropic",
			AuthToken: "sk-x",
			Model:     "deepseek-v4-pro",
			Env:       map[string]string{"CLAUDE_CODE_EFFORT_LEVEL": "max"},
		},
	}}
	got, err := ForProvider(c, "deepseek")
	if err != nil {
		t.Fatalf("ForProvider: %v", err)
	}
	want := "export ANTHROPIC_BASE_URL='https://api.deepseek.com/anthropic'\n" +
		"export ANTHROPIC_AUTH_TOKEN='sk-x'\n" +
		"export ANTHROPIC_MODEL='deepseek-v4-pro'\n" +
		"export CLAUDE_CODE_EFFORT_LEVEL='max'\n" +
		"export CLAUDE_SWITCH_PROVIDER='deepseek'\n" +
		"export _CS_MANAGED_VARS='ANTHROPIC_BASE_URL ANTHROPIC_AUTH_TOKEN ANTHROPIC_MODEL CLAUDE_CODE_EFFORT_LEVEL CLAUDE_SWITCH_PROVIDER _CS_MANAGED_VARS'\n"
	if got != want {
		t.Errorf("ForProvider golden mismatch:\n got: %q\nwant: %q", got, want)
	}
}

func TestForProviderEmptyFieldsSkipped(t *testing.T) {
	c := &config.Config{Providers: map[string]config.Provider{
		"m": {BaseURL: "https://x", Model: "MiniMax-M3"},
	}}
	got, _ := ForProvider(c, "m")
	if strings.Contains(got, "ANTHROPIC_AUTH_TOKEN") {
		t.Errorf("empty auth_token should not be exported:\n%s", got)
	}
}

func TestForProviderSkipsUnsafeEnvKey(t *testing.T) {
	// A bad key that somehow reached the config must never be emitted into the
	// eval'd output, or it would execute as shell code.
	c := &config.Config{Providers: map[string]config.Provider{
		"m": {BaseURL: "https://x", Env: map[string]string{"GOOD": "1", "BAD; touch /tmp/x": "2"}},
	}}
	got, _ := ForProvider(c, "m")
	if strings.Contains(got, "touch /tmp/x") {
		t.Errorf("unsafe key leaked into eval output:\n%s", got)
	}
	if !strings.Contains(got, "export GOOD='1'") {
		t.Errorf("valid key dropped:\n%s", got)
	}
	if strings.Contains(got, "BAD") {
		t.Errorf("unsafe key name should not appear at all:\n%s", got)
	}
}

func TestForProviderSingleQuoteEscaping(t *testing.T) {
	c := &config.Config{Providers: map[string]config.Provider{
		"m": {BaseURL: "https://x", AuthToken: "a'b"},
	}}
	got, _ := ForProvider(c, "m")
	if !strings.Contains(got, `export ANTHROPIC_AUTH_TOKEN='a'\''b'`) {
		t.Errorf("single quote not escaped:\n%s", got)
	}
}
