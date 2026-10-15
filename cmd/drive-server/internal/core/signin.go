package core

import (
	"context"
	"crypto/rand"
	"fmt"
	"math/big"
	"net/url"
	"time"

	"github.com/rs/zerolog"

	"github.com/colonyops/hive/pkg/logutils"
	"github.com/colonyops/hive/pkg/randid"
)

const (
	codeDigits      = 8
	codeTTL         = 10 * time.Minute
	codeMaxAttempts = 5
)

type SignIn struct {
	log      zerolog.Logger
	codes    CodeStore
	accounts AccountStore
	outbox   OutboxStore
	policies *Policies
	ensure   *Accounts
	sessions *Sessions
	baseURL  string
	now      func() time.Time
}

func NewSignIn(log zerolog.Logger, codes CodeStore, accounts AccountStore, outbox OutboxStore, policies *Policies, ensure *Accounts, sessions *Sessions, opts ...Option) *SignIn {
	o := newOptions(opts)
	return &SignIn{
		log:      logutils.Component(log, "signin"),
		codes:    codes,
		accounts: accounts,
		outbox:   outbox,
		policies: policies,
		ensure:   ensure,
		sessions: sessions,
		baseURL:  o.baseURL,
		now:      o.now,
	}
}

// Start queues a sign-in code for email. It returns nil whether or not a
// code was queued, so the response never says which emails can sign in.
func (s *SignIn) Start(ctx context.Context, email string) error {
	email = NormalizeEmail(email)
	policy, ok := s.policies.Match(email)
	if !ok {
		s.log.Info().Msg("sign-in code not sent: email matches no user policy")
		return nil
	}
	if !policy.SignUpAllowed() {
		if _, err := s.accounts.GetByEmail(ctx, email); IsNotFound(err) {
			s.log.Info().Str("policy", policy.Name).Msg("sign-in code not sent: policy disallows sign-up and the account does not exist")
			return nil
		} else if err != nil {
			return fmt.Errorf("look up account: %w", err)
		}
	}

	code, err := newCode()
	if err != nil {
		return err
	}
	now := s.now()
	if err := s.codes.Put(ctx, email, hashCode(email, code), now.Add(codeTTL)); err != nil {
		return fmt.Errorf("store sign-in code: %w", err)
	}
	m := Mail{
		ID:      randid.UUIDv7(),
		To:      email,
		Subject: "Your Hive Drive sign-in code",
		Body: fmt.Sprintf("Your sign-in code is:\n\n    %s\n\nIt expires in %d minutes. Enter it at %s/sign-in?email=%s\n\nIf you did not ask for it, ignore this mail.\n",
			code, int(codeTTL.Minutes()), s.baseURL, url.QueryEscape(email)),
		NextAt:    now,
		CreatedAt: now,
	}
	if err := s.outbox.Append(ctx, m); err != nil {
		return fmt.Errorf("queue sign-in mail: %w", err)
	}
	return nil
}

// Verify trades email's pending code for a session and returns the cookie
// value.
func (s *SignIn) Verify(ctx context.Context, email, code string) (string, error) {
	email = NormalizeEmail(email)
	ok, left, err := s.codes.Consume(ctx, email, hashCode(email, code), s.now(), codeMaxAttempts)
	if err != nil {
		return "", fmt.Errorf("check sign-in code: %w", err)
	}
	if !ok {
		if left == 0 {
			return "", NewError(KindUnauthenticated, "invalid or expired code; request a new one")
		}
		return "", NewError(KindUnauthenticated, fmt.Sprintf("invalid code; %d attempts left", left))
	}

	policy, matched := s.policies.Match(email)
	if !matched {
		return "", NewError(KindForbidden, "email matches no user policy")
	}
	if _, err := s.accounts.GetByEmail(ctx, email); IsNotFound(err) {
		if !policy.SignUpAllowed() {
			return "", NewError(KindForbidden, "sign-up is closed for this email")
		}
	} else if err != nil {
		return "", fmt.Errorf("look up account: %w", err)
	}
	acct, err := s.ensure.Ensure(ctx, email)
	if err != nil {
		return "", err
	}
	if err := s.accounts.SetLastSignIn(ctx, acct.ID, s.now()); err != nil {
		return "", fmt.Errorf("record sign-in: %w", err)
	}
	value, _, err := s.sessions.Mint(ctx, acct.ID)
	return value, err
}

func newCode() (string, error) {
	limit := new(big.Int).Exp(big.NewInt(10), big.NewInt(codeDigits), nil)
	n, err := rand.Int(rand.Reader, limit)
	if err != nil {
		return "", fmt.Errorf("generate sign-in code: %w", err)
	}
	return fmt.Sprintf("%0*d", codeDigits, n), nil
}

// hashCode binds the hash to the email: the store keeps code_hash unique, and
// two people can be sent the same 8 digits.
func hashCode(email, code string) string { return hashSecret(email + "\x00" + code) }
