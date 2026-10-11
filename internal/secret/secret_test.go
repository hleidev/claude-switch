package secret

import (
	"fmt"
	"os"
	"testing"
)

type mockBackend struct {
	items     map[string]string
	available bool
}

func (m *mockBackend) key(service, account string) string {
	return service + "/" + account
}

func (m *mockBackend) Get(service, account string) (string, error) {
	if !m.available {
		return "", fmt.Errorf("mock unavailable")
	}
	val, ok := m.items[m.key(service, account)]
	if !ok {
		return "", fmt.Errorf("item not found")
	}
	return val, nil
}

func (m *mockBackend) Set(service, account, secret string) error {
	if !m.available {
		return fmt.Errorf("mock unavailable")
	}
	m.items[m.key(service, account)] = secret
	return nil
}

func (m *mockBackend) Delete(service, account string) error {
	delete(m.items, m.key(service, account))
	return nil
}

func (m *mockBackend) IsAvailable() bool {
	return m.available
}

func TestDetectKind(t *testing.T) {
	tests := []struct {
		input string
		want  Kind
	}{
		{"", KindEmpty},
		{"   ", KindEmpty},
		{"keychain:coderplan", KindKeychain},
		{"keychain:service/account", KindKeychain},
		{"env:CODERPLAN_KEY", KindEnv},
		{"${CODERPLAN_KEY}", KindEnv},
		{"sk-ant-api03-xxx", KindPlaintext},
		{"random-token", KindPlaintext},
	}
	for _, tt := range tests {
		if got := DetectKind(tt.input); got != tt.want {
			t.Errorf("DetectKind(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestParseKeychainRef(t *testing.T) {
	s, a := ParseKeychainRef("keychain:coderplan")
	if s != DefaultService || a != "coderplan" {
		t.Errorf("ParseKeychainRef default = (%q, %q), want (%q, %q)", s, a, DefaultService, "coderplan")
	}

	s, a = ParseKeychainRef("keychain:CustomService/my-account")
	if s != "CustomService" || a != "my-account" {
		t.Errorf("ParseKeychainRef custom = (%q, %q), want (%q, %q)", s, a, "CustomService", "my-account")
	}
}

func TestResolve(t *testing.T) {
	origBackend := DefaultBackend
	defer func() { DefaultBackend = origBackend }()

	mock := &mockBackend{
		items: map[string]string{
			"claude-switch/coderplan": "sk-coderplan-secret",
			"CustomSvc/customAcc":     "sk-custom-secret",
		},
		available: true,
	}
	DefaultBackend = mock

	// Test 1: Keychain default
	got, err := Resolve("keychain:coderplan")
	if err != nil || got != "sk-coderplan-secret" {
		t.Fatalf("Resolve keychain:coderplan failed: got %q, err %v", got, err)
	}

	// Test 2: Keychain custom
	got, err = Resolve("keychain:CustomSvc/customAcc")
	if err != nil || got != "sk-custom-secret" {
		t.Fatalf("Resolve keychain:CustomSvc/customAcc failed: got %q, err %v", got, err)
	}

	// Test 3: Keychain missing item
	_, err = Resolve("keychain:missing")
	if err == nil {
		t.Fatal("expected error for missing keychain item")
	}

	// Test 4: Env ref
	os.Setenv("TEST_RESOLVE_VAR", "sk-env-token")
	defer os.Unsetenv("TEST_RESOLVE_VAR")

	got, err = Resolve("env:TEST_RESOLVE_VAR")
	if err != nil || got != "sk-env-token" {
		t.Fatalf("Resolve env:TEST_RESOLVE_VAR failed: got %q, err %v", got, err)
	}

	got, err = Resolve("${TEST_RESOLVE_VAR}")
	if err != nil || got != "sk-env-token" {
		t.Fatalf("Resolve ${TEST_RESOLVE_VAR} failed: got %q, err %v", got, err)
	}

	// Test 5: Plaintext
	got, err = Resolve("sk-plain-text")
	if err != nil || got != "sk-plain-text" {
		t.Fatalf("Resolve plaintext failed: got %q, err %v", got, err)
	}

	// Test 6: Empty
	got, err = Resolve("")
	if err != nil || got != "" {
		t.Fatalf("Resolve empty failed: got %q, err %v", got, err)
	}
}

func TestDetectKeychain(t *testing.T) {
	origBackend := DefaultBackend
	defer func() { DefaultBackend = origBackend }()

	mock := &mockBackend{
		items: map[string]string{
			"claude-switch/glm": "sk-glm-123",
		},
		available: true,
	}
	DefaultBackend = mock

	val, found := DetectKeychain("glm")
	if !found || val != "sk-glm-123" {
		t.Errorf("DetectKeychain glm = (%q, %v), want (sk-glm-123, true)", val, found)
	}

	_, found = DetectKeychain("deepseek")
	if found {
		t.Error("DetectKeychain deepseek should not be found")
	}
}

func TestDetectEnv(t *testing.T) {
	os.Setenv("CODERPLAN_API_KEY", "sk-detected")
	defer os.Unsetenv("CODERPLAN_API_KEY")

	varName, val, found := DetectEnv("coderplan")
	if !found || varName != "CODERPLAN_API_KEY" || val != "sk-detected" {
		t.Errorf("DetectEnv coderplan = (%q, %q, %v), want (CODERPLAN_API_KEY, sk-detected, true)", varName, val, found)
	}

	_, _, found = DetectEnv("nonexistent-provider")
	if found {
		t.Error("expected DetectEnv for nonexistent-provider to be false")
	}
}
