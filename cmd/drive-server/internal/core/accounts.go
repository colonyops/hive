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

// Exists reports whether email has an account, whatever the policies say.
func (a *Accounts) Exists(ctx context.Context, email string) (bool, error) {
	_, err := a.store.GetByEmail(ctx, NormalizeEmail(email))
	if IsNotFound(err) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("look up account: %w", err)
	}
	return true, nil
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
