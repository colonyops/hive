package core

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"time"

	"github.com/colonyops/hive/pkg/randid"
)

const (
	// SessionTTL is how long a session lives past its last use.
	SessionTTL = 30 * 24 * time.Hour
	// slideInterval bounds expiry writes, so a busy session does not turn
	// every request into a database write.
	slideInterval = time.Hour
)

type Sessions struct {
	store    SessionStore
	accounts AccountStore
	policies *Policies
	now      func() time.Time
}

func NewSessions(store SessionStore, accounts AccountStore, policies *Policies, opts ...Option) *Sessions {
	return &Sessions{store: store, accounts: accounts, policies: policies, now: newOptions(opts).now}
}

// Mint starts a session for userID and returns the cookie value, which is not
// recoverable afterwards: only its SHA-256 is stored.
func (s *Sessions) Mint(ctx context.Context, userID string) (string, Session, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", Session{}, fmt.Errorf("generate session: %w", err)
	}
	value := base64.RawURLEncoding.EncodeToString(b)
	now := s.now()
	sess := Session{ID: randid.UUIDv7(), UserID: userID, ExpiresAt: now.Add(SessionTTL), CreatedAt: now}
	if err := s.store.Insert(ctx, sess, hashSecret(value)); err != nil {
		return "", Session{}, fmt.Errorf("store session: %w", err)
	}
	return value, sess, nil
}

// Resolve authenticates a cookie value. Like a token, it re-checks the
// policy on every request, and every caller-caused failure is the same
// unauthenticated error.
func (s *Sessions) Resolve(ctx context.Context, value string) (Principal, error) {
	sess, err := s.store.GetByValueHash(ctx, hashSecret(value))
	if IsNotFound(err) {
		return Principal{}, errInvalidSession()
	}
	if err != nil {
		return Principal{}, fmt.Errorf("look up session: %w", err)
	}
	now := s.now()
	if !now.Before(sess.ExpiresAt) {
		return Principal{}, errInvalidSession()
	}

	acct, err := s.accounts.GetByID(ctx, sess.UserID)
	if IsNotFound(err) {
		return Principal{}, errInvalidSession()
	}
	if err != nil {
		return Principal{}, fmt.Errorf("look up account: %w", err)
	}
	policy, ok := s.policies.Match(acct.Email)
	if !ok {
		return Principal{}, errInvalidSession()
	}

	if next := now.Add(SessionTTL); next.Sub(sess.ExpiresAt) >= slideInterval {
		if err := s.store.SetExpiry(ctx, sess.ID, next); err != nil {
			return Principal{}, fmt.Errorf("extend session: %w", err)
		}
	}
	return Principal{UserID: acct.ID, Email: acct.Email, Scope: ScopePush, Policy: policy, ViaSession: true}, nil
}

// Revoke ends the session; an unknown value is not an error.
func (s *Sessions) Revoke(ctx context.Context, value string) error {
	if err := s.store.DeleteByValueHash(ctx, hashSecret(value)); err != nil {
		return fmt.Errorf("revoke session: %w", err)
	}
	return nil
}

func errInvalidSession() error { return NewError(KindUnauthenticated, "invalid session") }
