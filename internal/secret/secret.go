// Package secret provides unified secret storage and dynamic resolution for
// provider credentials. It supports macOS Keychain (encrypted system storage),
// environment variable references, and fallback to legacy plaintext tokens.
package secret

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

// DefaultService is the Keychain service name used by claude-switch.
const DefaultService = "claude-switch"

// Kind describes the origin or storage type of a secret reference.
type Kind int

const (
	KindEmpty Kind = iota
	KindKeychain
	KindEnv
	KindPlaintext
)

func (k Kind) String() string {
	switch k {
	case KindKeychain:
		return "keychain"
	case KindEnv:
		return "env"
	case KindPlaintext:
		return "config"
	default:
		return "none"
	}
}

// Backend defines the Keychain storage interface, allowing test mocking.
type Backend interface {
	Get(service, account string) (string, error)
	Set(service, account, secret string) error
	Delete(service, account string) error
	IsAvailable() bool
}

// DefaultBackend is the active Keychain implementation.
var DefaultBackend Backend = &systemKeychain{}

type systemKeychain struct{}

func (s *systemKeychain) IsAvailable() bool {
	return runtime.GOOS == "darwin"
}

func (s *systemKeychain) Get(service, account string) (string, error) {
	if !s.IsAvailable() {
		return "", fmt.Errorf("macOS Keychain is only supported on darwin")
	}
	cmd := exec.Command("/usr/bin/security", "find-generic-password", "-s", service, "-a", account, "-w")
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("read keychain (%s/%s): %w", service, account, err)
	}
	return strings.TrimRight(string(out), "\r\n"), nil
}

func (s *systemKeychain) Set(service, account, secret string) error {
	if !s.IsAvailable() {
		return fmt.Errorf("macOS Keychain is only supported on darwin")
	}
	// -U updates the item if it already exists
	cmd := exec.Command("/usr/bin/security", "add-generic-password", "-s", service, "-a", account, "-w", secret, "-U")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("write keychain (%s/%s): %w: %s", service, account, err, string(out))
	}
	return nil
}

func (s *systemKeychain) Delete(service, account string) error {
	if !s.IsAvailable() {
		return nil
	}
	cmd := exec.Command("/usr/bin/security", "delete-generic-password", "-s", service, "-a", account)
	_ = cmd.Run() // ignore if item didn't exist
	return nil
}

// DetectKind inspects a raw secret configuration value and reports its kind.
func DetectKind(raw string) Kind {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return KindEmpty
	}
	if strings.HasPrefix(raw, "keychain:") {
		return KindKeychain
	}
	if strings.HasPrefix(raw, "env:") {
		return KindEnv
	}
	if strings.HasPrefix(raw, "${") && strings.HasSuffix(raw, "}") {
		return KindEnv
	}
	return KindPlaintext
}

// ParseKeychainRef parses a "keychain:..." reference into service and account.
// Format can be "keychain:<account>" (using DefaultService) or "keychain:<service>/<account>".
func ParseKeychainRef(ref string) (service, account string) {
	target := strings.TrimPrefix(ref, "keychain:")
	if idx := strings.Index(target, "/"); idx != -1 {
		return target[:idx], target[idx+1:]
	}
	return DefaultService, target
}

// KeychainRef creates a canonical keychain reference string for an account.
func KeychainRef(account string) string {
	return "keychain:" + account
}

// Resolve resolves a raw configuration value to its actual secret value.
// It handles:
// - "keychain:<account>" or "keychain:<service>/<account>" -> fetches from Keychain
// - "env:<VAR>" or "${VAR}" -> fetches from environment
// - plaintext string -> returned as-is
func Resolve(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil
	}
	switch DetectKind(raw) {
	case KindKeychain:
		service, account := ParseKeychainRef(raw)
		val, err := DefaultBackend.Get(service, account)
		if err != nil {
			return "", err
		}
		return val, nil
	case KindEnv:
		var varName string
		if strings.HasPrefix(raw, "env:") {
			varName = strings.TrimPrefix(raw, "env:")
		} else {
			varName = strings.TrimSuffix(strings.TrimPrefix(raw, "${"), "}")
		}
		val := os.Getenv(varName)
		return val, nil
	default:
		return raw, nil
	}
}

// DetectKeychain searches the Keychain for an existing item under the provider name.
func DetectKeychain(provider string) (string, bool) {
	if !DefaultBackend.IsAvailable() {
		return "", false
	}
	val, err := DefaultBackend.Get(DefaultService, provider)
	if err == nil && val != "" {
		return val, true
	}
	return "", false
}

// DetectEnv checks if a relevant environment variable for provider exists.
func DetectEnv(provider string) (varName, value string, found bool) {
	cleanName := strings.ReplaceAll(strings.ToUpper(provider), "-", "_")
	candidates := []string{
		cleanName + "_API_KEY",
		cleanName + "_AUTH_TOKEN",
	}
	if provider == "anthropic" {
		candidates = append(candidates, "ANTHROPIC_AUTH_TOKEN", "ANTHROPIC_API_KEY")
	}
	for _, c := range candidates {
		if val := os.Getenv(c); val != "" {
			return c, val, true
		}
	}
	return "", "", false
}

// SaveToKeychain saves secret into the default Keychain service for the provider.
func SaveToKeychain(provider, secret string) error {
	return DefaultBackend.Set(DefaultService, provider, secret)
}

// DeleteFromKeychain removes the provider's entry from the default Keychain service.
func DeleteFromKeychain(provider string) error {
	return DefaultBackend.Delete(DefaultService, provider)
}
