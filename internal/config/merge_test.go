package config

import "testing"

func TestBuildEnvMappingAndOrder(t *testing.T) {
	c := &Config{Providers: map[string]Provider{
		"deepseek": {
			BaseURL:    "https://api.deepseek.com/anthropic",
			AuthToken:  "sk-x",
			Model:      "deepseek-v4-pro",
			HaikuModel: "deepseek-v4-flash",
			Env: map[string]string{
				"CLAUDE_CODE_EFFORT_LEVEL":   "max",
				"CLAUDE_CODE_SUBAGENT_MODEL": "deepseek-v4-flash",
			},
		},
	}}
	got, err := c.BuildEnv("deepseek")
	if err != nil {
		t.Fatalf("BuildEnv: %v", err)
	}
	want := []EnvVar{
		{"ANTHROPIC_BASE_URL", "https://api.deepseek.com/anthropic"},
		{"ANTHROPIC_AUTH_TOKEN", "sk-x"},
		{"ANTHROPIC_MODEL", "deepseek-v4-pro"},
		{"ANTHROPIC_DEFAULT_HAIKU_MODEL", "deepseek-v4-flash"},
		{"CLAUDE_CODE_EFFORT_LEVEL", "max"},
		{"CLAUDE_CODE_SUBAGENT_MODEL", "deepseek-v4-flash"},
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

func TestBuildEnvSkipsEmptyTypedFields(t *testing.T) {
	c := &Config{Providers: map[string]Provider{
		"m": {BaseURL: "https://x", Model: "MiniMax-M3"},
	}}
	got, _ := c.BuildEnv("m")
	for _, v := range got {
		if v.Key == "ANTHROPIC_AUTH_TOKEN" || v.Key == "ANTHROPIC_DEFAULT_OPUS_MODEL" {
			t.Errorf("unexpected empty field emitted: %s", v.Key)
		}
	}
}

func TestBuildEnvPrecedence(t *testing.T) {
	// defaults.env < typed < provider.env on the same key.
	c := &Config{
		Defaults: Defaults{Env: map[string]string{"ANTHROPIC_MODEL": "from-defaults"}},
		Providers: map[string]Provider{
			"p": {
				Model: "from-typed",
				Env:   map[string]string{"ANTHROPIC_MODEL": "from-provider-env"},
			},
		},
	}
	got, _ := c.BuildEnv("p")
	var model string
	for _, v := range got {
		if v.Key == "ANTHROPIC_MODEL" {
			model = v.Value
		}
	}
	if model != "from-provider-env" {
		t.Errorf("ANTHROPIC_MODEL = %q, want from-provider-env", model)
	}
}

func TestBuildEnvProviderLast(t *testing.T) {
	c := &Config{Providers: map[string]Provider{"p": {BaseURL: "https://x"}}}
	got, _ := c.BuildEnv("p")
	last := got[len(got)-1]
	if last.Key != "CLAUDE_SWITCH_PROVIDER" || last.Value != "p" {
		t.Errorf("last var = %+v, want CLAUDE_SWITCH_PROVIDER=p", last)
	}
}

func TestBuildEnvUnknownProvider(t *testing.T) {
	c := &Config{Providers: map[string]Provider{}}
	if _, err := c.BuildEnv("nope"); err == nil {
		t.Error("expected error for unknown provider")
	}
}
