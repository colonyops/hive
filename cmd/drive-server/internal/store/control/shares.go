package control

import (
	"context"
	"time"

	"github.com/colonyops/hive/cmd/drive-server/internal/core"
	"github.com/colonyops/hive/cmd/drive-server/internal/store/control/db"
)

type shares struct{ s *Store }

func (sh shares) InsertIfAbsent(ctx context.Context, l core.ShareLink) error {
	return wrap("insert share link", sh.s.q.InsertShareLinkIfAbsent(ctx, db.InsertShareLinkIfAbsentParams{
		ID:         l.ID,
		Slug:       l.Slug,
		UserID:     l.UserID,
		DocumentID: l.DocumentID,
		ExpiresAt:  toNullNanos(l.ExpiresAt),
		ExpiresIn:  l.ExpiresIn,
		CreatedAt:  toNanos(l.CreatedAt),
	}))
}

func (sh shares) GetBySlug(ctx context.Context, slug string) (core.ShareLink, error) {
	row, err := sh.s.q.GetShareLinkBySlug(ctx, slug)
	if err != nil {
		return core.ShareLink{}, wrap("get share link by slug", err)
	}
	return linkFromRow(row), nil
}

func (sh shares) GetByDocument(ctx context.Context, userID, documentID string) (core.ShareLink, error) {
	row, err := sh.s.q.GetShareLinkByDocument(ctx, db.GetShareLinkByDocumentParams{UserID: userID, DocumentID: documentID})
	if err != nil {
		return core.ShareLink{}, wrap("get share link by document", err)
	}
	return linkFromRow(row), nil
}

func (sh shares) ListByUser(ctx context.Context, userID string) ([]core.ShareLink, error) {
	rows, err := sh.s.q.ListShareLinksByUser(ctx, userID)
	if err != nil {
		return nil, wrap("list share links", err)
	}
	return linksFromRows(rows), nil
}

func (sh shares) UpdateExpiry(ctx context.Context, id string, expiresAt *time.Time, expiresIn string) error {
	return wrap("update share link expiry", sh.s.q.UpdateShareLinkExpiry(ctx, db.UpdateShareLinkExpiryParams{ExpiresAt: toNullNanos(expiresAt), ExpiresIn: expiresIn, ID: id}))
}

func (sh shares) Delete(ctx context.Context, id string) error {
	return wrap("delete share link", sh.s.q.DeleteShareLink(ctx, id))
}

func (sh shares) DeleteByDocument(ctx context.Context, userID, documentID string) error {
	return wrap("delete share links by document", sh.s.q.DeleteShareLinksByDocument(ctx, db.DeleteShareLinksByDocumentParams{UserID: userID, DocumentID: documentID}))
}

func (sh shares) DeleteExpired(ctx context.Context, now time.Time) (int, error) {
	n, err := sh.s.q.DeleteExpiredShareLinks(ctx, toNullNanos(&now))
	return int(n), wrap("delete expired share links", err)
}

func linksFromRows(rows []db.ShareLink) []core.ShareLink {
	out := make([]core.ShareLink, len(rows))
	for i, r := range rows {
		out[i] = linkFromRow(r)
	}
	return out
}

func linkFromRow(r db.ShareLink) core.ShareLink {
	return core.ShareLink{
		ID:         r.ID,
		Slug:       r.Slug,
		UserID:     r.UserID,
		DocumentID: r.DocumentID,
		ExpiresAt:  fromNullNanos(r.ExpiresAt),
		ExpiresIn:  r.ExpiresIn,
		CreatedAt:  fromNanos(r.CreatedAt),
	}
}
