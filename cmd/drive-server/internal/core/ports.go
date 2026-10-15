package core

import (
	"context"
	"time"
)

type Account struct {
	ID           string
	Email        string
	CreatedAt    time.Time
	LastSignInAt *time.Time
}

type Token struct {
	ID         string
	UserID     string
	Name       string
	Scope      Scope
	ExpiresAt  *time.Time
	LastUsedAt *time.Time
	CreatedAt  time.Time
}

// AccountStore methods return ErrNotFound for a missing account.
type AccountStore interface {
	// Insert stores a, and does nothing when its email already has an account.
	Insert(ctx context.Context, a Account) error
	GetByEmail(ctx context.Context, email string) (Account, error)
	GetByID(ctx context.Context, id string) (Account, error)
	List(ctx context.Context) ([]Account, error)
	// DeleteByEmail removes the account, and with it everything that
	// references it: tokens, sessions, share links, and pending sign-in codes.
	DeleteByEmail(ctx context.Context, email string) error
}

// TokenStore methods return ErrNotFound for a missing token.
type TokenStore interface {
	Insert(ctx context.Context, t Token, secretHash string) error
	GetBySecretHash(ctx context.Context, secretHash string) (Token, error)
	ListByUser(ctx context.Context, userID string) ([]Token, error)
	Delete(ctx context.Context, userID, id string) error
	Touch(ctx context.Context, id string, at time.Time) error
}
