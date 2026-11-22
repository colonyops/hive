package core

import (
	"context"
	"time"

	"github.com/colonyops/hive/internal/domain/drive"
)

type Account struct {
	ID           string
	Email        string
	DisplayName  string
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
	// GetByIDs returns the accounts that exist among ids, in no order.
	GetByIDs(ctx context.Context, ids []string) ([]Account, error)
	SetDisplayName(ctx context.Context, id, name string) error
	SetLastSignIn(ctx context.Context, id string, at time.Time) error
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
	Title     string
	Kind      drive.Kind
	SHA256    string
	Size      int64
	UpdatedAt time.Time
}

// NewDocument is what Put stores. ID is used only when the (space, path) row
// does not exist yet; an existing row keeps its id so share links follow a
// re-upload.
type NewDocument struct {
	ID         string
	Space      string
	Path       string
	Title      string
	Kind       drive.Kind
	SHA256     string
	Body       []byte
	SearchText string
	UpdatedAt  time.Time
}

// SearchHit is one match. Snippet wraps each matched run in
// driveapi.SnippetMatchStart and driveapi.SnippetMatchEnd.
type SearchHit struct {
	Document Document
	Snippet  string
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
	Get(ctx context.Context, space, path string) (Document, []byte, error)
	List(ctx context.Context, space string) ([]Document, error)
	Recent(ctx context.Context, limit int) ([]Document, error)
	// ListByIDs returns the documents that exist among ids, newest first.
	ListByIDs(ctx context.Context, ids []string) ([]Document, error)
	Spaces(ctx context.Context) ([]SpaceUsage, error)
	Delete(ctx context.Context, space, path string) (string, error)
	GetByID(ctx context.Context, id string) (Document, []byte, error)
	// Search matches q as plain terms, the last one as a prefix, in one space
	// or in all of them when space is empty. Title and path matches rank above
	// body matches.
	Search(ctx context.Context, q, space string, limit int) ([]SearchHit, error)
	// Usage is the sum of all stored body sizes.
	Usage(ctx context.Context) (int64, error)
}

type UserStores interface {
	// Open returns userID's store, creating an empty one on first use. The
	// caller must call release when done; the store stays valid until then.
	Open(ctx context.Context, userID string) (store DocumentStore, release func(), err error)
	// OpenThreads is Open for the threads in the same file.
	OpenThreads(ctx context.Context, userID string) (store ThreadStore, release func(), err error)
	// Remove deletes everything stored for userID, once its leases are released.
	Remove(ctx context.Context, userID string) error
}

// Actor is who made a change, as a thread or comment keeps it. ViaToken is
// the API token's name, or empty for a browser session.
type Actor struct {
	UserID   string
	Email    string
	ViaToken string
}

// ThreadAnchor is the part of a thread that re-anchoring reads and writes.
type ThreadAnchor struct {
	ThreadID  string
	StartLine int
	EndLine   int
	Quote     string
	Outdated  bool
}

type NewThread struct {
	ID         string
	DocumentID string
	// Anchor is nil for a general thread; its ThreadID is ignored.
	Anchor    *ThreadAnchor
	Author    Actor
	CommentID string
	Body      string
	CreatedAt time.Time
}

type NewComment struct {
	ID        string
	ThreadID  string
	Author    Actor
	Body      string
	CreatedAt time.Time
}

// StoredThread.Comments are oldest first, tombstones included.
type StoredThread struct {
	ID         string
	DocumentID string
	Anchor     *ThreadAnchor
	Author     Actor
	ResolvedAt *time.Time
	ResolvedBy *Actor
	CreatedAt  time.Time
	Comments   []StoredComment
}

type StoredComment struct {
	ID        string
	ThreadID  string
	Author    Actor
	Body      string
	CreatedAt time.Time
	EditedAt  *time.Time
	DeletedAt *time.Time
}

// CommentCaps bound what other people can write into an owner's file.
type CommentCaps struct {
	ThreadsPerDocument   int
	CommentsPerThread    int
	BodyBytesPerDocument int64
}

// ErrVersionChanged is what ThreadStore.InsertThread returns when the
// document is no longer at the version the caller rendered.
var ErrVersionChanged = NewError(KindConflict, "the document changed; reload it and try again")

// ThreadStore is one user's threads and comments. Like DocumentStore it
// takes no user id. Methods return ErrNotFound for a missing thread,
// comment, or document.
type ThreadStore interface {
	// InsertThread checks, in its transaction, that the document is at
	// wantSHA and that caps hold, then stores the thread and its first
	// comment. The anchor is recorded as checked against wantSHA.
	InsertThread(ctx context.Context, t NewThread, wantSHA string, caps CommentCaps) error
	// InsertComment checks caps in its transaction.
	InsertComment(ctx context.Context, c NewComment, caps CommentCaps) error
	GetThread(ctx context.Context, id string) (StoredThread, error)
	ListThreads(ctx context.Context, documentID string) ([]StoredThread, error)
	GetComment(ctx context.Context, id string) (StoredComment, error)
	// UpdateComment returns ErrNotFound for a deleted comment.
	UpdateComment(ctx context.Context, id, body string, at time.Time) error
	// DeleteComment leaves a tombstone when id is the thread's first comment
	// and others remain, and removes the thread with its last visible one.
	DeleteComment(ctx context.Context, id string, at time.Time) (threadDeleted bool, err error)
	// SetResolved resolves the thread when by is non-nil, else reopens it.
	SetResolved(ctx context.Context, id string, by *Actor, at time.Time) error
	// AnchoredAt returns the document's anchored threads still checked
	// against sha.
	AnchoredAt(ctx context.Context, documentID, sha string) ([]ThreadAnchor, error)
	// MoveAnchors writes anchors and records them as checked against toSHA.
	// It skips a thread no longer checked against fromSHA.
	MoveAnchors(ctx context.Context, documentID, fromSHA, toSHA string, anchors []ThreadAnchor) error
}

// ShareLink is the public link to one document. A document has at most one.
type ShareLink struct {
	ID         string
	Slug       string
	UserID     string
	DocumentID string
	ExpiresAt  *time.Time
	// ExpiresIn is the preset that set ExpiresAt, or empty for no expiry.
	ExpiresIn string
	CreatedAt time.Time
}

// ShareStore methods return ErrNotFound for a missing link.
type ShareStore interface {
	// InsertIfAbsent stores l unless its document already has a link.
	InsertIfAbsent(ctx context.Context, l ShareLink) error
	GetBySlug(ctx context.Context, slug string) (ShareLink, error)
	GetByDocument(ctx context.Context, userID, documentID string) (ShareLink, error)
	ListByUser(ctx context.Context, userID string) ([]ShareLink, error)
	UpdateExpiry(ctx context.Context, id string, expiresAt *time.Time, expiresIn string) error
	Delete(ctx context.Context, id string) error
	DeleteByDocument(ctx context.Context, userID, documentID string) error
	DeleteExpired(ctx context.Context, now time.Time) (int, error)
}

// CodeStore holds at most one pending sign-in code per email.
type CodeStore interface {
	Put(ctx context.Context, email, codeHash string, expiresAt time.Time) error
	// Consume checks codeHash against email's pending code. A match burns the
	// code and returns ok. A miss counts an attempt; the code burns on the
	// maxAttempts-th miss. left is the attempts remaining, and 0 when no live
	// code remains.
	Consume(ctx context.Context, email, codeHash string, now time.Time, maxAttempts int) (ok bool, left int, err error)
	DeleteExpired(ctx context.Context, now time.Time) (int, error)
}

type Session struct {
	ID        string
	UserID    string
	ExpiresAt time.Time
	CreatedAt time.Time
}

// SessionStore methods return ErrNotFound for a missing session.
type SessionStore interface {
	Insert(ctx context.Context, s Session, valueHash string) error
	GetByValueHash(ctx context.Context, valueHash string) (Session, error)
	SetExpiry(ctx context.Context, id string, expiresAt time.Time) error
	// DeleteByValueHash does nothing when the session is already gone.
	DeleteByValueHash(ctx context.Context, valueHash string) error
	DeleteExpired(ctx context.Context, now time.Time) (int, error)
}

type Mail struct {
	ID        string
	To        string
	Subject   string
	Body      string
	Attempts  int
	NextAt    time.Time
	CreatedAt time.Time
}

type OutboxStore interface {
	Append(ctx context.Context, m Mail) error
	// Due returns up to limit mails whose NextAt is not after now, oldest first.
	Due(ctx context.Context, now time.Time, limit int) ([]Mail, error)
	// MarkSent removes m: the outbox holds only mail still to send.
	MarkSent(ctx context.Context, id string) error
	MarkFailed(ctx context.Context, id string, nextAt time.Time) error
}
