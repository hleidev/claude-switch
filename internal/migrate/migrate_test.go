package migrate

import (
	"testing"

	"github.com/hleidev/claude-switch/internal/config"
)

func TestToProviderFlattensVars(t *testing.T) {
	vars := map[string]string{
		"ANTHROPIC_BASE_URL":            "https://x/anthropic",
		"ANTHROPIC_MODEL":               "custom-model",
		"ANTHROPIC_AUTH_TOKEN":          "sk-tok",
		"ANTHROPIC_DEFAULT_HAIKU_MODEL": "fast",
		"API_TIMEOUT_MS":                "3000000",
		"CLAUDE_CODE_EFFORT_LEVEL":      "max",
		"BAD KEY":                       "dropped", // unsafe name must be skipped
	}
	p := ToProvider(config.Provider{}, vars)
	if p.BaseURL() != "https://x/anthropic" || p["ANTHROPIC_MODEL"] != "custom-model" || p.AuthToken() != "sk-tok" {
		t.Errorf("core vars wrong: %+v", p)
	}
	if p["ANTHROPIC_DEFAULT_HAIKU_MODEL"] != "fast" {
		t.Errorf("role model not carried: %+v", p)
	}
	if p["API_TIMEOUT_MS"] != "3000000" || p["CLAUDE_CODE_EFFORT_LEVEL"] != "max" {
		t.Errorf("passthrough vars not imported: %+v", p)
	}
	if _, ok := p["BAD KEY"]; ok {
		t.Error("unsafe env key should have been skipped")
	}
}

func TestParseLegacyDefault(t *testing.T) {
	cases := map[string]string{
		"[core]\ndefault_provider = \"minimax\"\n":        "minimax",
		"default_provider = 'deepseek'\n":                 "deepseek",
		"# comment\ndefault_provider=claude\n":            "claude",
		"[core]\nother = 1\n":                             "",
		"# default_provider = \"commented\"\nother = 1\n": "",
	}
	for content, want := range cases {
		if got := ParseLegacyDefault(content); got != want {
			t.Errorf("ParseLegacyDefault(%q) = %q, want %q", content, got, want)
		}
	}
}

func TestToProviderResolvesKeyReference(t *testing.T) {
	// Mirrors the real legacy layout: a holder var referenced by AUTH_TOKEN.
	vars := map[string]string{
		"DEEPSEEK_API_KEY":     "sk-real-1234567890",
		"ANTHROPIC_AUTH_TOKEN": "${DEEPSEEK_API_KEY}",
	}
	p := ToProvider(config.Provider{}, vars)
	if p.AuthToken() != "sk-real-1234567890" {
		t.Errorf("auth_token = %q, want resolved real key", p.AuthToken())
	}
	if _, ok := p["DEEPSEEK_API_KEY"]; ok {
		t.Errorf("holder var should not be duplicated: %+v", p)
	}
}

func TestToProviderPreservesPresetWhenAbsent(t *testing.T) {
	base := config.Provider{"ANTHROPIC_BASE_URL": "https://preset", "ANTHROPIC_MODEL": "preset-model", "K": "v"}
	p := ToProvider(base, map[string]string{"ANTHROPIC_AUTH_TOKEN": "sk-only"})
	if p.BaseURL() != "https://preset" || p["ANTHROPIC_MODEL"] != "preset-model" || p["K"] != "v" {
		t.Errorf("preset fields should be preserved when not in vars: %+v", p)
	}
	if p.AuthToken() != "sk-only" {
		t.Errorf("token not applied: %+v", p)
	}
}

func TestParseExports(t *testing.T) {
	content := `# MiniMax provider key
export ANTHROPIC_AUTH_TOKEN="sk-secret-123"
export ANTHROPIC_BASE_URL='https://api.minimaxi.com/anthropic'
export ANTHROPIC_MODEL=MiniMax-M3

# a comment
not_an_export_line
export API_TIMEOUT_MS="3000000"
`
	got := ParseExports(content)
	want := map[string]string{
		"ANTHROPIC_AUTH_TOKEN": "sk-secret-123",
		"ANTHROPIC_BASE_URL":   "https://api.minimaxi.com/anthropic",
		"ANTHROPIC_MODEL":      "MiniMax-M3",
		"API_TIMEOUT_MS":       "3000000",
	}
	if len(got) != len(want) {
		t.Fatalf("got %d pairs %v, want %d", len(got), got, len(want))
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s = %q, want %q", k, got[k], v)
		}
	}
}

func TestParseExportsHandlesBareAssignment(t *testing.T) {
	got := ParseExports("ANTHROPIC_AUTH_TOKEN=sk-bare")
	if got["ANTHROPIC_AUTH_TOKEN"] != "sk-bare" {
		t.Errorf("got %v", got)
	}
}

func TestParseExportsStripsInlineComments(t *testing.T) {
	cases := map[string]string{
		`export TOKEN="sk-x" # trailing comment`: "sk-x",
		`export TOKEN='sk-y'  # note`:            "sk-y",
		`export TOKEN=sk-z # bare with comment`:  "sk-z",
		`export TOKEN=sk-plain`:                  "sk-plain",
	}
	for line, want := range cases {
		got := ParseExports(line)
		if got["TOKEN"] != want {
			t.Errorf("ParseExports(%q) TOKEN = %q, want %q", line, got["TOKEN"], want)
		}
	}
}
