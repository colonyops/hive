package core

import (
	"context"
	"time"

	"github.com/colonyops/hive/internal/domain/drive"
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

type Document struct {
	ID        string
	Space     string
	Path      string
	Kind      drive.Kind
	SHA256    string
	Size      int64
	UpdatedAt time.Time
}

// NewDocument is what Put stores. ID is used only when the (space, path) row
// does not exist yet; an existing row keeps its id so share links follow a
// re-upload.
type NewDocument struct {
	ID        string
	Space     string
	Path      string
	Kind      drive.Kind
	SHA256    string
	Body      []byte
	UpdatedAt time.Time
}

type SpaceUsage struct {
	Space     string
	Documents int
	Bytes     int64
}

// ErrQuotaExceeded is what DocumentStore.Put returns when the write would
// push the user's stored bytes over the limit.
var ErrQuotaExceeded = NewError(KindTooLarge, "storage quota exceeded")

// DocumentStore is one user's documents. No method takes a user id: the
// handle is the user, so a caller cannot reach another user's rows. Methods
// return ErrNotFound for a missing document.
type DocumentStore interface {
	// Put checks the quota and upserts in one transaction. A nil quota means
	// unlimited.
	Put(ctx context.Context, d NewDocument, quota *int64) (Document, error)
	// Get returns the document and its body.
	Get(ctx context.Context, space, path string) (Document, []byte, error)
	List(ctx context.Context, space string) ([]Document, error)
	Spaces(ctx context.Context) ([]SpaceUsage, error)
	// Delete returns the id of the document it removed.
	Delete(ctx context.Context, space, path string) (string, error)
	GetByID(ctx context.Context, id string) (Document, []byte, error)
	HasID(ctx context.Context, id string) (bool, error)
	// Usage is the sum of all stored body sizes.
	Usage(ctx context.Context) (int64, error)
}

// UserStores hands out per-user DocumentStores.
type UserStores interface {
	// Open returns userID's store, creating an empty one on first use. The
	// caller must call release when done; the store stays valid until then.
	Open(ctx context.Context, userID string) (store DocumentStore, release func(), err error)
	// Remove deletes everything stored for userID, once its leases are released.
	Remove(ctx context.Context, userID string) error
}

// ShareLink is the public link to one document. A document has at most one.
type ShareLink struct {
	ID         string
	Slug       string
	UserID     string
	DocumentID string
	ExpiresAt  *time.Time
	CreatedAt  time.Time
}

// ShareStore methods return ErrNotFound for a missing link.
type ShareStore interface {
	// InsertIfAbsent stores l unless its document already has a link.
	InsertIfAbsent(ctx context.Context, l ShareLink) error
	GetBySlug(ctx context.Context, slug string) (ShareLink, error)
	GetByDocument(ctx context.Context, userID, documentID string) (ShareLink, error)
	ListByUser(ctx context.Context, userID string) ([]ShareLink, error)
	// ListAfter returns up to limit links with an id after afterID, in id order.
	ListAfter(ctx context.Context, afterID string, limit int) ([]ShareLink, error)
	UpdateExpiry(ctx context.Context, id string, expiresAt *time.Time) error
	Delete(ctx context.Context, id string) error
	DeleteByDocument(ctx context.Context, userID, documentID string) error
	DeleteExpired(ctx context.Context, now time.Time) (int, error)
}
