package userdb

import (
	"context"
	"database/sql"
	"time"

	"github.com/colonyops/hive/cmd/drive-server/internal/core"
	"github.com/colonyops/hive/cmd/drive-server/internal/store/userdb/db"
	"github.com/colonyops/hive/internal/platform/sqlite"
)

func (u *UserDB) InsertThread(ctx context.Context, t core.NewThread, wantSHA string, caps core.CommentCaps) error {
	return sqlite.WithinTx(ctx, u.conn, func(ctx context.Context, tx *sql.Tx) error {
		q := u.q.WithTx(tx)
		sha, err := q.GetDocumentVersion(ctx, t.DocumentID)
		if err != nil {
			return wrap("get document version", err)
		}
		if sha != wantSHA {
			return core.ErrVersionChanged
		}
		if caps.ThreadsPerDocument > 0 {
			n, err := q.CountThreads(ctx, t.DocumentID)
			if err != nil {
				return wrap("count threads", err)
			}
			if n >= int64(caps.ThreadsPerDocument) {
				return core.NewError(core.KindTooLarge, "this document has too many threads")
			}
		}
		if err := checkDocumentBytes(ctx, q, t.DocumentID, t.Body, caps); err != nil {
			return err
		}
		params := db.InsertThreadParams{
			ID:             t.ID,
			DocumentID:     t.DocumentID,
			AnchorSha256:   wantSHA,
			AuthorID:       t.Author.UserID,
			AuthorEmail:    t.Author.Email,
			AuthorViaToken: t.Author.ViaToken,
			CreatedAt:      t.CreatedAt.UnixNano(),
		}
		if a := t.Anchor; a != nil {
			params.StartLine = sql.NullInt64{Int64: int64(a.StartLine), Valid: true}
			params.EndLine = sql.NullInt64{Int64: int64(a.EndLine), Valid: true}
			params.Quote = sql.NullString{String: a.Quote, Valid: true}
		}
		if err := q.InsertThread(ctx, params); err != nil {
			return wrap("insert thread", err)
		}
		return wrap("insert comment", q.InsertComment(ctx, db.InsertCommentParams{
			ID:          t.CommentID,
			ThreadID:    t.ID,
			AuthorID:    t.Author.UserID,
			AuthorEmail: t.Author.Email,
			ViaToken:    t.Author.ViaToken,
			Body:        t.Body,
			CreatedAt:   t.CreatedAt.UnixNano(),
		}))
	})
}

func (u *UserDB) InsertComment(ctx context.Context, c core.NewComment, caps core.CommentCaps) error {
	return sqlite.WithinTx(ctx, u.conn, func(ctx context.Context, tx *sql.Tx) error {
		q := u.q.WithTx(tx)
		thread, err := q.GetThread(ctx, c.ThreadID)
		if err != nil {
			return wrap("get thread", err)
		}
		if caps.CommentsPerThread > 0 {
			n, err := q.CountThreadComments(ctx, c.ThreadID)
			if err != nil {
				return wrap("count comments", err)
			}
			if n >= int64(caps.CommentsPerThread) {
				return core.NewError(core.KindTooLarge, "this thread has too many comments")
			}
		}
		if err := checkDocumentBytes(ctx, q, thread.DocumentID, c.Body, caps); err != nil {
			return err
		}
		return wrap("insert comment", q.InsertComment(ctx, db.InsertCommentParams{
			ID:          c.ID,
			ThreadID:    c.ThreadID,
			AuthorID:    c.Author.UserID,
			AuthorEmail: c.Author.Email,
			ViaToken:    c.Author.ViaToken,
			Body:        c.Body,
			CreatedAt:   c.CreatedAt.UnixNano(),
		}))
	})
}

func checkDocumentBytes(ctx context.Context, q *db.Queries, documentID, body string, caps core.CommentCaps) error {
	if caps.BodyBytesPerDocument <= 0 {
		return nil
	}
	used, err := q.SumDocumentCommentBytes(ctx, documentID)
	if err != nil {
		return wrap("sum comment bytes", err)
	}
	if used+int64(len(body)) > caps.BodyBytesPerDocument {
		return core.NewError(core.KindTooLarge, "this document has too much comment text")
	}
	return nil
}

func (u *UserDB) GetThread(ctx context.Context, id string) (core.StoredThread, error) {
	row, err := u.q.GetThread(ctx, id)
	if err != nil {
		return core.StoredThread{}, wrap("get thread", err)
	}
	comments, err := u.q.ListThreadComments(ctx, id)
	if err != nil {
		return core.StoredThread{}, wrap("list comments", err)
	}
	t := threadFromRow(row)
	for _, c := range comments {
		t.Comments = append(t.Comments, commentFromRow(c))
	}
	return t, nil
}

func (u *UserDB) ListThreads(ctx context.Context, documentID string) ([]core.StoredThread, error) {
	rows, err := u.q.ListThreads(ctx, documentID)
	if err != nil {
		return nil, wrap("list threads", err)
	}
	comments, err := u.q.ListDocumentComments(ctx, documentID)
	if err != nil {
		return nil, wrap("list comments", err)
	}
	out := make([]core.StoredThread, len(rows))
	index := make(map[string]int, len(rows))
	for i, row := range rows {
		out[i] = threadFromRow(row)
		index[row.ID] = i
	}
	for _, c := range comments {
		if i, ok := index[c.ThreadID]; ok {
			out[i].Comments = append(out[i].Comments, commentFromRow(c))
		}
	}
	return out, nil
}

func (u *UserDB) GetComment(ctx context.Context, id string) (core.StoredComment, error) {
	row, err := u.q.GetComment(ctx, id)
	if err != nil {
		return core.StoredComment{}, wrap("get comment", err)
	}
	return commentFromRow(row), nil
}

func (u *UserDB) UpdateComment(ctx context.Context, id, body string, at time.Time) error {
	n, err := u.q.UpdateCommentBody(ctx, db.UpdateCommentBodyParams{
		Body:     body,
		EditedAt: sql.NullInt64{Int64: at.UnixNano(), Valid: true},
		ID:       id,
	})
	if err != nil {
		return wrap("update comment", err)
	}
	if n == 0 {
		return core.ErrNotFound
	}
	return nil
}

func (u *UserDB) DeleteComment(ctx context.Context, id string, at time.Time) (bool, error) {
	var threadDeleted bool
	err := sqlite.WithinTx(ctx, u.conn, func(ctx context.Context, tx *sql.Tx) error {
		q := u.q.WithTx(tx)
		c, err := q.GetComment(ctx, id)
		if err != nil {
			return wrap("get comment", err)
		}
		comments, err := q.ListThreadComments(ctx, c.ThreadID)
		if err != nil {
			return wrap("list comments", err)
		}
		visible := 0
		for _, other := range comments {
			if !other.DeletedAt.Valid {
				visible++
			}
		}
		switch {
		case visible <= 1 && !c.DeletedAt.Valid:
			threadDeleted = true
			return wrap("delete thread", q.DeleteThread(ctx, c.ThreadID))
		case comments[0].ID == id:
			return wrap("tombstone comment", q.TombstoneComment(ctx, db.TombstoneCommentParams{
				DeletedAt: sql.NullInt64{Int64: at.UnixNano(), Valid: true},
				ID:        id,
			}))
		default:
			return wrap("delete comment", q.DeleteComment(ctx, id))
		}
	})
	return threadDeleted, err
}

func (u *UserDB) SetResolved(ctx context.Context, id string, by *core.Actor, at time.Time) error {
	params := db.SetThreadResolvedParams{ID: id}
	if by != nil {
		params.ResolvedAt = sql.NullInt64{Int64: at.UnixNano(), Valid: true}
		params.ResolvedBy = sql.NullString{String: by.UserID, Valid: true}
		params.ResolvedByEmail = sql.NullString{String: by.Email, Valid: true}
		params.ResolvedViaToken = by.ViaToken
	}
	n, err := u.q.SetThreadResolved(ctx, params)
	if err != nil {
		return wrap("set resolved", err)
	}
	if n == 0 {
		return core.ErrNotFound
	}
	return nil
}

func (u *UserDB) AnchoredBehind(ctx context.Context, documentID, sha string) (map[string][]core.ThreadAnchor, error) {
	rows, err := u.q.ListAnchoredThreadsBehind(ctx, db.ListAnchoredThreadsBehindParams{DocumentID: documentID, AnchorSha256: sha})
	if err != nil {
		return nil, wrap("list anchored threads", err)
	}
	out := make(map[string][]core.ThreadAnchor)
	for _, r := range rows {
		out[r.AnchorSha256] = append(out[r.AnchorSha256], core.ThreadAnchor{
			ThreadID:  r.ID,
			StartLine: int(r.StartLine.Int64),
			EndLine:   int(r.EndLine.Int64),
			Quote:     r.Quote.String,
			Outdated:  r.Outdated != 0,
		})
	}
	return out, nil
}

func (u *UserDB) MoveAnchors(ctx context.Context, documentID, fromSHA, toSHA string, anchors []core.ThreadAnchor) error {
	return sqlite.WithinTx(ctx, u.conn, func(ctx context.Context, tx *sql.Tx) error {
		q := u.q.WithTx(tx)
		for _, a := range anchors {
			outdated := int64(0)
			if a.Outdated {
				outdated = 1
			}
			_, err := q.UpdateThreadAnchor(ctx, db.UpdateThreadAnchorParams{
				StartLine:      sql.NullInt64{Int64: int64(a.StartLine), Valid: true},
				EndLine:        sql.NullInt64{Int64: int64(a.EndLine), Valid: true},
				Outdated:       outdated,
				AnchorSha256:   toSHA,
				ID:             a.ThreadID,
				DocumentID:     documentID,
				AnchorSha256_2: fromSHA,
			})
			if err != nil {
				return wrap("move anchor", err)
			}
		}
		return nil
	})
}

func threadFromRow(r db.Thread) core.StoredThread {
	t := core.StoredThread{
		ID:         r.ID,
		DocumentID: r.DocumentID,
		Author:     core.Actor{UserID: r.AuthorID, Email: r.AuthorEmail, ViaToken: r.AuthorViaToken},
		CreatedAt:  fromNanos(r.CreatedAt),
	}
	if r.StartLine.Valid {
		t.Anchor = &core.ThreadAnchor{
			ThreadID:  r.ID,
			StartLine: int(r.StartLine.Int64),
			EndLine:   int(r.EndLine.Int64),
			Quote:     r.Quote.String,
			Outdated:  r.Outdated != 0,
		}
	}
	if r.ResolvedAt.Valid {
		at := fromNanos(r.ResolvedAt.Int64)
		t.ResolvedAt = &at
		t.ResolvedBy = &core.Actor{UserID: r.ResolvedBy.String, Email: r.ResolvedByEmail.String, ViaToken: r.ResolvedViaToken}
	}
	return t
}

func commentFromRow(r db.Comment) core.StoredComment {
	c := core.StoredComment{
		ID:        r.ID,
		ThreadID:  r.ThreadID,
		Author:    core.Actor{UserID: r.AuthorID, Email: r.AuthorEmail, ViaToken: r.ViaToken},
		Body:      r.Body,
		CreatedAt: fromNanos(r.CreatedAt),
	}
	if r.EditedAt.Valid {
		at := fromNanos(r.EditedAt.Int64)
		c.EditedAt = &at
	}
	if r.DeletedAt.Valid {
		at := fromNanos(r.DeletedAt.Int64)
		c.DeletedAt = &at
	}
	return c
}
