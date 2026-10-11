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
	Find(queryArgs ...string) (*KeychainMatch, error)
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

func (s *systemKeychain) Find(queryArgs ...string) (*KeychainMatch, error) {
	if !s.IsAvailable() {
		return nil, fmt.Errorf("macOS Keychain is only supported on darwin")
	}
	args := append([]string{"find-generic-password"}, queryArgs...)
	cmd := exec.Command("/usr/bin/security", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, err
	}
	text := string(out)
	svce := extractBlob(text, "svce")
	acct := extractBlob(text, "acct")
	if svce == "" || acct == "" {
		return nil, fmt.Errorf("could not parse service or account from keychain output")
	}
	sec, err := s.Get(svce, acct)
	if err != nil {
		return nil, err
	}
	return &KeychainMatch{Service: svce, Account: acct, Secret: sec}, nil
}

func extractBlob(text, key string) string {
	target := `"` + key + `"<blob>="`
	idx := strings.Index(text, target)
	if idx == -1 {
		return ""
	}
	start := idx + len(target)
	end := strings.Index(text[start:], `"`)
	if end == -1 {
		return ""
	}
	return text[start : start+end]
}

// KeychainMatch represents a discovered Keychain item.
type KeychainMatch struct {
	Service string
	Account string
	Secret  string
}

// Ref returns the canonical reference format.
func (m KeychainMatch) Ref() string {
	if m.Service == DefaultService {
		return "keychain:" + m.Account
	}
	return "keychain:" + m.Service + "/" + m.Account
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
	account = strings.TrimPrefix(account, "keychain:")
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

// DetectKeychainSmart performs a tiered scan for an existing Keychain entry:
// 1. Exact match under default service: (claude-switch, provider)
// 2. Service name equals provider: (provider, *)
// 3. Account name equals provider: (*, provider)
// 4. Alternate naming (e.g. hyphen <-> underscore)
// 5. Service name equals baseURL host or root domain (if baseURL provided)
func DetectKeychainSmart(provider, baseURL string) (*KeychainMatch, bool) {
	if !DefaultBackend.IsAvailable() {
		return nil, false
	}
	// 1. Default service + provider account
	if m, err := DefaultBackend.Find("-s", DefaultService, "-a", provider); err == nil && m.Secret != "" {
		return m, true
	}
	// 2. Service equals provider
	if m, err := DefaultBackend.Find("-s", provider); err == nil && m.Secret != "" {
		return m, true
	}
	// 3. Account equals provider
	if m, err := DefaultBackend.Find("-a", provider); err == nil && m.Secret != "" {
		return m, true
	}
	// 4. Alternate naming convention (hyphen <-> underscore)
	altProvider := strings.ReplaceAll(provider, "-", "_")
	if altProvider == provider {
		altProvider = strings.ReplaceAll(provider, "_", "-")
	}
	if altProvider != provider {
		if m, err := DefaultBackend.Find("-s", altProvider); err == nil && m.Secret != "" {
			return m, true
		}
		if m, err := DefaultBackend.Find("-a", altProvider); err == nil && m.Secret != "" {
			return m, true
		}
	}
	// 5. Host domain and root domain from baseURL
	if baseURL != "" {
		host := cleanHost(baseURL)
		if host != "" && host != provider {
			if m, err := DefaultBackend.Find("-s", host); err == nil && m.Secret != "" {
				return m, true
			}
			parts := strings.Split(host, ".")
			if len(parts) > 2 {
				rootDomain := strings.Join(parts[len(parts)-2:], ".")
				if rootDomain != host && rootDomain != provider {
					if m, err := DefaultBackend.Find("-s", rootDomain); err == nil && m.Secret != "" {
						return m, true
					}
				}
			}
		}
	}
	return nil, false
}

func cleanHost(rawURL string) string {
	raw := rawURL
	if idx := strings.Index(raw, "://"); idx != -1 {
		raw = raw[idx+3:]
	}
	if idx := strings.Index(raw, "/"); idx != -1 {
		raw = raw[:idx]
	}
	if idx := strings.Index(raw, ":"); idx != -1 {
		raw = raw[:idx]
	}
	return strings.TrimSpace(raw)
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
