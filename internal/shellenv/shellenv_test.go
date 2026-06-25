package shellenv

import (
	"strings"
	"testing"

	"github.com/hleidev/claude-switch/internal/config"
)

func TestForProviderGolden(t *testing.T) {
	c := &config.Config{Providers: map[string]config.Provider{
		"deepseek": {
			"ANTHROPIC_BASE_URL":       "https://api.deepseek.com/anthropic",
			"ANTHROPIC_AUTH_TOKEN":     "sk-x",
			"ANTHROPIC_MODEL":          "deepseek-v4-pro",
			"CLAUDE_CODE_EFFORT_LEVEL": "max",
		},
	}}
	got, err := ForProvider(c, "deepseek", nil)
	if err != nil {
		t.Fatalf("ForProvider: %v", err)
	}
	// Output is sorted by variable name, with the provider marker last.
	want := "export ANTHROPIC_AUTH_TOKEN='sk-x'\n" +
		"export ANTHROPIC_BASE_URL='https://api.deepseek.com/anthropic'\n" +
		"export ANTHROPIC_MODEL='deepseek-v4-pro'\n" +
		"export CLAUDE_CODE_EFFORT_LEVEL='max'\n" +
		"export CLAUDE_SWITCH_PROVIDER='deepseek'\n" +
		"export ENABLE_CLAUDEAI_MCP_SERVERS=false\n" +
		"export _CS_MANAGED_VARS='ANTHROPIC_AUTH_TOKEN ANTHROPIC_BASE_URL ANTHROPIC_MODEL CLAUDE_CODE_EFFORT_LEVEL CLAUDE_SWITCH_PROVIDER ENABLE_CLAUDEAI_MCP_SERVERS _CS_MANAGED_VARS'\n"
	if got != want {
		t.Errorf("ForProvider golden mismatch:\n got: %q\nwant: %q", got, want)
	}
}

func TestForProviderMergesPreset(t *testing.T) {
	// The provider supplies only the key; the preset supplies the rest.
	c := &config.Config{Providers: map[string]config.Provider{
		"glm": {"ANTHROPIC_AUTH_TOKEN": "sk-x"},
	}}
	preset := map[string]string{
		"ANTHROPIC_BASE_URL": "https://open.bigmodel.cn/api/anthropic",
		"ANTHROPIC_MODEL":    "glm-5.2",
	}
	got, _ := ForProvider(c, "glm", preset)
	if !strings.Contains(got, "export ANTHROPIC_MODEL='glm-5.2'") {
		t.Errorf("preset model not merged:\n%s", got)
	}
	if !strings.Contains(got, "export ANTHROPIC_AUTH_TOKEN='sk-x'") {
		t.Errorf("provider key missing:\n%s", got)
	}
}

func TestForProviderEmptyFieldsSkipped(t *testing.T) {
	c := &config.Config{Providers: map[string]config.Provider{
		"m": {"ANTHROPIC_BASE_URL": "https://x", "ANTHROPIC_MODEL": "MiniMax-M3"},
	}}
	got, _ := ForProvider(c, "m", nil)
	if strings.Contains(got, "ANTHROPIC_AUTH_TOKEN") {
		t.Errorf("absent auth_token should not be exported:\n%s", got)
	}
}

func TestForProviderSkipsUnsafeEnvKey(t *testing.T) {
	// A bad key that somehow reached the config must never be emitted into the
	// eval'd output, or it would execute as shell code.
	c := &config.Config{Providers: map[string]config.Provider{
		"m": {"ANTHROPIC_BASE_URL": "https://x", "GOOD": "1", "BAD; touch /tmp/x": "2"},
	}}
	got, _ := ForProvider(c, "m", nil)
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
		"m": {"ANTHROPIC_BASE_URL": "https://x", "ANTHROPIC_AUTH_TOKEN": "a'b"},
	}}
	got, _ := ForProvider(c, "m", nil)
	if !strings.Contains(got, `export ANTHROPIC_AUTH_TOKEN='a'\''b'`) {
		t.Errorf("single quote not escaped:\n%s", got)
	}
}
