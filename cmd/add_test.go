package cmd

import (
	"testing"

	"github.com/hleidev/claude-switch/internal/config"
)

func TestApplyAddWritesProvider(t *testing.T) {
	cfg := &config.Config{Providers: map[string]config.Provider{}}
	opts := addOptions{
		Name:      "deepseek",
		BaseURL:   "https://api.deepseek.com/anthropic",
		Model:     "deepseek-v4-pro",
		AuthToken: "sk-x",
		Env:       map[string]string{"CLAUDE_CODE_EFFORT_LEVEL": "max"},
	}
	if err := applyAdd(cfg, opts); err != nil {
		t.Fatalf("applyAdd: %v", err)
	}
	p, ok := cfg.Providers["deepseek"]
	if !ok {
		t.Fatal("provider not written")
	}
	if p.BaseURL != opts.BaseURL || p.Model != opts.Model || p.AuthToken != "sk-x" {
		t.Errorf("provider mismatch: %+v", p)
	}
	if p.Env["CLAUDE_CODE_EFFORT_LEVEL"] != "max" {
		t.Errorf("env not written: %+v", p.Env)
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
		"minimax": {BaseURL: "https://old"},
	}}
	opts := addOptions{Name: "minimax", BaseURL: "https://new"}
	if err := applyAdd(cfg, opts); err == nil {
		t.Fatal("expected duplicate without --force to error")
	}
	opts.Force = true
	if err := applyAdd(cfg, opts); err != nil {
		t.Fatalf("applyAdd with force: %v", err)
	}
	if cfg.Providers["minimax"].BaseURL != "https://new" {
		t.Errorf("force did not overwrite: %+v", cfg.Providers["minimax"])
	}
}

func TestApplyAddRequiresBaseURL(t *testing.T) {
	cfg := &config.Config{Providers: map[string]config.Provider{}}
	if err := applyAdd(cfg, addOptions{Name: "x"}); err == nil {
		t.Error("expected error when base_url missing")
	}
}
