// Package secret provides unified secret storage and dynamic resolution for
// provider credentials. It supports macOS Keychain (encrypted system storage),
// environment variable references, 1Password (op://) references, shell command
// helpers (cmd:), and fallback to legacy plaintext tokens.
package secret

import (
	"context"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

// DefaultService is the Keychain service name used by claude-switch.
const DefaultService = "claude-switch"

// CommandTimeout bounds external secret resolution commands (security, op, sh)
// so a locked vault or hung helper never freezes terminal startup.
var CommandTimeout = 5 * time.Second

// Kind describes the origin or storage type of a secret reference.
type Kind int

const (
	KindEmpty Kind = iota
	KindKeychain
	KindEnv
	KindOnePassword
	KindCommand
	KindPlaintext
)

func (k Kind) String() string {
	switch k {
	case KindKeychain:
		return "keychain"
	case KindEnv:
		return "env"
	case KindOnePassword:
		return "1password"
	case KindCommand:
		return "command"
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

// CommandRunner executes external secret helper commands (op, sh -c).
// Overridable in tests.
var CommandRunner = defaultCommandRunner

func defaultCommandRunner(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdin = nil
	return cmd.Output()
}

type systemKeychain struct{}

func (s *systemKeychain) IsAvailable() bool {
	return runtime.GOOS == "darwin"
}

func (s *systemKeychain) Get(service, account string) (string, error) {
	if !s.IsAvailable() {
		return "", fmt.Errorf("macOS Keychain is only supported on darwin")
	}
	ctx, cancel := context.WithTimeout(context.Background(), CommandTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "/usr/bin/security", "find-generic-password", "-s", service, "-a", account, "-w")
	cmd.Stdin = nil
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("read keychain (%s/%s): %w", service, account, err)
	}
	raw := strings.TrimRight(string(out), "\r\n")
	return decodeSecurityPassword(raw), nil
}

func (s *systemKeychain) Set(service, account, secret string) error {
	if !s.IsAvailable() {
		return fmt.Errorf("macOS Keychain is only supported on darwin")
	}
	ctx, cancel := context.WithTimeout(context.Background(), CommandTimeout)
	defer cancel()
	// -U updates the item if it already exists
	cmd := exec.CommandContext(ctx, "/usr/bin/security", "add-generic-password", "-s", service, "-a", account, "-w", secret, "-U")
	cmd.Stdin = nil
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("write keychain (%s/%s): %w: %s", service, account, err, strings.TrimSpace(string(out)))
	}
	return nil
}

func (s *systemKeychain) Delete(service, account string) error {
	if !s.IsAvailable() {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), CommandTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "/usr/bin/security", "delete-generic-password", "-s", service, "-a", account)
	cmd.Stdin = nil
	_ = cmd.Run() // ignore if item didn't exist
	return nil
}

func (s *systemKeychain) Find(queryArgs ...string) (*KeychainMatch, error) {
	if !s.IsAvailable() {
		return nil, fmt.Errorf("macOS Keychain is only supported on darwin")
	}
	ctx, cancel := context.WithTimeout(context.Background(), CommandTimeout)
	defer cancel()
	args := append([]string{"find-generic-password"}, queryArgs...)
	cmd := exec.CommandContext(ctx, "/usr/bin/security", args...)
	cmd.Stdin = nil
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

// extractBlob parses an attribute value from `security find-generic-password`
// output. It supports both standard quoted strings (`"svce"<blob>="foo"`) and
// hex-encoded blobs (`"svce"<blob>=0x666F6F`) emitted when non-ASCII bytes are present.
func extractBlob(text, key string) string {
	quotedPrefix := `"` + key + `"<blob>="`
	if idx := strings.Index(text, quotedPrefix); idx != -1 {
		start := idx + len(quotedPrefix)
		if end := strings.Index(text[start:], `"`); end != -1 {
			return text[start : start+end]
		}
	}
	hexPrefix := `"` + key + `"<blob>=0x`
	if idx := strings.Index(text, hexPrefix); idx != -1 {
		start := idx + len(hexPrefix)
		end := start
		for end < len(text) && isHexByte(text[end]) {
			end++
		}
		if end > start && (end-start)%2 == 0 {
			if decoded, err := hex.DecodeString(text[start:end]); err == nil {
				return string(decoded)
			}
		}
	}
	return ""
}

// decodeSecurityPassword handles the case where `security find-generic-password -w`
// returns a hex-encoded string because the stored password had a trailing newline
// when pasted into Keychain Access. To avoid corrupting legitimate hex API keys,
// it only decodes when the decoded bytes contain a newline/carriage-return and
// all remaining bytes are printable ASCII.
func decodeSecurityPassword(raw string) string {
	if len(raw) < 4 || len(raw)%2 != 0 {
		return raw
	}
	for i := 0; i < len(raw); i++ {
		if !isHexByte(raw[i]) {
			return raw
		}
	}
	decoded, err := hex.DecodeString(raw)
	if err != nil {
		return raw
	}
	hasNewline := false
	for _, b := range decoded {
		if b == '\n' || b == '\r' {
			hasNewline = true
			continue
		}
		if b < 0x20 || b > 0x7e {
			return raw
		}
	}
	if !hasNewline {
		return raw
	}
	return strings.TrimRight(string(decoded), "\r\n")
}

func isHexByte(b byte) bool {
	return (b >= '0' && b <= '9') || (b >= 'a' && b <= 'f') || (b >= 'A' && b <= 'F')
}

// Mask returns a redacted fingerprint of a secret value (e.g. "sk-cp-...9a2f")
// safe for terminal display in status, doctor, and interactive selection menus.
func Mask(val string) string {
	val = strings.TrimSpace(val)
	n := len(val)
	if n == 0 {
		return ""
	}
	if n <= 8 {
		return "••••"
	}
	if n <= 12 {
		return val[:2] + "..." + val[n-2:]
	}
	prefixLen := 6
	if idx := strings.Index(val, "-"); idx != -1 && idx < 6 {
		if idx2 := strings.Index(val[idx+1:], "-"); idx2 != -1 && idx+1+idx2 < 8 {
			prefixLen = idx + 1 + idx2 + 1
		}
	}
	return val[:prefixLen] + "..." + val[n-4:]
}

// FormatSource returns a human-readable description of a raw secret reference
// without resolving the secret itself (safe for fast zero-I/O commands like `cs list`).
func FormatSource(raw string) string {
	raw = strings.TrimSpace(raw)
	switch DetectKind(raw) {
	case KindKeychain:
		svc, acct := ParseKeychainRef(raw)
		if svc == DefaultService {
			return "keychain"
		}
		return fmt.Sprintf("keychain:%s/%s", svc, acct)
	case KindEnv:
		return raw
	case KindOnePassword:
		return raw
	case KindCommand:
		return raw
	case KindPlaintext:
		return "config"
	default:
		return "none"
	}
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

// DisplayName returns the "service/account" label for UI prompts.
func (m KeychainMatch) DisplayName() string {
	return m.Service + "/" + m.Account
}

// EnvMatch represents a discovered environment variable holding a candidate key.
type EnvMatch struct {
	Name  string
	Value string
}

// Ref returns the canonical dynamic environment reference string.
func (m EnvMatch) Ref() string {
	return "env:" + m.Name
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
	if strings.HasPrefix(raw, "op://") {
		return KindOnePassword
	}
	if strings.HasPrefix(raw, "cmd:") {
		return KindCommand
	}
	return KindPlaintext
}

// ParseKeychainRef parses a "keychain:..." reference into service and account.
// Format can be "keychain:<account>" (using DefaultService) or "keychain:<service>/<account>".
func ParseKeychainRef(ref string) (service, account string) {
	target := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(ref), "keychain:"))
	if idx := strings.Index(target, "/"); idx != -1 {
		return strings.TrimSpace(target[:idx]), strings.TrimSpace(target[idx+1:])
	}
	return DefaultService, target
}

// ParseEnvRef extracts the environment variable name from "env:VAR" or "${VAR}".
func ParseEnvRef(ref string) string {
	ref = strings.TrimSpace(ref)
	if strings.HasPrefix(ref, "env:") {
		return strings.TrimSpace(strings.TrimPrefix(ref, "env:"))
	}
	if strings.HasPrefix(ref, "${") && strings.HasSuffix(ref, "}") {
		return strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(ref, "${"), "}"))
	}
	return ""
}

// KeychainRef creates a canonical keychain reference string for an account.
func KeychainRef(account string) string {
	account = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(account), "keychain:"))
	return "keychain:" + account
}

// isManagedEnvVar reports whether varName is managed/overwritten by claude-switch
// itself on `cs use`, which would cause a self-referential empty lookup.
func isManagedEnvVar(varName string) bool {
	switch varName {
	case "ANTHROPIC_AUTH_TOKEN", "ANTHROPIC_BASE_URL", "CLAUDE_SWITCH_PROVIDER", "CLAUDE_SWITCH_PROTOCOL", "_CS_MANAGED_VARS":
		return true
	default:
		return false
	}
}

// Resolve resolves a raw configuration value to its actual secret value.
// It handles:
// - "keychain:<account>" or "keychain:<service>/<account>" -> fetches from Keychain
// - "env:<VAR>" or "${VAR}" -> fetches from environment
// - "op://<vault>/<item>/<field>" -> fetches via 1Password CLI (`op read`)
// - "cmd:<shell command>" -> executes command via `sh -c` and reads stdout
// - plaintext string -> returned as-is
func Resolve(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil
	}
	switch DetectKind(raw) {
	case KindKeychain:
		service, account := ParseKeychainRef(raw)
		if service == "" || account == "" {
			return "", fmt.Errorf("invalid keychain reference %q (expected keychain:<account> or keychain:<service>/<account>)", raw)
		}
		val, err := DefaultBackend.Get(service, account)
		if err != nil {
			return "", err
		}
		if val == "" {
			return "", fmt.Errorf("keychain item (%s/%s) is empty", service, account)
		}
		return val, nil

	case KindEnv:
		varName := ParseEnvRef(raw)
		if varName == "" {
			return "", fmt.Errorf("invalid env reference %q (expected env:<VAR>)", raw)
		}
		if isManagedEnvVar(varName) {
			return "", fmt.Errorf("cannot reference claude-switch managed variable $%s (use a distinct variable like $ANTHROPIC_API_KEY)", varName)
		}
		val := strings.TrimSpace(os.Getenv(varName))
		if val == "" {
			return "", fmt.Errorf("environment variable $%s is not set", varName)
		}
		return val, nil

	case KindOnePassword:
		if len(strings.TrimPrefix(raw, "op://")) == 0 {
			return "", fmt.Errorf("invalid 1Password reference %q (expected op://vault/item/field)", raw)
		}
		ctx, cancel := context.WithTimeout(context.Background(), CommandTimeout)
		defer cancel()
		out, err := CommandRunner(ctx, "op", "read", raw)
		if err != nil {
			return "", fmt.Errorf("read 1Password (%s): %w", raw, err)
		}
		val := strings.TrimRight(string(out), "\r\n")
		if val == "" {
			return "", fmt.Errorf("1Password item (%s) returned empty value", raw)
		}
		return val, nil

	case KindCommand:
		command := strings.TrimSpace(strings.TrimPrefix(raw, "cmd:"))
		if command == "" {
			return "", fmt.Errorf("empty command in %q (expected cmd:<shell-command>)", raw)
		}
		ctx, cancel := context.WithTimeout(context.Background(), CommandTimeout)
		defer cancel()
		out, err := CommandRunner(ctx, "sh", "-c", command)
		if err != nil {
			return "", fmt.Errorf("run secret command %q: %w", command, err)
		}
		val := strings.TrimRight(string(out), "\r\n")
		if val == "" {
			return "", fmt.Errorf("secret command %q returned empty output", command)
		}
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

// DetectKeychainSmart returns the highest-priority Keychain match for provider.
func DetectKeychainSmart(provider, baseURL string) (*KeychainMatch, bool) {
	matches := DetectKeychainAll(provider, baseURL)
	if len(matches) == 0 {
		return nil, false
	}
	return matches[0], true
}

// DetectKeychainAll performs a tiered scan for existing Keychain entries and
// returns all unique matches (deduplicated by service/account) in priority order:
// 1. Exact match under default service: (claude-switch, provider)
// 2. Service name equals provider: (provider, *)
// 3. Account name equals provider: (*, provider)
// 4. Common env-style variable names (e.g. CODERPLAN_API_KEY)
// 5. Alternate naming (hyphen <-> underscore)
// 6. Service name equals baseURL host or root domain (if baseURL provided)
func DetectKeychainAll(provider, baseURL string) []*KeychainMatch {
	if !DefaultBackend.IsAvailable() {
		return nil
	}
	var results []*KeychainMatch
	seen := map[string]bool{}
	addMatch := func(m *KeychainMatch) {
		if m == nil || m.Secret == "" {
			return
		}
		key := m.Service + "/" + m.Account
		if seen[key] {
			return
		}
		seen[key] = true
		results = append(results, m)
	}

	// 1. Default service + provider account
	if m, err := DefaultBackend.Find("-s", DefaultService, "-a", provider); err == nil {
		addMatch(m)
	}
	// 2. Service equals provider
	if m, err := DefaultBackend.Find("-s", provider); err == nil {
		addMatch(m)
	}
	// 3. Account equals provider
	if m, err := DefaultBackend.Find("-a", provider); err == nil {
		addMatch(m)
	}
	// 4. Common env-style variable names (focused set to minimize subprocess calls)
	upper := strings.ReplaceAll(strings.ToUpper(provider), "-", "_")
	candidates := []string{
		upper + "_API_KEY",
		upper + "_AUTH_TOKEN",
		upper + "_KEY",
	}
	if provider == "anthropic" || provider == "claude" {
		candidates = append(candidates, "ANTHROPIC_API_KEY")
	}
	for _, c := range candidates {
		if c == provider {
			continue
		}
		if m, err := DefaultBackend.Find("-s", c); err == nil {
			addMatch(m)
		}
		if m, err := DefaultBackend.Find("-a", c); err == nil {
			addMatch(m)
		}
	}
	// 5. Alternate naming convention (hyphen <-> underscore)
	altProvider := strings.ReplaceAll(provider, "-", "_")
	if altProvider == provider {
		altProvider = strings.ReplaceAll(provider, "_", "-")
	}
	if altProvider != provider {
		if m, err := DefaultBackend.Find("-s", altProvider); err == nil {
			addMatch(m)
		}
		if m, err := DefaultBackend.Find("-a", altProvider); err == nil {
			addMatch(m)
		}
	}
	// 6. Host domain and root domain from baseURL
	if baseURL != "" {
		host := cleanHost(baseURL)
		if host != "" && host != provider {
			if m, err := DefaultBackend.Find("-s", host); err == nil {
				addMatch(m)
			}
			parts := strings.Split(host, ".")
			if len(parts) > 2 {
				rootDomain := strings.Join(parts[len(parts)-2:], ".")
				if rootDomain != host && rootDomain != provider {
					if m, err := DefaultBackend.Find("-s", rootDomain); err == nil {
						addMatch(m)
					}
				}
			}
		}
	}
	return results
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
// It never inspects ANTHROPIC_AUTH_TOKEN because claude-switch manages that
// variable itself (reading it would capture whatever provider is active in the shell).
func DetectEnv(provider string) (varName, value string, found bool) {
	matches := DetectEnvAll(provider)
	if len(matches) == 0 {
		return "", "", false
	}
	return matches[0].Name, matches[0].Value, true
}

// DetectEnvAll returns all matching environment variables for provider.
func DetectEnvAll(provider string) []EnvMatch {
	cleanName := strings.ReplaceAll(strings.ToUpper(provider), "-", "_")
	candidates := []string{
		cleanName + "_API_KEY",
		cleanName + "_AUTH_TOKEN",
		cleanName + "_KEY",
	}
	if provider == "anthropic" {
		candidates = []string{"ANTHROPIC_API_KEY"}
	}
	var results []EnvMatch
	seen := map[string]bool{}
	for _, c := range candidates {
		if isManagedEnvVar(c) || seen[c] {
			continue
		}
		seen[c] = true
		if val := strings.TrimSpace(os.Getenv(c)); val != "" {
			results = append(results, EnvMatch{Name: c, Value: val})
		}
	}
	return results
}

// SaveToKeychain saves secret into the default Keychain service for the provider.
func SaveToKeychain(provider, secret string) error {
	return DefaultBackend.Set(DefaultService, provider, secret)
}

// DeleteFromKeychain removes the provider's entry from the default Keychain service.
func DeleteFromKeychain(provider string) error {
	return DefaultBackend.Delete(DefaultService, provider)
}
