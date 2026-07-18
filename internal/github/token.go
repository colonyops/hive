package github

import (
	"errors"
	"fmt"
	"os"

	"github.com/zalando/go-keyring"
)

// EnvToken is the environment override for the GitHub token. It takes
// precedence over the keychain so headless environments (CI, server builds)
// never need keychain access.
const EnvToken = "HIVE_GITHUB_TOKEN"

const (
	keyringService = "sh.hive.desktop"
	keyringAccount = "github-token"
)

// TokenStore persists the GitHub token. Token returns "" (no error) when no
// token is stored.
type TokenStore interface {
	Token() (string, error)
	SetToken(token string) error
	DeleteToken() error
}

// KeychainStore stores the token in the OS keychain, with the EnvToken
// environment variable as a read-only override.
type KeychainStore struct{}

func NewKeychainStore() *KeychainStore {
	return &KeychainStore{}
}

func (s *KeychainStore) Token() (string, error) {
	if token := os.Getenv(EnvToken); token != "" {
		return token, nil
	}
	token, err := keyring.Get(keyringService, keyringAccount)
	if errors.Is(err, keyring.ErrNotFound) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("github: read token from keychain: %w", err)
	}
	return token, nil
}

func (s *KeychainStore) SetToken(token string) error {
	if err := keyring.Set(keyringService, keyringAccount, token); err != nil {
		return fmt.Errorf("github: store token in keychain: %w", err)
	}
	return nil
}

func (s *KeychainStore) DeleteToken() error {
	err := keyring.Delete(keyringService, keyringAccount)
	if err != nil && !errors.Is(err, keyring.ErrNotFound) {
		return fmt.Errorf("github: delete token from keychain: %w", err)
	}
	return nil
}

// MemoryTokenStore is an in-memory TokenStore for tests and mock mode.
type MemoryTokenStore struct {
	token string
}

func NewMemoryTokenStore(token string) *MemoryTokenStore {
	return &MemoryTokenStore{token: token}
}

func (s *MemoryTokenStore) Token() (string, error)      { return s.token, nil }
func (s *MemoryTokenStore) SetToken(token string) error { s.token = token; return nil }
func (s *MemoryTokenStore) DeleteToken() error          { s.token = ""; return nil }
