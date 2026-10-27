package core

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/colonyops/hive/pkg/randid"
)

type Accounts struct {
	store    AccountStore
	users    UserStores
	policies *Policies
	now      func() time.Time
}

func NewAccounts(store AccountStore, users UserStores, policies *Policies, opts ...Option) *Accounts {
	return &Accounts{store: store, users: users, policies: policies, now: newOptions(opts).now}
}

// Ensure returns the account for email, creating it on first use, and
// reports whether this call created it. An email that matches no policy gets
// no account. A policy with sign_up: false refuses to create one unless
// operator is set.
func (a *Accounts) Ensure(ctx context.Context, email string, operator bool) (Account, bool, error) {
	email = NormalizeEmail(email)
	if email == "" {
		return Account{}, false, NewError(KindInvalid, "email is required")
	}
	policy, ok := a.policies.Match(email)
	if !ok {
		return Account{}, false, NewError(KindForbidden, "email matches no user policy")
	}

	acct, err := a.store.GetByEmail(ctx, email)
	if err == nil {
		return acct, false, nil
	}
	if !IsNotFound(err) {
		return Account{}, false, fmt.Errorf("look up account: %w", err)
	}
	if !operator && !policy.SignUpAllowed() {
		return Account{}, false, NewError(KindForbidden, "sign-up is closed for this email")
	}

	fresh := Account{ID: randid.UUIDv7(), Email: email, CreatedAt: a.now()}
	if err := a.store.Insert(ctx, fresh); err != nil {
		return Account{}, false, fmt.Errorf("create account: %w", err)
	}
	// Read back rather than return fresh: a concurrent Ensure may have won
	// the insert, and its id is the one that stuck.
	acct, err = a.store.GetByEmail(ctx, email)
	if err != nil {
		return Account{}, false, fmt.Errorf("read back account: %w", err)
	}
	return acct, acct.ID == fresh.ID, nil
}

func (a *Accounts) List(ctx context.Context) ([]Account, error) {
	accts, err := a.store.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("list accounts: %w", err)
	}
	return accts, nil
}

// Delete removes the account's control rows, then its document file. It does
// not need a matching policy, so a locked account can still be cleaned up.
// The order matters: once the control rows are gone no request resolves to
// the user, so nothing recreates the file after Remove.
func (a *Accounts) Delete(ctx context.Context, email string) error {
	email = NormalizeEmail(email)
	acct, err := a.store.GetByEmail(ctx, email)
	if errors.Is(err, ErrNotFound) {
		return NewError(KindNotFound, "account not found")
	}
	if err != nil {
		return fmt.Errorf("look up account: %w", err)
	}
	err = a.store.DeleteByEmail(ctx, email)
	if errors.Is(err, ErrNotFound) {
		return NewError(KindNotFound, "account not found")
	}
	if err != nil {
		return fmt.Errorf("delete account: %w", err)
	}
	if err := a.users.Remove(ctx, acct.ID); err != nil {
		return fmt.Errorf("remove user documents: %w", err)
	}
	return nil
}
