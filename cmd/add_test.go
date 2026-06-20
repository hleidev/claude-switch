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
