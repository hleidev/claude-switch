package config

import "testing"

func TestBuildEnvMergesAndSorts(t *testing.T) {
	// Preset supplies the base; the provider overrides the key and adds nothing
	// else. Output is sorted, with CLAUDE_SWITCH_PROVIDER last.
	preset := map[string]string{
		"ANTHROPIC_BASE_URL":            "https://api.deepseek.com/anthropic",
		"ANTHROPIC_MODEL":               "deepseek-v4-pro",
		"ANTHROPIC_DEFAULT_HAIKU_MODEL": "deepseek-v4-flash",
		"CLAUDE_CODE_EFFORT_LEVEL":      "max",
	}
	c := &Config{Providers: map[string]Provider{
		"deepseek": {"ANTHROPIC_AUTH_TOKEN": "sk-x"},
	}}
	got, err := c.BuildEnv("deepseek", preset)
	if err != nil {
		t.Fatalf("BuildEnv: %v", err)
	}
	want := []EnvVar{
		{"ANTHROPIC_AUTH_TOKEN", "sk-x"},
		{"ANTHROPIC_BASE_URL", "https://api.deepseek.com/anthropic"},
		{"ANTHROPIC_DEFAULT_HAIKU_MODEL", "deepseek-v4-flash"},
		{"ANTHROPIC_MODEL", "deepseek-v4-pro"},
		{"CLAUDE_CODE_EFFORT_LEVEL", "max"},
		{"CLAUDE_SWITCH_PROVIDER", "deepseek"},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d vars %+v, want %d", len(got), got, len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("var[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestBuildEnvPrecedence(t *testing.T) {
	// defaults < preset < provider on the same key.
	c := &Config{
		Defaults:  map[string]string{"ANTHROPIC_MODEL": "from-defaults"},
		Providers: map[string]Provider{"p": {"ANTHROPIC_MODEL": "from-provider"}},
	}
	preset := map[string]string{"ANTHROPIC_MODEL": "from-preset"}
	got, _ := c.BuildEnv("p", preset)
	var model string
	for _, v := range got {
		if v.Key == "ANTHROPIC_MODEL" {
			model = v.Value
		}
	}
	if model != "from-provider" {
		t.Errorf("ANTHROPIC_MODEL = %q, want from-provider", model)
	}
}

func TestBuildEnvEmptyOverrideRemoves(t *testing.T) {
	// A provider override set to empty blanks out an inherited preset value.
	c := &Config{Providers: map[string]Provider{
		"p": {"ANTHROPIC_BASE_URL": "https://x", "ANTHROPIC_MODEL": ""},
	}}
	preset := map[string]string{"ANTHROPIC_MODEL": "from-preset"}
	got, _ := c.BuildEnv("p", preset)
	for _, v := range got {
		if v.Key == "ANTHROPIC_MODEL" {
			t.Errorf("ANTHROPIC_MODEL should have been removed by empty override, got %q", v.Value)
		}
	}
}

func TestBuildEnvSkipsUnsafeKey(t *testing.T) {
	c := &Config{Providers: map[string]Provider{
		"p": {"ANTHROPIC_BASE_URL": "https://x", "BAD KEY": "v"},
	}}
	got, _ := c.BuildEnv("p", nil)
	for _, v := range got {
		if v.Key == "BAD KEY" {
			t.Errorf("unsafe key should be skipped: %+v", got)
		}
	}
}

func TestBuildEnvProviderLast(t *testing.T) {
	c := &Config{Providers: map[string]Provider{"p": {"ANTHROPIC_BASE_URL": "https://x"}}}
	got, _ := c.BuildEnv("p", nil)
	last := got[len(got)-1]
	if last.Key != "CLAUDE_SWITCH_PROVIDER" || last.Value != "p" {
		t.Errorf("last var = %+v, want CLAUDE_SWITCH_PROVIDER=p", last)
	}
}

func TestBuildEnvUnknownProvider(t *testing.T) {
	c := &Config{Providers: map[string]Provider{}}
	if _, err := c.BuildEnv("nope", nil); err == nil {
		t.Error("expected error for unknown provider")
	}
}
