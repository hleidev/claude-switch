package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
)

func TestShellenvLoadFailureEmitsNoCode(t *testing.T) {
	for _, tc := range []struct {
		name   string
		config string
	}{
		{"invalid_config", "version = ["},
		{"missing_secret", "version = 2\ndefault_provider = 'test'\n[providers.test]\nANTHROPIC_AUTH_TOKEN = 'env:CS_TEST_MISSING_KEY'\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			t.Setenv("XDG_CONFIG_HOME", dir)
			t.Setenv("CS_TEST_MISSING_KEY", "")
			if err := os.Mkdir(filepath.Join(dir, "claude-switch"), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, "claude-switch", "config.toml"), []byte(tc.config), 0o600); err != nil {
				t.Fatal(err)
			}
			previous := shellenvAnnounce
			t.Cleanup(func() { shellenvAnnounce = previous })
			for _, announce := range []bool{true, false} {
				shellenvAnnounce = announce
				var output, errOutput bytes.Buffer
				command := &cobra.Command{}
				command.SetOut(&output)
				command.SetErr(&errOutput)
				err := shellenvCmd.RunE(command, nil)
				if announce && err == nil {
					t.Fatal("explicit switch must report the load error")
				}
				if !announce && err != nil {
					t.Fatalf("startup must remain non-fatal: %v", err)
				}
				if output.Len() != 0 {
					t.Fatal("failed load emitted shell code")
				}
			}
		})
	}
}
