package relay

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/colonyops/hive/pkg/randid"
)

const (
	touchInterval = time.Hour
	// touchCacheSize bounds the per-token touch memory. Eviction only costs
	// an extra Touch write, so which entry goes does not matter.
	touchCacheSize = 10_000
)

type Auth struct {
	store Store
	allow func(email string) bool
	now   func() time.Time

	mu      sync.Mutex
	touched map[string]time.Time
}

// NewAuth returns an Auth over store. allow is the allowlist and user-list
// check run on every request; nil allows every email.
func NewAuth(store Store, allow func(email string) bool) *Auth {
	if allow == nil {
		allow = func(string) bool { return true }
	}
	return &Auth{
		store:   store,
		allow:   allow,
		now:     time.Now,
		touched: map[string]time.Time{},
	}
}

// Authenticate resolves a bearer value. A hash miss is KindUnauthorized; an
// email the allow func rejects is KindForbidden (the kill switch).
func (a *Auth) Authenticate(ctx context.Context, bearer string) (Account, Token, error) {
	if bearer == "" {
		return Account{}, Token{}, NewError(KindUnauthorized, "missing token")
	}
	tok, err := a.store.Tokens().GetByHash(ctx, HashToken(bearer))
	if errors.Is(err, ErrNotFound) {
		return Account{}, Token{}, NewError(KindUnauthorized, "invalid token")
	}
	if err != nil {
		return Account{}, Token{}, fmt.Errorf("look up token: %w", err)
	}
	if !a.allow(tok.Email) {
		return Account{}, Token{}, NewError(KindForbidden, "email is not allowed on this relay")
	}
	acct, err := a.store.Accounts().Get(ctx, tok.Email)
	if errors.Is(err, ErrNotFound) {
		return Account{}, Token{}, NewError(KindUnauthorized, "invalid token")
	}
	if err != nil {
		return Account{}, Token{}, fmt.Errorf("look up account: %w", err)
	}

	now := a.now()
	if a.dueForTouch(tok, now) {
		err := a.store.Tokens().Touch(ctx, tok.ID, now)
		if errors.Is(err, ErrNotFound) {
			a.forget(tok.ID)
			return Account{}, Token{}, NewError(KindUnauthorized, "invalid token")
		}
		if err != nil {
			a.forget(tok.ID)
			return Account{}, Token{}, fmt.Errorf("touch token: %w", err)
		}
		tok.LastUsedAt = now
	}
	return acct, tok, nil
}

// dueForTouch reports whether tok's LastUsedAt should be written, and records
// now as its last touch when it is, so concurrent requests write once.
func (a *Auth) dueForTouch(tok Token, now time.Time) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	last, ok := a.touched[tok.ID]
	if !ok || tok.LastUsedAt.After(last) {
		last = tok.LastUsedAt
	}
	if !last.IsZero() && now.Sub(last) < touchInterval {
		a.remember(tok.ID, last)
		return false
	}
	a.remember(tok.ID, now)
	return true
}

func (a *Auth) remember(id string, at time.Time) {
	if _, ok := a.touched[id]; !ok && len(a.touched) >= touchCacheSize {
		for k := range a.touched {
			delete(a.touched, k)
			break
		}
	}
	a.touched[id] = at
}

func (a *Auth) forget(id string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	delete(a.touched, id)
}

// MintToken creates a token for email, creating the account if needed, and
// returns the plaintext once.
func (a *Auth) MintToken(ctx context.Context, email, name string, kind TokenKind) (string, Token, error) {
	email = NormalizeEmail(email)
	if email == "" {
		return "", Token{}, NewError(KindInvalid, "email is required")
	}
	switch kind {
	case TokenDevice, TokenAPI:
	default:
		return "", Token{}, NewError(KindInvalid, fmt.Sprintf("unknown token kind %q", kind))
	}

	now := a.now()
	if err := ensureAccount(ctx, a.store, email, now); err != nil {
		return "", Token{}, err
	}
	plaintext, err := NewToken()
	if err != nil {
		return "", Token{}, err
	}
	tok := Token{
		ID:        randid.UUIDv7(),
		Email:     email,
		Hash:      HashToken(plaintext),
		Kind:      kind,
		Device:    strings.TrimSpace(name),
		CreatedAt: now,
	}
	if err := a.store.Tokens().Put(ctx, tok); err != nil {
		return "", Token{}, fmt.Errorf("store token: %w", err)
	}
	return plaintext, tok, nil
}

func (a *Auth) ListTokens(ctx context.Context, email string) ([]Token, error) {
	toks, err := a.store.Tokens().ListByEmail(ctx, NormalizeEmail(email))
	if err != nil {
		return nil, fmt.Errorf("list tokens: %w", err)
	}
	return toks, nil
}

func (a *Auth) RevokeToken(ctx context.Context, email, id string) error {
	err := a.store.Tokens().Delete(ctx, NormalizeEmail(email), id)
	if errors.Is(err, ErrNotFound) {
		return NewError(KindNotFound, "token not found")
	}
	if err != nil {
		return fmt.Errorf("delete token: %w", err)
	}
	a.forget(id)
	return nil
}

func NormalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

// ensureAccount creates email's account when it has none.
func ensureAccount(ctx context.Context, store Store, email string, now time.Time) error {
	_, err := store.Accounts().Get(ctx, email)
	if err == nil {
		return nil
	}
	if !errors.Is(err, ErrNotFound) {
		return fmt.Errorf("look up account: %w", err)
	}
	if err := store.Accounts().Put(ctx, Account{Email: email, CreatedAt: now}); err != nil {
		return fmt.Errorf("create account: %w", err)
	}
	return nil
}

// HashToken is the at-rest form of a bearer, ingress token, or secret.
func HashToken(plaintext string) string {
	sum := sha256.Sum256([]byte(plaintext))
	return hex.EncodeToString(sum[:])
}

// NewToken returns 32 random bytes, base64url without padding.
func NewToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
