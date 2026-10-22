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

func TestBuildEnvResolvesSecret(t *testing.T) {
	t.Setenv("TEST_KEY_VAR", "sk-from-env")
	c := &Config{Providers: map[string]Provider{
		"test-env": {"ANTHROPIC_AUTH_TOKEN": "env:TEST_KEY_VAR"},
	}}
	got, err := c.BuildEnv("test-env", nil)
	if err != nil {
		t.Fatalf("BuildEnv: %v", err)
	}
	var token string
	for _, v := range got {
		if v.Key == "ANTHROPIC_AUTH_TOKEN" {
			token = v.Value
		}
	}
	if token != "sk-from-env" {
		t.Errorf("ANTHROPIC_AUTH_TOKEN = %q, want sk-from-env", token)
	}

	// Missing env var must fail BuildEnv completely so ANTHROPIC_BASE_URL is never half-injected.
	cMissing := &Config{Providers: map[string]Provider{
		"broken-env": {
			"ANTHROPIC_BASE_URL":   "https://third-party.example.com",
			"ANTHROPIC_AUTH_TOKEN": "env:MISSING_SECRET_VAR_12345",
		},
	}}
	if _, err := cMissing.BuildEnv("broken-env", nil); err == nil {
		t.Fatal("expected BuildEnv to fail when env secret reference is unset")
	}
}

func TestWrapBridgeURL(t *testing.T) {
	tests := []struct {
		bridgeBase string
		rawURL     string
		want       string
	}{
		{
			bridgeBase: "http://127.0.0.1:8088",
			rawURL:     "https://wawazz.xyz",
			want:       "http://127.0.0.1:8088/https/wawazz.xyz",
		},
		{
			bridgeBase: "http://127.0.0.1:8088",
			rawURL:     "https://company.ai/v1",
			want:       "http://127.0.0.1:8088/https/company.ai/v1",
		},
		{
			bridgeBase: "http://127.0.0.1:8088",
			rawURL:     "http://localhost:11434",
			want:       "http://127.0.0.1:8088/http/localhost:11434",
		},
		{
			bridgeBase: "http://127.0.0.1:8088",
			rawURL:     "http://127.0.0.1:8088/https/wawazz.xyz",
			want:       "http://127.0.0.1:8088/https/wawazz.xyz", // already wrapped
		},
		{
			bridgeBase: "",
			rawURL:     "https://wawazz.xyz",
			want:       "http://127.0.0.1:8088/https/wawazz.xyz", // default bridge base
		},
	}

	for _, tt := range tests {
		got := WrapBridgeURL(tt.bridgeBase, tt.rawURL)
		if got != tt.want {
			t.Errorf("WrapBridgeURL(%q, %q) = %q, want %q", tt.bridgeBase, tt.rawURL, got, tt.want)
		}
	}
}

func TestBuildEnvOpenAIProtocolWrapsBaseURL(t *testing.T) {
	c := &Config{Providers: map[string]Provider{
		"sol": {
			"ANTHROPIC_AUTH_TOKEN":   "sk-test",
			"ANTHROPIC_BASE_URL":     "https://wawazz.xyz",
			"CLAUDE_SWITCH_PROTOCOL": "openai",
			"ANTHROPIC_MODEL":        "gpt-6.1-sol",
		},
	}}

	got, err := c.BuildEnv("sol", nil)
	if err != nil {
		t.Fatalf("BuildEnv: %v", err)
	}

	var baseURL, proto string
	for _, v := range got {
		if v.Key == "ANTHROPIC_BASE_URL" {
			baseURL = v.Value
		}
		if v.Key == "CLAUDE_SWITCH_PROTOCOL" {
			proto = v.Value
		}
	}

	wantURL := "http://127.0.0.1:8088/https/wawazz.xyz"
	if baseURL != wantURL {
		t.Errorf("ANTHROPIC_BASE_URL = %q, want %q", baseURL, wantURL)
	}
	if proto != "openai" {
		t.Errorf("CLAUDE_SWITCH_PROTOCOL = %q, want openai", proto)
	}
}

func TestBuildEnvCodexPreset(t *testing.T) {
	c := &Config{Providers: map[string]Provider{
		"codex": {},
	}}
	preset := map[string]string{
		"ANTHROPIC_BASE_URL":     "https://chatgpt.com/backend-api/codex",
		"CLAUDE_SWITCH_PROTOCOL": "openai",
	}

	got, err := c.BuildEnv("codex", preset)
	if err != nil {
		t.Fatalf("BuildEnv: %v", err)
	}

	envMap := map[string]string{}
	for _, v := range got {
		envMap[v.Key] = v.Value
	}

	wantURL := "http://127.0.0.1:8088/https/chatgpt.com/backend-api/codex"
	if envMap["ANTHROPIC_BASE_URL"] != wantURL {
		t.Errorf("ANTHROPIC_BASE_URL = %q, want %q", envMap["ANTHROPIC_BASE_URL"], wantURL)
	}
	if envMap["CLAUDE_SWITCH_PROTOCOL"] != "openai" {
		t.Errorf("CLAUDE_SWITCH_PROTOCOL = %q, want openai", envMap["CLAUDE_SWITCH_PROTOCOL"])
	}
	if envMap["ANTHROPIC_AUTH_TOKEN"] != "codex-oauth-session" {
		t.Errorf("ANTHROPIC_AUTH_TOKEN = %q, want codex-oauth-session", envMap["ANTHROPIC_AUTH_TOKEN"])
	}
	if envMap["CLAUDE_CODE_AUTO_COMPACT_WINDOW"] != "1000000" {
		t.Errorf("CLAUDE_CODE_AUTO_COMPACT_WINDOW = %q, want 1000000", envMap["CLAUDE_CODE_AUTO_COMPACT_WINDOW"])
	}
	if envMap["CLAUDE_CODE_MAX_CONTEXT_TOKENS"] != "1000000" {
		t.Errorf("CLAUDE_CODE_MAX_CONTEXT_TOKENS = %q, want 1000000", envMap["CLAUDE_CODE_MAX_CONTEXT_TOKENS"])
	}
}
