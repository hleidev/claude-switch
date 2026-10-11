package cmd

import (
	"fmt"
	"testing"

	"github.com/hleidev/claude-switch/internal/config"
	"github.com/hleidev/claude-switch/internal/secret"
)

type mockRemoveBackend struct {
	deleted []string
}

func (m *mockRemoveBackend) Get(service, account string) (string, error) {
	return "test", nil
}

func (m *mockRemoveBackend) Set(service, account, secret string) error {
	return nil
}

func (m *mockRemoveBackend) Delete(service, account string) error {
	m.deleted = append(m.deleted, service+"/"+account)
	return nil
}

func (m *mockRemoveBackend) Find(queryArgs ...string) (*secret.KeychainMatch, error) {
	return nil, fmt.Errorf("not implemented")
}

func (m *mockRemoveBackend) IsAvailable() bool {
	return true
}

func TestCleanupKeychainOnRemove(t *testing.T) {
	mock := &mockRemoveBackend{}
	orig := secret.DefaultBackend
	secret.DefaultBackend = mock
	defer func() { secret.DefaultBackend = orig }()

	cfg := &config.Config{
		Providers: map[string]config.Provider{
			"own-item": {
				config.AuthTokenKey: "keychain:own-item",
			},
			"external-item": {
				config.AuthTokenKey: "keychain:other-tool/my-key",
			},
			"borrowed-item": {
				config.AuthTokenKey: "keychain:shared-account",
			},
			"shared-item-1": {
				config.AuthTokenKey: "keychain:shared-item-1",
			},
			"shared-item-2": {
				config.AuthTokenKey: "keychain:shared-item-1",
			},
		},
	}

	// Case 1: External service item -> NEVER deleted
	mock.deleted = nil
	cleanupKeychainOnRemove(cfg, "external-item")
	if len(mock.deleted) != 0 {
		t.Fatalf("expected external item not to be deleted, got %v", mock.deleted)
	}

	// Case 2: Borrowed account from another provider/name -> NEVER deleted
	mock.deleted = nil
	cleanupKeychainOnRemove(cfg, "borrowed-item")
	if len(mock.deleted) != 0 {
		t.Fatalf("expected borrowed account item not to be deleted, got %v", mock.deleted)
	}

	// Case 3: Shared item referenced by another provider -> NOT deleted while another references it
	mock.deleted = nil
	cleanupKeychainOnRemove(cfg, "shared-item-1")
	if len(mock.deleted) != 0 {
		t.Fatalf("expected shared item not to be deleted when referenced elsewhere, got %v", mock.deleted)
	}

	// Case 4: Solely owned item -> successfully deleted
	mock.deleted = nil
	cleanupKeychainOnRemove(cfg, "own-item")
	if len(mock.deleted) != 1 || mock.deleted[0] != "claude-switch/own-item" {
		t.Fatalf("expected own-item to be deleted, got %v", mock.deleted)
	}
}
