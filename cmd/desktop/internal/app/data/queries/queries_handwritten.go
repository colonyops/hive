package queries

import (
	"context"
)

const getEventLogTailOffset = `
SELECT CAST(COALESCE((SELECT seq FROM sqlite_sequence WHERE name = 'event_log'), 0) AS INTEGER)
`

// GetEventLogTailOffset returns the AUTOINCREMENT high-water mark. Unlike
// MAX(offset), it remains stable after event-log retention deletes rows.
func (q *Queries) GetEventLogTailOffset(ctx context.Context) (int64, error) {
	var tail int64
	err := q.db.QueryRowContext(ctx, getEventLogTailOffset).Scan(&tail)
	return tail, err
}

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
