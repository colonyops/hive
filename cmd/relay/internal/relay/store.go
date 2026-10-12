package relay

import (
	"context"
	"errors"
	"time"
)

var ErrNotFound = errors.New("relay: not found")

// Store is the persistence port. Every driver (memory, sqlite) implements it
// and passes storetest.Run.
type Store interface {
	Accounts() AccountStore
	Tokens() TokenStore
	Sources() SourceStore
	Deliveries() DeliveryStore
	AuthCodes() AuthCodeStore
	Outbox() OutboxStore
	Recipients() RecipientStore
	Close() error
}

type AccountStore interface {
	Get(ctx context.Context, email string) (Account, error)
	// Put inserts a when absent. For an existing row it updates Encryption
	// only: BytesUsed belongs to Append and AddBytes, CreatedAt to the first
	// insert, so a Get-then-Put from any service can never reset either.
	Put(ctx context.Context, a Account) error
	Delete(ctx context.Context, email string) error // cascades sources, deliveries, tokens, recipients
	List(ctx context.Context) ([]Account, error)
	// AddBytes credits bytes back after a trim or an expiry and returns the
	// new value. Debiting happens inside DeliveryStore.Append, never here.
	AddBytes(ctx context.Context, email string, delta int64) (int64, error)
}

type TokenStore interface {
	GetByHash(ctx context.Context, hash string) (Token, error)
	Put(ctx context.Context, t Token) error
	Delete(ctx context.Context, email, id string) error
	ListByEmail(ctx context.Context, email string) ([]Token, error)
	Touch(ctx context.Context, id string, at time.Time) error
}

type SourceStore interface {
	Get(ctx context.Context, email, id string) (Source, error)
	GetByIngressHash(ctx context.Context, hash string) (Source, error)
	Put(ctx context.Context, s Source) error
	Delete(ctx context.Context, email, id string) error // cascades deliveries
	ListByEmail(ctx context.Context, email string) ([]Source, error)
	CountByEmail(ctx context.Context, email string) (int, error)
}

type DeliveryStore interface {
	// Append stores d and debits d.Size from the owning account inside one
	// critical section (the memory driver's mutex; one sqlite transaction
	// with UPDATE ... RETURNING). It returns KindOverCap, storing nothing,
	// when the account's bytes would pass maxAccountBytes. It then trims the
	// source to keepPerSource newest rows and credits the freed bytes back,
	// so a bounded driver needs no sweeper to stay bounded.
	Append(ctx context.Context, email string, d Delivery, maxAccountBytes int64, keepPerSource int) error
	// ListAfter returns up to limit deliveries with ID > afterID in ID order,
	// and whether more remain. afterID "" means from the beginning.
	ListAfter(ctx context.Context, sourceID, afterID string, limit int) ([]Delivery, bool, error)
	// DeleteBefore and TrimTo credit the freed bytes back to the account
	// themselves and return what they removed.
	DeleteBefore(ctx context.Context, email, sourceID string, before time.Time) (n int, bytes int64, err error)
	TrimTo(ctx context.Context, email, sourceID string, keep int) (n int, bytes int64, err error)
	Stats(ctx context.Context, sourceID string) (count int, bytes int64, err error)
}

type RecipientStore interface {
	// Put inserts r, or for an (Email, Recipient) pair already stored updates
	// Device only, keeping the first ID and CreatedAt: one desktop
	// registering twice is one row.
	Put(ctx context.Context, r Recipient) error
	Delete(ctx context.Context, email, id string) error
	ListByEmail(ctx context.Context, email string) ([]Recipient, error) // CreatedAt then ID order
}

// AuthCodeStore holds at most one pending sign-in code per email.
type AuthCodeStore interface {
	// Put replaces any pending code for email and resets its attempts.
	Put(ctx context.Context, email, hash string, expires time.Time) error
	// Consume checks hash against email's pending code. A match deletes the
	// code and returns ok. A miss increments attempts; after maxAttempts the
	// code is deleted. An expired code is a miss and is deleted. No pending
	// code is a miss with no attempts left.
	Consume(ctx context.Context, email, hash string, now time.Time, maxAttempts int) (ok bool, attemptsLeft int, err error)
	DeleteExpired(ctx context.Context, now time.Time) (int, error)
}

// Mail is one queued message. SentAt is zero until it is sent.
type Mail struct {
	ID        string
	To        string
	Subject   string
	Body      string
	CreatedAt time.Time
	NextAt    time.Time
	Attempts  int
	SentAt    time.Time
}

type OutboxStore interface {
	Append(ctx context.Context, m Mail) error
	// Due returns up to limit unsent mails with NextAt <= now, oldest first.
	Due(ctx context.Context, now time.Time, limit int) ([]Mail, error)
	MarkSent(ctx context.Context, id string, at time.Time) error
	// MarkFailed defers the mail to next and bumps Attempts.
	MarkFailed(ctx context.Context, id string, next time.Time) error
}
