package config

import "testing"

func newConfigWith(p Provider) *Config {
	return &Config{Providers: map[string]Provider{"p": p}}
}

func TestSetFieldTyped(t *testing.T) {
	c := newConfigWith(Provider{})
	if err := c.SetField("p", "model", "MiniMax-M3"); err != nil {
		t.Fatalf("SetField: %v", err)
	}
	if c.Providers["p"].Model != "MiniMax-M3" {
		t.Errorf("Model = %q", c.Providers["p"].Model)
	}
}

func TestSetFieldEnv(t *testing.T) {
	c := newConfigWith(Provider{})
	if err := c.SetField("p", "env.API_TIMEOUT_MS", "3000000"); err != nil {
		t.Fatalf("SetField: %v", err)
	}
	if c.Providers["p"].Env["API_TIMEOUT_MS"] != "3000000" {
		t.Errorf("Env = %+v", c.Providers["p"].Env)
	}
}

func TestSetFieldUnknownBareErrors(t *testing.T) {
	c := newConfigWith(Provider{})
	if err := c.SetField("p", "bogus", "x"); err == nil {
		t.Error("expected error for unknown bare field")
	}
}

func TestSetFieldUnknownProvider(t *testing.T) {
	c := &Config{Providers: map[string]Provider{}}
	if err := c.SetField("nope", "model", "x"); err == nil {
		t.Error("expected error for unknown provider")
	}
}

func TestUnsetFieldTyped(t *testing.T) {
	c := newConfigWith(Provider{Model: "x"})
	if err := c.UnsetField("p", "model"); err != nil {
		t.Fatalf("UnsetField: %v", err)
	}
	if c.Providers["p"].Model != "" {
		t.Errorf("Model = %q, want empty", c.Providers["p"].Model)
	}
}

func TestUnsetFieldEnv(t *testing.T) {
	c := newConfigWith(Provider{Env: map[string]string{"K": "v"}})
	if err := c.UnsetField("p", "env.K"); err != nil {
		t.Fatalf("UnsetField: %v", err)
	}
	if _, ok := c.Providers["p"].Env["K"]; ok {
		t.Error("env key K still present")
	}
}

func TestUnsetFieldEmptyEnvKeyErrors(t *testing.T) {
	c := newConfigWith(Provider{Env: map[string]string{"K": "v"}})
	if err := c.UnsetField("p", "env."); err == nil {
		t.Error("expected error for empty env key")
	}
}

func TestUnsetFieldMissingEnvKeyErrors(t *testing.T) {
	c := newConfigWith(Provider{Env: map[string]string{"K": "v"}})
	if err := c.UnsetField("p", "env.NOPE"); err == nil {
		t.Error("expected error unsetting a non-existent env key")
	}
}

func TestSetFieldRejectsUnsafeEnvKey(t *testing.T) {
	c := newConfigWith(Provider{})
	for _, bad := range []string{"FOO; rm -rf ~", "HAS SPACE", "", "1LEADING_DIGIT", "a-b"} {
		if err := c.SetField("p", "env."+bad, "x"); err == nil {
			t.Errorf("expected SetField to reject unsafe env key %q", bad)
		}
	}
}

func TestSetFieldAcceptsValidEnvKey(t *testing.T) {
	c := newConfigWith(Provider{})
	if err := c.SetField("p", "env.API_TIMEOUT_MS", "1"); err != nil {
		t.Errorf("valid env key rejected: %v", err)
	}
}
