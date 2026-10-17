package cmd

import (
	"testing"

	"github.com/hleidev/claude-switch/internal/config"
)

func TestApplyAddPresetWritesKeyOnly(t *testing.T) {
	cfg := &config.Config{Providers: map[string]config.Provider{}}
	// deepseek is a built-in preset, so no base_url is required.
	if err := applyAdd(cfg, addOptions{Name: "deepseek", Key: "sk-x"}); err != nil {
		t.Fatalf("applyAdd: %v", err)
	}
	p, ok := cfg.Providers["deepseek"]
	if !ok {
		t.Fatal("provider not written")
	}
	if p.AuthToken() != "sk-x" {
		t.Errorf("token not written: %+v", p)
	}
	if _, hasURL := p[config.BaseURLKey]; hasURL {
		t.Errorf("preset add should not materialize base_url: %+v", p)
	}
	if len(p) != 1 {
		t.Errorf("preset provider should hold only the key, got %+v", p)
	}
}

func TestApplyAddCustomRequiresBaseURL(t *testing.T) {
	cfg := &config.Config{Providers: map[string]config.Provider{}}
	// "mycustom" is not a preset, so base_url is required.
	if err := applyAdd(cfg, addOptions{Name: "mycustom", Key: "sk"}); err == nil {
		t.Error("expected error when a custom provider has no base_url")
	}
	if err := applyAdd(cfg, addOptions{Name: "mycustom", Key: "sk", BaseURL: "https://x"}); err != nil {
		t.Fatalf("applyAdd custom: %v", err)
	}
	p := cfg.Providers["mycustom"]
	if p.BaseURL() != "https://x" || p.AuthToken() != "sk" {
		t.Errorf("custom provider mismatch: %+v", p)
	}
}

func TestApplyAddRejectsClaude(t *testing.T) {
	cfg := &config.Config{Providers: map[string]config.Provider{}}
	if err := applyAdd(cfg, addOptions{Name: "claude", BaseURL: "https://x"}); err == nil {
		t.Error("expected applyAdd to reject claude")
	}
}

func TestApplyAddDuplicateRequiresForce(t *testing.T) {
	cfg := &config.Config{Providers: map[string]config.Provider{
		"minimax": {config.AuthTokenKey: "old"},
	}}
	opts := addOptions{Name: "minimax", Key: "new"}
	if err := applyAdd(cfg, opts); err == nil {
		t.Fatal("expected duplicate without --force to error")
	}
	opts.Force = true
	if err := applyAdd(cfg, opts); err != nil {
		t.Fatalf("applyAdd with force: %v", err)
	}
	if cfg.Providers["minimax"].AuthToken() != "new" {
		t.Errorf("force did not overwrite: %+v", cfg.Providers["minimax"])
	}
}

func TestApplyAddCoderplanPreset(t *testing.T) {
	cfg := &config.Config{Providers: map[string]config.Provider{}}
	if err := applyAdd(cfg, addOptions{Name: "coderplan", Key: "keychain:coderplan"}); err != nil {
		t.Fatalf("applyAdd coderplan: %v", err)
	}
	p, ok := cfg.Providers["coderplan"]
	if !ok {
		t.Fatal("coderplan not added")
	}
	if p.AuthTokenRaw() != "keychain:coderplan" {
		t.Errorf("AuthTokenRaw = %q, want keychain:coderplan", p.AuthTokenRaw())
	}
}

func TestApplyAddKeychainAndEnvRefs(t *testing.T) {
	cfg := &config.Config{Providers: map[string]config.Provider{}}
	if err := applyAdd(cfg, addOptions{Name: "deepseek", Key: "keychain:my-deepseek"}); err != nil {
		t.Fatalf("applyAdd: %v", err)
	}
	if cfg.Providers["deepseek"].AuthTokenRaw() != "keychain:my-deepseek" {
		t.Errorf("AuthTokenRaw = %q, want keychain:my-deepseek", cfg.Providers["deepseek"].AuthTokenRaw())
	}

	if err := applyAdd(cfg, addOptions{Name: "glm", Key: "env:GLM_KEY"}); err != nil {
		t.Fatalf("applyAdd: %v", err)
	}
	if cfg.Providers["glm"].AuthTokenRaw() != "env:GLM_KEY" {
		t.Errorf("AuthTokenRaw = %q, want env:GLM_KEY", cfg.Providers["glm"].AuthTokenRaw())
	}

	if err := applyAdd(cfg, addOptions{Name: "custom-tool", Key: "keychain:other-tool/admin-key", BaseURL: "https://api.example.com"}); err != nil {
		t.Fatalf("applyAdd custom keychain ref: %v", err)
	}
	if cfg.Providers["custom-tool"].AuthTokenRaw() != "keychain:other-tool/admin-key" {
		t.Errorf("AuthTokenRaw = %q, want keychain:other-tool/admin-key", cfg.Providers["custom-tool"].AuthTokenRaw())
	}
}

func TestApplyAddOpenAIProtocolDefaultModels(t *testing.T) {
	cfg := &config.Config{Providers: map[string]config.Provider{}}
	opts := addOptions{
		Name:     "wawazz",
		Protocol: "openai",
		BaseURL:  "https://wawazz.xyz",
		Key:      "sk-test",
	}
	if err := applyAdd(cfg, opts); err != nil {
		t.Fatalf("applyAdd: %v", err)
	}
	p, ok := cfg.Providers["wawazz"]
	if !ok {
		t.Fatal("provider wawazz not created")
	}
	if p.Protocol() != config.ProtocolOpenAI {
		t.Errorf("Protocol = %q, want openai", p.Protocol())
	}
	if p.BaseURL() != "https://wawazz.xyz" {
		t.Errorf("BaseURL = %q, want https://wawazz.xyz", p.BaseURL())
	}

	// Verify tier mappings: Opus -> gpt-6.1-sol[1m], Sonnet -> gpt-6-sol[1m], Haiku -> gpt-6-luna[1m], Fable -> gpt-6-astra[1m]
	if p[config.OpusModelKey] != "gpt-6.1-sol[1m]" {
		t.Errorf("Opus = %q, want gpt-6.1-sol[1m]", p[config.OpusModelKey])
	}
	if p[config.SonnetModelKey] != "gpt-6-sol[1m]" {
		t.Errorf("Sonnet = %q, want gpt-6-sol[1m]", p[config.SonnetModelKey])
	}
	if p[config.HaikuModelKey] != "gpt-6-luna[1m]" {
		t.Errorf("Haiku = %q, want gpt-6-luna[1m]", p[config.HaikuModelKey])
	}
	if p[config.SubagentModelKey] != "gpt-6-luna[1m]" {
		t.Errorf("Subagent = %q, want gpt-6-luna[1m]", p[config.SubagentModelKey])
	}
	if p[config.FableModelKey] != "gpt-6-astra[1m]" {
		t.Errorf("Fable = %q, want gpt-6-astra[1m]", p[config.FableModelKey])
	}
	if p[config.ModelKey] != "gpt-6-sol[1m]" {
		t.Errorf("Model = %q, want gpt-6-sol[1m]", p[config.ModelKey])
	}
	if p[config.AutoCompactWindowKey] != "1000000" {
		t.Errorf("AutoCompactWindow = %q, want 1000000", p[config.AutoCompactWindowKey])
	}
}

func TestApplyAddOpenAIProtocolModelOverride(t *testing.T) {
	cfg := &config.Config{Providers: map[string]config.Provider{}}
	opts := addOptions{
		Name:     "work-relay",
		Protocol: "openai",
		BaseURL:  "https://api.work.ai/v1",
		Model:    "o3-mini",
		Key:      "sk-test",
	}
	if err := applyAdd(cfg, opts); err != nil {
		t.Fatalf("applyAdd: %v", err)
	}
	p := cfg.Providers["work-relay"]
	if p.Protocol() != config.ProtocolOpenAI {
		t.Errorf("Protocol = %q, want openai", p.Protocol())
	}
	// All roles should be overridden to o3-mini
	for _, role := range []string{config.ModelKey, config.SonnetModelKey, config.HaikuModelKey, config.OpusModelKey, config.FableModelKey} {
		if p[role] != "o3-mini" {
			t.Errorf("role %q = %q, want o3-mini", role, p[role])
		}
	}
}

func TestApplyAddOpenAIProtocolSpecificOverrides(t *testing.T) {
	cfg := &config.Config{Providers: map[string]config.Provider{}}
	opts := addOptions{
		Name:       "tiered-relay",
		Protocol:   "openai",
		BaseURL:    "https://api.work.ai",
		Model:      "gpt-6.1-sol",
		HaikuModel: "gpt-5-mini",
		OpusModel:  "o3-high",
		Key:        "sk-test",
	}
	if err := applyAdd(cfg, opts); err != nil {
		t.Fatalf("applyAdd: %v", err)
	}
	p := cfg.Providers["tiered-relay"]
	if p[config.SonnetModelKey] != "gpt-6.1-sol" {
		t.Errorf("Sonnet = %q, want gpt-6.1-sol", p[config.SonnetModelKey])
	}
	if p[config.HaikuModelKey] != "gpt-5-mini" {
		t.Errorf("Haiku = %q, want gpt-5-mini", p[config.HaikuModelKey])
	}
	if p[config.OpusModelKey] != "o3-high" {
		t.Errorf("Opus = %q, want o3-high", p[config.OpusModelKey])
	}
}
