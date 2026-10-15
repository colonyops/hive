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
	policies *Policies
	now      func() time.Time
}

func NewAccounts(store AccountStore, policies *Policies, opts ...Option) *Accounts {
	return &Accounts{store: store, policies: policies, now: newOptions(opts).now}
}

// Ensure returns the account for email, creating it on first use. An email
// that matches no policy gets no account.
func (a *Accounts) Ensure(ctx context.Context, email string) (Account, error) {
	email = NormalizeEmail(email)
	if email == "" {
		return Account{}, NewError(KindInvalid, "email is required")
	}
	if _, ok := a.policies.Match(email); !ok {
		return Account{}, NewError(KindForbidden, "email matches no user policy")
	}

	acct, err := a.store.GetByEmail(ctx, email)
	if err == nil {
		return acct, nil
	}
	if !IsNotFound(err) {
		return Account{}, fmt.Errorf("look up account: %w", err)
	}

	if err := a.store.Insert(ctx, Account{ID: randid.UUIDv7(), Email: email, CreatedAt: a.now()}); err != nil {
		return Account{}, fmt.Errorf("create account: %w", err)
	}
	// Read back rather than return the inserted value: a concurrent Ensure
	// may have won the insert, and its id is the one that stuck.
	acct, err = a.store.GetByEmail(ctx, email)
	if err != nil {
		return Account{}, fmt.Errorf("read back account: %w", err)
	}
	return acct, nil
}

func (a *Accounts) List(ctx context.Context) ([]Account, error) {
	accts, err := a.store.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("list accounts: %w", err)
	}
	return accts, nil
}

// Delete removes the account's control rows. It does not need a matching
// policy, so a locked account can still be cleaned up.
func (a *Accounts) Delete(ctx context.Context, email string) error {
	err := a.store.DeleteByEmail(ctx, NormalizeEmail(email))
	if errors.Is(err, ErrNotFound) {
		return NewError(KindNotFound, "account not found")
	}
	if err != nil {
		return fmt.Errorf("delete account: %w", err)
	}
	return nil
}
