package userdb

import (
	"context"
	"database/sql"

	"github.com/colonyops/hive/cmd/drive-server/internal/core"
	"github.com/colonyops/hive/cmd/drive-server/internal/store/userdb/db"
	"github.com/colonyops/hive/internal/domain/drive"
	"github.com/colonyops/hive/internal/platform/sqlite"
)

// backfillSearchText fills search_text for HTML rows written before migration
// 0003. It pages by id because an HTML document with no text stays empty and
// would otherwise match the query again.
func (u *UserDB) backfillSearchText(ctx context.Context) error {
	after := ""
	for {
		rows, err := u.q.ListUnindexedHTMLDocuments(ctx, db.ListUnindexedHTMLDocumentsParams{ID: after, Limit: backfillBatch})
		if err != nil {
			return err
		}
		if len(rows) == 0 {
			return nil
		}
		err = sqlite.WithinTx(ctx, u.conn, func(ctx context.Context, tx *sql.Tx) error {
			q := u.q.WithTx(tx)
			for _, row := range rows {
				text := core.SearchText(drive.KindHtml, row.Body)
				if err := q.SetDocumentSearchText(ctx, db.SetDocumentSearchTextParams{SearchText: text, ID: row.ID}); err != nil {
					return err
				}
			}
			return nil
		})
		if err != nil {
			return err
		}
		after = rows[len(rows)-1].ID
	}
}
