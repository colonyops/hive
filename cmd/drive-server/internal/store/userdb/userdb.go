// Package userdb is the per-user document databases, <data>/users/<id>.db.
// It implements the core UserStores and DocumentStore ports.
package userdb

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"time"

	"github.com/colonyops/hive/cmd/drive-server/internal/core"
	"github.com/colonyops/hive/cmd/drive-server/internal/store/userdb/db"
	"github.com/colonyops/hive/internal/platform/sqlite"
	"github.com/colonyops/hive/internal/store/migrate"
)

//go:embed migrations/*.up.sql
var migrationsFS embed.FS

// UserDB is one user's open database. Its methods take no user id: the file
// is the user.
type UserDB struct {
	conn *sql.DB
	q    *db.Queries
}

func openUserDB(ctx context.Context, path string) (*UserDB, error) {
	// One connection: a query on u.q inside a transaction would wait forever
	// for the connection the transaction holds, so transactions use the
	// tx-bound Queries they are handed.
	conn, err := sqlite.Open(ctx, path, sqlite.Options{
		MaxOpenConns: 1,
		MaxIdleConns: 1,
		BusyTimeout:  5 * time.Second,
	})
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	sub, err := fs.Sub(migrationsFS, "migrations")
	if err == nil {
		err = migrate.Up(ctx, conn, sub)
	}
	if err != nil {
		return nil, errors.Join(fmt.Errorf("migrate %s: %w", path, err), conn.Close())
	}
	return &UserDB{conn: conn, q: db.New(conn)}, nil
}

func (u *UserDB) close() error { return u.conn.Close() }

func (u *UserDB) Put(ctx context.Context, d core.NewDocument, quota *int64) (core.Document, error) {
	var out core.Document
	err := sqlite.WithinTx(ctx, u.conn, func(ctx context.Context, tx *sql.Tx) error {
		q := u.q.WithTx(tx)
		if quota != nil {
			used, err := q.SumDocumentSize(ctx)
			if err != nil {
				return wrap("sum sizes", err)
			}
			existing, err := q.GetDocumentSize(ctx, db.GetDocumentSizeParams{Space: d.Space, Path: d.Path})
			if err != nil && !errors.Is(err, sql.ErrNoRows) {
				return wrap("get existing size", err)
			}
			if used-existing+int64(len(d.Body)) > *quota {
				return core.ErrQuotaExceeded
			}
		}
		row, err := q.UpsertDocument(ctx, db.UpsertDocumentParams{
			ID:         d.ID,
			Space:      d.Space,
			Path:       d.Path,
			Title:      d.Title,
			Kind:       d.Kind,
			Sha256:     d.SHA256,
			Size:       int64(len(d.Body)),
			Body:       d.Body,
			SearchText: d.SearchText,
			UpdatedAt:  d.UpdatedAt.UnixNano(),
		})
		if err != nil {
			return wrap("upsert document", err)
		}
		out = core.Document{ID: row.ID, Space: row.Space, Path: row.Path, Title: row.Title, Kind: row.Kind, SHA256: row.Sha256, Size: row.Size, UpdatedAt: fromNanos(row.UpdatedAt)}
		return nil
	})
	return out, err
}

func (u *UserDB) Get(ctx context.Context, space, path string) (core.Document, []byte, error) {
	row, err := u.q.GetDocument(ctx, db.GetDocumentParams{Space: space, Path: path})
	if err != nil {
		return core.Document{}, nil, wrap("get document", err)
	}
	return core.Document{ID: row.ID, Space: row.Space, Path: row.Path, Title: row.Title, Kind: row.Kind, SHA256: row.Sha256, Size: row.Size, UpdatedAt: fromNanos(row.UpdatedAt)}, row.Body, nil
}

func (u *UserDB) List(ctx context.Context, space string) ([]core.Document, error) {
	rows, err := u.q.ListDocuments(ctx, space)
	if err != nil {
		return nil, wrap("list documents", err)
	}
	out := make([]core.Document, len(rows))
	for i, row := range rows {
		out[i] = core.Document{ID: row.ID, Space: row.Space, Path: row.Path, Title: row.Title, Kind: row.Kind, SHA256: row.Sha256, Size: row.Size, UpdatedAt: fromNanos(row.UpdatedAt)}
	}
	return out, nil
}

func (u *UserDB) Recent(ctx context.Context, limit int) ([]core.Document, error) {
	if limit <= 0 {
		return nil, nil
	}
	rows, err := u.q.ListRecentDocuments(ctx, int64(limit))
	if err != nil {
		return nil, wrap("list recent documents", err)
	}
	out := make([]core.Document, len(rows))
	for i, row := range rows {
		out[i] = listedDocument(db.ListDocumentsRow(row))
	}
	return out, nil
}

func (u *UserDB) ListByIDs(ctx context.Context, ids []string) ([]core.Document, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	rows, err := u.q.ListDocumentsByIDs(ctx, ids)
	if err != nil {
		return nil, wrap("list documents by id", err)
	}
	out := make([]core.Document, len(rows))
	for i, row := range rows {
		out[i] = listedDocument(db.ListDocumentsRow(row))
	}
	return out, nil
}

func listedDocument(row db.ListDocumentsRow) core.Document {
	return core.Document{ID: row.ID, Space: row.Space, Path: row.Path, Title: row.Title, Kind: row.Kind, SHA256: row.Sha256, Size: row.Size, UpdatedAt: fromNanos(row.UpdatedAt)}
}

func (u *UserDB) Spaces(ctx context.Context) ([]core.SpaceUsage, error) {
	rows, err := u.q.ListSpaceUsage(ctx)
	if err != nil {
		return nil, wrap("list spaces", err)
	}
	out := make([]core.SpaceUsage, len(rows))
	for i, row := range rows {
		out[i] = core.SpaceUsage{Space: row.Space, Documents: int(row.Documents), Bytes: row.Bytes}
	}
	return out, nil
}

func (u *UserDB) Delete(ctx context.Context, space, path string) (string, error) {
	id, err := u.q.DeleteDocument(ctx, db.DeleteDocumentParams{Space: space, Path: path})
	if err != nil {
		return "", wrap("delete document", err)
	}
	return id, nil
}

func (u *UserDB) GetByID(ctx context.Context, id string) (core.Document, []byte, error) {
	row, err := u.q.GetDocumentByID(ctx, id)
	if err != nil {
		return core.Document{}, nil, wrap("get document by id", err)
	}
	return core.Document{ID: row.ID, Space: row.Space, Path: row.Path, Title: row.Title, Kind: row.Kind, SHA256: row.Sha256, Size: row.Size, UpdatedAt: fromNanos(row.UpdatedAt)}, row.Body, nil
}

func (u *UserDB) Usage(ctx context.Context) (int64, error) {
	n, err := u.q.SumDocumentSize(ctx)
	return n, wrap("sum sizes", err)
}

func wrap(op string, err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, sql.ErrNoRows):
		return core.ErrNotFound
	default:
		return fmt.Errorf("userdb: %s: %w", op, err)
	}
}

func fromNanos(n int64) time.Time { return time.Unix(0, n).UTC() }
