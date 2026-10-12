package relay

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
)

const MaxRecipientDevice = 80

type Accounts struct {
	store Store
	now   func() time.Time
	newID func() string

	// mu serializes the check-then-write in SetEncryption and
	// RemoveRecipient, so two concurrent calls cannot leave encryption on
	// with no recipient. The relay is one process over its store.
	mu sync.Mutex
}

func NewAccounts(store Store) *Accounts {
	return &Accounts{store: store, now: time.Now, newID: uuid.NewString}
}

// SetEncryption refuses "on" with no recipients (KindInvalid): sealing to
// nobody would make every delivery unreadable. The switch applies to later
// deliveries only; stored rows keep the form they were written in.
func (a *Accounts) SetEncryption(ctx context.Context, email string, on bool) (Account, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	acct, err := a.account(ctx, email)
	if err != nil {
		return Account{}, err
	}
	if on {
		rs, err := a.store.Recipients().ListByEmail(ctx, email)
		if err != nil {
			return Account{}, fmt.Errorf("list recipients: %w", err)
		}
		if len(rs) == 0 {
			return Account{}, NewError(KindInvalid, "register a recipient before turning encryption on")
		}
	}
	acct.Encryption = on
	if err := a.store.Accounts().Put(ctx, acct); err != nil {
		return Account{}, fmt.Errorf("store account: %w", err)
	}
	return a.account(ctx, email)
}

// AddRecipient registers an age X25519 public key. Registering one already
// stored updates its device and returns the existing row.
func (a *Accounts) AddRecipient(ctx context.Context, email, recipient, device string) (Recipient, error) {
	recipient = strings.TrimSpace(recipient)
	device = strings.TrimSpace(device)
	if _, err := parseRecipient(recipient); err != nil {
		return Recipient{}, NewError(KindInvalid, "recipient is not an age X25519 public key (age1...)")
	}
	if utf8.RuneCountInString(device) > MaxRecipientDevice {
		return Recipient{}, NewError(KindInvalid, fmt.Sprintf("device is at most %d characters", MaxRecipientDevice))
	}
	if err := ensureAccount(ctx, a.store, email, a.now()); err != nil {
		return Recipient{}, err
	}
	err := a.store.Recipients().Put(ctx, Recipient{
		ID:        a.newID(),
		Email:     email,
		Recipient: recipient,
		Device:    device,
		CreatedAt: a.now(),
	})
	if err != nil {
		return Recipient{}, fmt.Errorf("store recipient: %w", err)
	}
	rs, err := a.ListRecipients(ctx, email)
	if err != nil {
		return Recipient{}, err
	}
	for _, r := range rs {
		if r.Recipient == recipient {
			return r, nil
		}
	}
	return Recipient{}, errors.New("stored recipient is missing on read back")
}

func (a *Accounts) ListRecipients(ctx context.Context, email string) ([]Recipient, error) {
	rs, err := a.store.Recipients().ListByEmail(ctx, email)
	if err != nil {
		return nil, fmt.Errorf("list recipients: %w", err)
	}
	return rs, nil
}

// RemoveRecipient is KindNotFound for an unknown id and KindInvalid when it
// would remove the last recipient while encryption is on. Rows already sealed
// to the key stay sealed to it; nothing re-encrypts.
func (a *Accounts) RemoveRecipient(ctx context.Context, email, id string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	rs, err := a.ListRecipients(ctx, email)
	if err != nil {
		return err
	}
	found := false
	for _, r := range rs {
		if r.ID == id {
			found = true
			break
		}
	}
	if !found {
		return NewError(KindNotFound, "recipient not found")
	}
	if len(rs) == 1 {
		acct, err := a.account(ctx, email)
		if err != nil {
			return err
		}
		if acct.Encryption {
			return NewError(KindInvalid, "this is the last recipient; turn encryption off first")
		}
	}
	err = a.store.Recipients().Delete(ctx, email, id)
	if errors.Is(err, ErrNotFound) {
		return NewError(KindNotFound, "recipient not found")
	}
	if err != nil {
		return fmt.Errorf("delete recipient: %w", err)
	}
	return nil
}

func (a *Accounts) account(ctx context.Context, email string) (Account, error) {
	acct, err := a.store.Accounts().Get(ctx, email)
	if errors.Is(err, ErrNotFound) {
		return Account{}, NewError(KindNotFound, "account not found")
	}
	if err != nil {
		return Account{}, fmt.Errorf("look up account: %w", err)
	}
	return acct, nil
}
