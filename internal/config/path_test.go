package config

import "testing"

func newConfigWith(p Provider) *Config {
	return &Config{Providers: map[string]Provider{"p": p}}
}

func TestSetFieldByEnvName(t *testing.T) {
	c := newConfigWith(Provider{})
	if err := c.SetField("p", "ANTHROPIC_MODEL", "MiniMax-M3"); err != nil {
		t.Fatalf("SetField: %v", err)
	}
	if c.Providers["p"]["ANTHROPIC_MODEL"] != "MiniMax-M3" {
		t.Errorf("ANTHROPIC_MODEL = %q", c.Providers["p"]["ANTHROPIC_MODEL"])
	}
}

func TestSetFieldKeyAlias(t *testing.T) {
	c := newConfigWith(Provider{})
	if err := c.SetField("p", "key", "sk-x"); err != nil {
		t.Fatalf("SetField: %v", err)
	}
	if c.Providers["p"].AuthToken() != "sk-x" {
		t.Errorf("key alias did not set %s: %+v", AuthTokenKey, c.Providers["p"])
	}
}

func TestSetFieldUnknownProvider(t *testing.T) {
	c := &Config{Providers: map[string]Provider{}}
	if err := c.SetField("nope", "ANTHROPIC_MODEL", "x"); err == nil {
		t.Error("expected error for unknown provider")
	}
}

func TestSetFieldOnNilProviderMap(t *testing.T) {
	// A provider whose map is nil (e.g. an empty table) must still accept a set.
	c := &Config{Providers: map[string]Provider{"p": nil}}
	if err := c.SetField("p", "ANTHROPIC_MODEL", "x"); err != nil {
		t.Fatalf("SetField on nil provider map: %v", err)
	}
	if c.Providers["p"]["ANTHROPIC_MODEL"] != "x" {
		t.Errorf("value not set: %+v", c.Providers["p"])
	}
}

func TestUnsetField(t *testing.T) {
	c := newConfigWith(Provider{"ANTHROPIC_MODEL": "x"})
	if err := c.UnsetField("p", "ANTHROPIC_MODEL"); err != nil {
		t.Fatalf("UnsetField: %v", err)
	}
	if _, ok := c.Providers["p"]["ANTHROPIC_MODEL"]; ok {
		t.Error("ANTHROPIC_MODEL still present")
	}
}

func TestUnsetFieldKeyAlias(t *testing.T) {
	c := newConfigWith(Provider{"ANTHROPIC_AUTH_TOKEN": "sk-x"})
	if err := c.UnsetField("p", "key"); err != nil {
		t.Fatalf("UnsetField: %v", err)
	}
	if c.Providers["p"].AuthToken() != "" {
		t.Error("key alias did not unset the token")
	}
}

func TestUnsetFieldMissingKeyErrors(t *testing.T) {
	c := newConfigWith(Provider{"K": "v"})
	if err := c.UnsetField("p", "ANTHROPIC_MODEL"); err == nil {
		t.Error("expected error unsetting a non-existent variable")
	}
}

func TestSetFieldRejectsUnsafeName(t *testing.T) {
	c := newConfigWith(Provider{})
	for _, bad := range []string{"FOO; rm -rf ~", "HAS SPACE", "", "1LEADING_DIGIT", "a-b"} {
		if err := c.SetField("p", bad, "x"); err == nil {
			t.Errorf("expected SetField to reject unsafe name %q", bad)
		}
	}
}

func TestSetFieldAcceptsValidName(t *testing.T) {
	c := newConfigWith(Provider{})
	if err := c.SetField("p", "API_TIMEOUT_MS", "1"); err != nil {
		t.Errorf("valid name rejected: %v", err)
	}
}
