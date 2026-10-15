package core

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/colonyops/hive/internal/domain/drive"
	"github.com/colonyops/hive/internal/platform/driveapi"
	"github.com/colonyops/hive/pkg/randid"
)

type Documents struct {
	users   UserStores
	shares  ShareStore
	maxSize int64
	baseURL string
	now     func() time.Time
}

// NewDocuments returns the document service. A maxDocumentSize of zero or
// less means no cap.
func NewDocuments(users UserStores, shares ShareStore, maxDocumentSize int64, opts ...Option) *Documents {
	o := newOptions(opts)
	return &Documents{users: users, shares: shares, maxSize: maxDocumentSize, baseURL: o.baseURL, now: o.now}
}

// MaxDocumentSize is the largest accepted body, or zero for no cap.
func (d *Documents) MaxDocumentSize() int64 { return max(d.maxSize, 0) }

func (d *Documents) Put(ctx context.Context, p Principal, space drive.Space, path string, body []byte) (driveapi.Document, error) {
	kind, err := writableKind(path)
	if err != nil {
		return driveapi.Document{}, err
	}
	if d.maxSize > 0 && int64(len(body)) > d.maxSize {
		return driveapi.Document{}, NewError(KindTooLarge, fmt.Sprintf("document exceeds the %d byte limit", d.maxSize))
	}
	var quota *int64
	if !p.Policy.Quotas.Limit.Unlimited {
		limit := p.Policy.Quotas.Limit.Bytes
		quota = &limit
	}

	sum := sha256.Sum256(body)
	store, release, err := d.users.Open(ctx, p.UserID)
	if err != nil {
		return driveapi.Document{}, fmt.Errorf("open user store: %w", err)
	}
	defer release()

	doc, err := store.Put(ctx, NewDocument{
		ID:        randid.UUIDv7(),
		Space:     space.Key,
		Path:      path,
		Title:     DeriveTitle(path, kind, body),
		Kind:      kind,
		SHA256:    hex.EncodeToString(sum[:]),
		Body:      body,
		UpdatedAt: d.now(),
	}, quota)
	if err != nil {
		return driveapi.Document{}, fmt.Errorf("put document: %w", err)
	}
	return toWire(doc), nil
}

func (d *Documents) Get(ctx context.Context, p Principal, space drive.Space, path string) (driveapi.Document, []byte, error) {
	if err := drive.ValidatePath(path); err != nil {
		return driveapi.Document{}, nil, invalidPath(err)
	}
	store, release, err := d.users.Open(ctx, p.UserID)
	if err != nil {
		return driveapi.Document{}, nil, fmt.Errorf("open user store: %w", err)
	}
	defer release()

	doc, body, err := store.Get(ctx, space.Key, path)
	if err != nil {
		return driveapi.Document{}, nil, notFoundOr(err, "get document")
	}
	return toWire(doc), body, nil
}

func (d *Documents) List(ctx context.Context, p Principal, space drive.Space) ([]driveapi.Document, error) {
	store, release, err := d.users.Open(ctx, p.UserID)
	if err != nil {
		return nil, fmt.Errorf("open user store: %w", err)
	}
	defer release()

	docs, err := store.List(ctx, space.Key)
	if err != nil {
		return nil, fmt.Errorf("list documents: %w", err)
	}
	links, err := d.shares.ListByUser(ctx, p.UserID)
	if err != nil {
		return nil, fmt.Errorf("list share links: %w", err)
	}
	byDoc := make(map[string]ShareLink, len(links))
	for _, l := range links {
		byDoc[l.DocumentID] = l
	}
	out := make([]driveapi.Document, len(docs))
	for i, doc := range docs {
		out[i] = toWire(doc)
		if l, ok := byDoc[doc.ID]; ok {
			share := toWireShare(d.baseURL, l)
			out[i].Shared = &share
		}
	}
	return out, nil
}

// Search looks across the caller's documents, or one space's when space is
// non-nil. A limit of zero or less returns nothing.
func (d *Documents) Search(ctx context.Context, p Principal, q string, space *drive.Space, limit int) ([]driveapi.SearchHit, error) {
	if strings.TrimSpace(q) == "" {
		return nil, NewError(KindInvalid, "search text is empty")
	}
	store, release, err := d.users.Open(ctx, p.UserID)
	if err != nil {
		return nil, fmt.Errorf("open user store: %w", err)
	}
	defer release()

	key := ""
	if space != nil {
		key = space.Key
	}
	hits, err := store.Search(ctx, q, key, limit)
	if err != nil {
		return nil, fmt.Errorf("search documents: %w", err)
	}
	out := make([]driveapi.SearchHit, len(hits))
	for i, h := range hits {
		out[i] = driveapi.SearchHit{Document: toWire(h.Document), Snippet: h.Snippet}
	}
	return out, nil
}

func (d *Documents) Spaces(ctx context.Context, p Principal) ([]driveapi.SpaceSummary, error) {
	store, release, err := d.users.Open(ctx, p.UserID)
	if err != nil {
		return nil, fmt.Errorf("open user store: %w", err)
	}
	defer release()

	usage, err := store.Spaces(ctx)
	if err != nil {
		return nil, fmt.Errorf("list spaces: %w", err)
	}
	out := make([]driveapi.SpaceSummary, 0, len(usage))
	for _, u := range usage {
		space, err := drive.ParseSpaceKey(u.Space)
		if err != nil {
			return nil, fmt.Errorf("stored space %q: %w", u.Space, err)
		}
		out = append(out, driveapi.SpaceSummary{Space: u.Space, Kind: space.Kind, Documents: u.Documents, Bytes: u.Bytes})
	}
	return out, nil
}

func (d *Documents) Delete(ctx context.Context, p Principal, space drive.Space, path string) error {
	if err := drive.ValidatePath(path); err != nil {
		return invalidPath(err)
	}
	store, release, err := d.users.Open(ctx, p.UserID)
	if err != nil {
		return fmt.Errorf("open user store: %w", err)
	}
	defer release()

	id, err := store.Delete(ctx, space.Key, path)
	if err != nil {
		return notFoundOr(err, "delete document")
	}
	// A failure here leaves a dangling link, which Resolve and the sweeper
	// drop.
	if err := d.shares.DeleteByDocument(ctx, p.UserID, id); err != nil {
		return fmt.Errorf("delete share links: %w", err)
	}
	return nil
}

func (d *Documents) Usage(ctx context.Context, p Principal) (int64, error) {
	store, release, err := d.users.Open(ctx, p.UserID)
	if err != nil {
		return 0, fmt.Errorf("open user store: %w", err)
	}
	defer release()

	used, err := store.Usage(ctx)
	if err != nil {
		return 0, fmt.Errorf("read usage: %w", err)
	}
	return used, nil
}

func writableKind(path string) (drive.Kind, error) {
	if err := drive.ValidatePath(path); err != nil {
		return "", invalidPath(err)
	}
	kind := drive.KindOf(path)
	if kind != drive.KindMarkdown && kind != drive.KindHtml {
		return "", NewError(KindInvalid, "only markdown and html documents can be stored")
	}
	return kind, nil
}

func invalidPath(err error) *Error {
	return &Error{Kind: KindInvalid, Message: "invalid document path", Err: err}
}

func notFoundOr(err error, op string) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, ErrNotFound):
		return NewError(KindNotFound, "document not found")
	default:
		return fmt.Errorf("%s: %w", op, err)
	}
}

func toWire(d Document) driveapi.Document {
	return driveapi.Document{ID: d.ID, Space: d.Space, Path: d.Path, Title: d.Title, Kind: d.Kind, SHA256: d.SHA256, Size: d.Size, UpdatedAt: d.UpdatedAt}
}
