package app

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/colonyops/hive/cmd/drive-server/internal/core"
)

const day = 24 * time.Hour

// ParseTTL reads "30d" or any time.ParseDuration value. An empty string means
// no expiry and returns zero.
func ParseTTL(s string) (time.Duration, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, nil
	}
	if days, ok := strings.CutSuffix(s, "d"); ok {
		n, err := strconv.Atoi(days)
		if err != nil || n <= 0 {
			return 0, fmt.Errorf("invalid expiry %q: want a positive number of days, like 30d", s)
		}
		return time.Duration(n) * day, nil
	}
	d, err := time.ParseDuration(s)
	if err != nil || d <= 0 {
		return 0, fmt.Errorf("invalid expiry %q: want a duration like 30d or 720h", s)
	}
	return d, nil
}

// CreateToken mints a token for email, creating the account when the email
// matches a policy. A zero ttl means the token never expires.
//
// Minting is an operator action, so it creates the account even when the
// policy sets sign_up: false. That setting only governs self sign-up. When
// it did so, bypassed names the policy; otherwise bypassed is empty.
func (s *Services) CreateToken(ctx context.Context, email string, scope core.Scope, name string, ttl time.Duration) (secret string, token core.Token, bypassed string, err error) {
	acct, created, err := s.Accounts.Ensure(ctx, email, true)
	if err != nil {
		return "", core.Token{}, "", err
	}
	if created {
		if policy, ok := s.Policies.Match(acct.Email); ok && !policy.SignUpAllowed() {
			bypassed = policy.Name
		}
	}
	var expiresAt *time.Time
	if ttl > 0 {
		t := time.Now().Add(ttl)
		expiresAt = &t
	}
	secret, token, err = s.Tokens.Mint(ctx, acct.ID, name, scope, expiresAt)
	return secret, token, bypassed, err
}

// WriteAccounts prints one line per account with the policy that admits it,
// or "locked" when no policy does.
func (s *Services) WriteAccounts(ctx context.Context, w io.Writer) error {
	accts, err := s.Accounts.List(ctx)
	if err != nil {
		return err
	}
	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "EMAIL\tID\tCREATED\tPOLICY")
	for _, a := range accts {
		policy := "locked"
		if p, ok := s.Policies.Match(a.Email); ok {
			policy = p.Name
		}
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", a.Email, a.ID, a.CreatedAt.Format(time.DateOnly), policy)
	}
	return tw.Flush()
}
