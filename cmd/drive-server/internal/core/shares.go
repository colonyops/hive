package core

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"time"

	"github.com/colonyops/hive/internal/domain/drive"
	"github.com/colonyops/hive/internal/platform/driveapi"
	"github.com/colonyops/hive/pkg/randid"
)

const slugBytes = 16

var shareLifetimes = map[string]time.Duration{
	"1d":  24 * time.Hour,
	"7d":  7 * 24 * time.Hour,
	"30d": 30 * 24 * time.Hour,
}

type Shares struct {
	links    ShareStore
	accounts AccountStore
	users    UserStores
	policies *Policies
	baseURL  string
	now      func() time.Time
}

func NewShares(links ShareStore, accounts AccountStore, users UserStores, policies *Policies, opts ...Option) *Shares {
	o := newOptions(opts)
	return &Shares{links: links, accounts: accounts, users: users, policies: policies, baseURL: o.baseURL, now: o.now}
}

// Create returns the document's share link, making one when it has none. A
// preset expiresIn ("1d", "7d", "30d") or driveapi.ShareExpiryNone sets the
// expiry of a new or existing link; an empty one leaves an existing link's
// expiry as it is.
func (s *Shares) Create(ctx context.Context, p Principal, space drive.Space, path, expiresIn string) (driveapi.Share, error) {
	var expiresAt *time.Time
	preset := expiresIn
	switch expiresIn {
	case "":
	case driveapi.ShareExpiryNone:
		preset = ""
	default:
		lifetime, ok := shareLifetimes[expiresIn]
		if !ok {
			return driveapi.Share{}, NewError(KindInvalid, `expires_in must be "", "none", "1d", "7d", or "30d"`)
		}
		t := s.now().Add(lifetime)
		expiresAt = &t
	}
	doc, err := s.document(ctx, p, space, path)
	if err != nil {
		return driveapi.Share{}, err
	}

	slug, err := newSlug()
	if err != nil {
		return driveapi.Share{}, err
	}
	err = s.links.InsertIfAbsent(ctx, ShareLink{
		ID:         randid.UUIDv7(),
		Slug:       slug,
		UserID:     p.UserID,
		DocumentID: doc.ID,
		ExpiresAt:  expiresAt,
		ExpiresIn:  preset,
		CreatedAt:  s.now(),
	})
	if err != nil {
		return driveapi.Share{}, fmt.Errorf("create share link: %w", err)
	}
	link, err := s.links.GetByDocument(ctx, p.UserID, doc.ID)
	if err != nil {
		return driveapi.Share{}, fmt.Errorf("read back share link: %w", err)
	}
	if expiresIn != "" {
		if err := s.links.UpdateExpiry(ctx, link.ID, expiresAt, preset); err != nil {
			return driveapi.Share{}, fmt.Errorf("update share expiry: %w", err)
		}
		link.ExpiresAt, link.ExpiresIn = expiresAt, preset
	}
	return toWireShare(s.baseURL, link), nil
}

// Revoke removes the document's link. A document with no link is not an error.
func (s *Shares) Revoke(ctx context.Context, p Principal, space drive.Space, path string) error {
	doc, err := s.document(ctx, p, space, path)
	if err != nil {
		return err
	}
	if err := s.links.DeleteByDocument(ctx, p.UserID, doc.ID); err != nil {
		return fmt.Errorf("revoke share link: %w", err)
	}
	return nil
}

// Resolve returns the shared document for slug, with its owner. Every reason
// a link stops working is the same not_found, so a caller cannot tell a
// revoked link from a guessed one.
func (s *Shares) Resolve(ctx context.Context, slug string) (Account, driveapi.Document, []byte, error) {
	notFound := NewError(KindNotFound, "link not found")

	link, err := s.links.GetBySlug(ctx, slug)
	if IsNotFound(err) {
		return Account{}, driveapi.Document{}, nil, notFound
	}
	if err != nil {
		return Account{}, driveapi.Document{}, nil, fmt.Errorf("get share link: %w", err)
	}
	if link.ExpiresAt != nil && !s.now().Before(*link.ExpiresAt) {
		return Account{}, driveapi.Document{}, nil, notFound
	}

	owner, err := s.accounts.GetByID(ctx, link.UserID)
	if IsNotFound(err) {
		return Account{}, driveapi.Document{}, nil, notFound
	}
	if err != nil {
		return Account{}, driveapi.Document{}, nil, fmt.Errorf("get link owner: %w", err)
	}
	if _, ok := s.policies.Match(owner.Email); !ok {
		return Account{}, driveapi.Document{}, nil, notFound
	}

	store, release, err := s.users.Open(ctx, link.UserID)
	if err != nil {
		return Account{}, driveapi.Document{}, nil, fmt.Errorf("open user store: %w", err)
	}
	defer release()

	doc, body, err := store.GetByID(ctx, link.DocumentID)
	if IsNotFound(err) {
		return Account{}, driveapi.Document{}, nil, notFound
	}
	if err != nil {
		return Account{}, driveapi.Document{}, nil, fmt.Errorf("get shared document: %w", err)
	}
	return owner, toWire(doc), body, nil
}

func (s *Shares) document(ctx context.Context, p Principal, space drive.Space, path string) (Document, error) {
	if err := drive.ValidatePath(path); err != nil {
		return Document{}, invalidPath(err)
	}
	store, release, err := s.users.Open(ctx, p.UserID)
	if err != nil {
		return Document{}, fmt.Errorf("open user store: %w", err)
	}
	defer release()

	doc, _, err := store.Get(ctx, space.Key, path)
	if err != nil {
		return Document{}, notFoundOr(err, "get document")
	}
	return doc, nil
}

func newSlug() (string, error) {
	b := make([]byte, slugBytes)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate slug: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func toWireShare(baseURL string, l ShareLink) driveapi.Share {
	return driveapi.Share{URL: baseURL + "/s/" + l.Slug, ExpiresAt: l.ExpiresAt, ExpiresIn: l.ExpiresIn}
}
