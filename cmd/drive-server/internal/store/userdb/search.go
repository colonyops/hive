package userdb

import (
	"context"
	"strings"
	"unicode"

	"github.com/colonyops/hive/cmd/drive-server/internal/core"
	"github.com/colonyops/hive/internal/domain/drive"
	"github.com/colonyops/hive/internal/platform/driveapi"
)

// The bm25 weights rank a title hit above a path hit above a body hit.
const searchQuery = `
SELECT d.id, d.space, d.path, d.title, d.kind, d.sha256, d.size, d.updated_at,
       snippet(documents_fts, 2, ?, ?, '...', 24)
FROM documents_fts
JOIN documents d ON d.rowid = documents_fts.rowid
WHERE documents_fts MATCH ? AND (? = '' OR d.space = ?)
ORDER BY bm25(documents_fts, 20.0, 10.0, 1.0), d.path
LIMIT ?`

func (u *UserDB) Search(ctx context.Context, q, space string, limit int) ([]core.SearchHit, error) {
	match := matchExpression(q)
	if match == "" || limit <= 0 {
		return nil, nil
	}
	rows, err := u.conn.QueryContext(ctx, searchQuery,
		driveapi.SnippetMatchStart, driveapi.SnippetMatchEnd, match, space, space, limit)
	if err != nil {
		return nil, wrap("search documents", err)
	}
	defer func() { _ = rows.Close() }()

	var hits []core.SearchHit
	for rows.Next() {
		var (
			h       core.SearchHit
			kind    string
			updated int64
		)
		d := &h.Document
		if err := rows.Scan(&d.ID, &d.Space, &d.Path, &d.Title, &kind, &d.SHA256, &d.Size, &updated, &h.Snippet); err != nil {
			return nil, wrap("scan search hit", err)
		}
		d.Kind = drive.Kind(kind)
		d.UpdatedAt = fromNanos(updated)
		hits = append(hits, h)
	}
	return hits, wrap("search documents", rows.Err())
}

// matchExpression turns user text into an FTS5 query of quoted terms, so
// operators and punctuation in the text cannot change the query. The last
// term matches as a prefix because users search while they type.
func matchExpression(q string) string {
	terms := strings.FieldsFunc(q, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsNumber(r) && !unicode.IsMark(r)
	})
	if len(terms) == 0 {
		return ""
	}
	quoted := make([]string, len(terms))
	for i, t := range terms {
		quoted[i] = `"` + t + `"`
	}
	quoted[len(quoted)-1] += "*"
	return strings.Join(quoted, " ")
}
