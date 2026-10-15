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
// policy sets sign_up: false. That setting only governs self sign-up.
func (s *Services) CreateToken(ctx context.Context, email string, scope core.Scope, name string, ttl time.Duration) (string, core.Token, error) {
	acct, err := s.Accounts.Ensure(ctx, email)
	if err != nil {
		return "", core.Token{}, err
	}
	var expiresAt *time.Time
	if ttl > 0 {
		t := time.Now().Add(ttl)
		expiresAt = &t
	}
	return s.Tokens.Mint(ctx, acct.ID, name, scope, expiresAt)
}

// NoteSignUpBypass writes a note to w when CreateToken would create a new
// account under a policy that disallows sign-up.
func (s *Services) NoteSignUpBypass(ctx context.Context, w io.Writer, email string) error {
	policy, ok := s.Policies.Match(email)
	if !ok || policy.SignUpAllowed() {
		return nil
	}
	exists, err := s.Accounts.Exists(ctx, email)
	if err != nil || exists {
		return err
	}
	_, err = fmt.Fprintf(w, "Note: policy %q sets sign_up: false. Creating the account for %s anyway; minting a token is an operator action.\n",
		policy.Name, core.NormalizeEmail(email))
	return err
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
