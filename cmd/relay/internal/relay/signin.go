package relay

import (
	"context"
	"crypto/rand"
	"fmt"
	"math/big"
	"sync/atomic"
	"time"

	"github.com/colonyops/hive/pkg/randid"
)

const (
	codeDigits      = 8
	codeTTL         = 10 * time.Minute
	codeMaxAttempts = 5
)

// Methods is what GET /v1/auth/methods reports, so a client offers only the
// sign-in methods this relay supports.
type Methods struct {
	EmailCode bool `json:"email_code"`
	Password  bool `json:"password"`
	APIKey    bool `json:"api_key"`
}

// SignIn runs the email-code and password flows. Both end in a device token
// minted the same way as any other.
type SignIn struct {
	store        Store
	auth         *Auth
	allow        *Allowlist
	users        atomic.Pointer[map[string]string]
	outbox       OutboxStore
	publicURL    string
	emailEnabled bool
	now          func() time.Time
}

// NewSignIn returns a SignIn. users maps a lowercased email to its argon2id
// hash. emailEnabled says a mail sender is configured; email codes are offered
// only when it is and the allowlist has a pattern.
func NewSignIn(store Store, allow *Allowlist, users map[string]string, outbox OutboxStore, publicURL string, emailEnabled bool) *SignIn {
	s := &SignIn{
		store:        store,
		auth:         NewAuth(store, nil),
		allow:        allow,
		outbox:       outbox,
		publicURL:    publicURL,
		emailEnabled: emailEnabled,
		now:          time.Now,
	}
	s.SetUsers(users)
	return s
}

func (s *SignIn) SetUsers(users map[string]string) {
	m := make(map[string]string, len(users))
	for email, hash := range users {
		m[NormalizeEmail(email)] = hash
	}
	s.users.Store(&m)
}

func (s *SignIn) Methods() Methods {
	return Methods{
		EmailCode: s.emailEnabled && s.allow.HasPatterns(),
		Password:  len(*s.users.Load()) > 0,
		APIKey:    true,
	}
}

// Start queues a sign-in code for email. A disallowed email is a clear 403,
// not a silent success: the design accepts leaking allowlist membership over
// leaving someone waiting for a code that never comes.
func (s *SignIn) Start(ctx context.Context, email string) error {
	email = NormalizeEmail(email)
	if !s.Methods().EmailCode {
		return NewError(KindForbidden, "email sign-in is not enabled on this relay")
	}
	if !s.allow.Allow(email) {
		return NewError(KindForbidden, "email is not allowed on this relay")
	}
	code, err := newCode()
	if err != nil {
		return err
	}
	now := s.now()
	if err := s.store.AuthCodes().Put(ctx, email, HashToken(code), now.Add(codeTTL)); err != nil {
		return fmt.Errorf("store sign-in code: %w", err)
	}
	m := Mail{
		ID:        randid.UUIDv7(),
		To:        email,
		Subject:   "Your Hive Relay sign-in code",
		Body:      fmt.Sprintf("Your sign-in code for %s is:\n\n    %s\n\nIt expires in %d minutes. If you did not ask for it, ignore this mail.\n", s.publicURL, code, int(codeTTL.Minutes())),
		CreatedAt: now,
		NextAt:    now,
	}
	if err := s.outbox.Append(ctx, m); err != nil {
		return fmt.Errorf("queue sign-in mail: %w", err)
	}
	return nil
}

// Verify trades email's pending code for a device token.
func (s *SignIn) Verify(ctx context.Context, email, code, device string) (string, Token, error) {
	email = NormalizeEmail(email)
	if !s.allow.Allow(email) {
		return "", Token{}, NewError(KindForbidden, "email is not allowed on this relay")
	}
	ok, left, err := s.store.AuthCodes().Consume(ctx, email, HashToken(code), s.now(), codeMaxAttempts)
	if err != nil {
		return "", Token{}, fmt.Errorf("check sign-in code: %w", err)
	}
	if !ok {
		if left == 0 {
			return "", Token{}, NewError(KindUnauthorized, "invalid or expired code; request a new one")
		}
		return "", Token{}, NewError(KindUnauthorized, fmt.Sprintf("invalid code; %d attempts left", left))
	}
	return s.auth.MintToken(ctx, email, device, TokenDevice)
}

// Login trades a listed user's password for a device token.
func (s *SignIn) Login(ctx context.Context, email, password, device string) (string, Token, error) {
	email = NormalizeEmail(email)
	hash, listed := (*s.users.Load())[email]
	if !listed {
		return "", Token{}, NewError(KindForbidden, "password sign-in is not enabled for this email")
	}
	match, err := VerifyPassword(hash, password)
	if err != nil {
		return "", Token{}, fmt.Errorf("verify password for %s: %w", email, err)
	}
	if !match {
		return "", Token{}, NewError(KindUnauthorized, "wrong email or password")
	}
	return s.auth.MintToken(ctx, email, device, TokenDevice)
}

func newCode() (string, error) {
	limit := new(big.Int).Exp(big.NewInt(10), big.NewInt(codeDigits), nil)
	n, err := rand.Int(rand.Reader, limit)
	if err != nil {
		return "", fmt.Errorf("generate sign-in code: %w", err)
	}
	return fmt.Sprintf("%0*d", codeDigits, n), nil
}
