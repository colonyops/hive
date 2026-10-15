package core

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/colonyops/hive/pkg/randid"
)

const (
	tokenPrefix = "hds_"
	// touchInterval bounds last_used_at writes, so a busy token does not
	// turn every request into a database write.
	touchInterval = time.Hour
)

type Tokens struct {
	tokens   TokenStore
	accounts AccountStore
	policies *Policies
	now      func() time.Time
}

func NewTokens(tokens TokenStore, accounts AccountStore, policies *Policies, opts ...Option) *Tokens {
	return &Tokens{tokens: tokens, accounts: accounts, policies: policies, now: newOptions(opts).now}
}

// Mint creates a token for userID and returns its secret, which is not
// recoverable afterwards: only its SHA-256 is stored.
func (t *Tokens) Mint(ctx context.Context, userID, name string, scope Scope, expiresAt *time.Time) (string, Token, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", Token{}, NewError(KindInvalid, "token name is required")
	}
	if !scope.IsValid() {
		return "", Token{}, NewError(KindInvalid, fmt.Sprintf("scope must be one of %s", strings.Join(ScopeNames(), ", ")))
	}
	now := t.now()
	if expiresAt != nil && !expiresAt.After(now) {
		return "", Token{}, NewError(KindInvalid, "expiry must be in the future")
	}

	secret, err := newSecret()
	if err != nil {
		return "", Token{}, err
	}
	tok := Token{
		ID:        randid.UUIDv7(),
		UserID:    userID,
		Name:      name,
		Scope:     scope,
		ExpiresAt: expiresAt,
		CreatedAt: now,
	}
	if err := t.tokens.Insert(ctx, tok, hashSecret(secret)); err != nil {
		return "", Token{}, fmt.Errorf("store token: %w", err)
	}
	return secret, tok, nil
}

// Resolve authenticates a token secret. Every failure a caller could cause is
// unauthenticated with the same message, so a response does not say whether a
// secret exists.
func (t *Tokens) Resolve(ctx context.Context, secret string) (Principal, error) {
	if !strings.HasPrefix(secret, tokenPrefix) {
		return Principal{}, errInvalidToken()
	}
	tok, err := t.tokens.GetBySecretHash(ctx, hashSecret(secret))
	if IsNotFound(err) {
		return Principal{}, errInvalidToken()
	}
	if err != nil {
		return Principal{}, fmt.Errorf("look up token: %w", err)
	}

	now := t.now()
	if tok.ExpiresAt != nil && !now.Before(*tok.ExpiresAt) {
		return Principal{}, errInvalidToken()
	}

	acct, err := t.accounts.GetByID(ctx, tok.UserID)
	if IsNotFound(err) {
		return Principal{}, errInvalidToken()
	}
	if err != nil {
		return Principal{}, fmt.Errorf("look up account: %w", err)
	}
	// Checked on every request, so removing a policy locks its accounts out
	// at once without touching their tokens.
	policy, ok := t.policies.Match(acct.Email)
	if !ok {
		return Principal{}, errInvalidToken()
	}

	if tok.LastUsedAt == nil || now.Sub(*tok.LastUsedAt) >= touchInterval {
		if err := t.tokens.Touch(ctx, tok.ID, now); err != nil {
			return Principal{}, fmt.Errorf("touch token: %w", err)
		}
	}
	return Principal{UserID: acct.ID, Email: acct.Email, Scope: tok.Scope, Policy: policy}, nil
}

func (t *Tokens) List(ctx context.Context, userID string) ([]Token, error) {
	toks, err := t.tokens.ListByUser(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("list tokens: %w", err)
	}
	return toks, nil
}

func (t *Tokens) Revoke(ctx context.Context, userID, id string) error {
	err := t.tokens.Delete(ctx, userID, id)
	if IsNotFound(err) {
		return NewError(KindNotFound, "token not found")
	}
	if err != nil {
		return fmt.Errorf("revoke token: %w", err)
	}
	return nil
}

func errInvalidToken() error { return NewError(KindUnauthenticated, "invalid token") }

func newSecret() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate token: %w", err)
	}
	return tokenPrefix + base64.RawURLEncoding.EncodeToString(b), nil
}

func hashSecret(secret string) string {
	sum := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(sum[:])
}
