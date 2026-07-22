package pipelinedb

import "context"

const trimInboxItemEvents = `
DELETE FROM inbox_event
WHERE id IN (
    SELECT id FROM (
        SELECT id, ROW_NUMBER() OVER (PARTITION BY item_id ORDER BY id DESC) AS rn
        FROM inbox_event
    ) WHERE rn > ?
)
`

// TrimInboxItemEvents keeps the newest limit events for every inbox item.
func (q *Queries) TrimInboxItemEvents(ctx context.Context, limit int64) error {
	_, err := q.db.ExecContext(ctx, trimInboxItemEvents, limit)
	return err
}
