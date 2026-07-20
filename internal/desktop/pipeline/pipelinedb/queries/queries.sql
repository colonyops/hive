-- name: AppendEvent :one
INSERT INTO event_log (topic, key, payload, created_at, snapshot)
VALUES (?, ?, ?, ?, ?)
RETURNING "offset";

-- name: GetSourceHeadPayload :one
SELECT payload FROM source_head
WHERE topic = ? AND key = ?;

-- name: UpsertSourceHead :exec
INSERT INTO source_head (topic, key, payload)
VALUES (?, ?, ?)
ON CONFLICT(topic, key) DO UPDATE SET payload = excluded.payload;

-- name: ReadEventsFrom :many
SELECT * FROM event_log
WHERE "offset" > ?
ORDER BY "offset" ASC
LIMIT ?;

-- name: GetConsumerOffset :one
SELECT * FROM consumer_offset
WHERE consumer = ?;

-- name: ListConsumerOffsets :many
SELECT * FROM consumer_offset;

-- name: DeleteEventsThrough :exec
DELETE FROM event_log
WHERE "offset" <= ?;

-- name: CommitConsumerOffset :exec
-- Monotonic upsert: on conflict, only advance the stored offset when the
-- incoming one is greater. If it isn't, the WHERE clause makes the DO UPDATE
-- a no-op (SQLite upsert semantics), so a stale/out-of-order commit never
-- regresses a consumer's checkpoint.
INSERT INTO consumer_offset (consumer, "offset")
VALUES (?, ?)
ON CONFLICT(consumer) DO UPDATE SET "offset" = excluded."offset"
WHERE excluded."offset" > consumer_offset."offset";

-- name: UpsertFeedItem :exec
-- Idempotent by (feed_id, item_id): committing the same key twice updates
-- the row in place rather than duplicating it.
INSERT INTO feed_item (feed_id, item_id, payload, updated_at, unread, source_topic, snapshot_id)
VALUES (?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(feed_id, item_id) DO UPDATE SET
    payload      = excluded.payload,
    updated_at   = excluded.updated_at,
    unread       = excluded.unread,
    source_topic = excluded.source_topic,
    snapshot_id  = excluded.snapshot_id;

-- name: UpsertFeedItemSnapshot :exec
-- Snapshot outputs preserve a previously-read row's unread state while still
-- updating its payload and reconciliation marker.
INSERT INTO feed_item (feed_id, item_id, payload, updated_at, unread, source_topic, snapshot_id)
VALUES (?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(feed_id, item_id) DO UPDATE SET
    payload      = excluded.payload,
    updated_at   = excluded.updated_at,
    unread       = feed_item.unread,
    source_topic = excluded.source_topic,
    snapshot_id  = excluded.snapshot_id;

-- name: DeleteFeedItemsNotInSnapshot :exec
DELETE FROM feed_item
WHERE feed_id = ?
  AND source_topic = ?
  AND snapshot_id != ?;

-- name: ListFeedItemsByFeed :many
SELECT feed_id, item_id, payload, updated_at, unread FROM feed_item
WHERE feed_id = ?
ORDER BY updated_at DESC;

-- name: MarkFeedItemRead :exec
UPDATE feed_item SET unread = 0
WHERE feed_id = ? AND item_id = ?;

-- name: CountFeedItemsByFlow :many
-- Per-feed total and unread counts for every feed belonging to a flow, for
-- the sidebar rail badges: one query instead of an N-feed fan-out of
-- ListFeedItemsByFeed. The caller passes a LIKE prefix like "myflow/%".
SELECT
    feed_id,
    CAST(COUNT(*) AS INTEGER) AS total,
    CAST(SUM(unread) AS INTEGER) AS unread
FROM feed_item
WHERE feed_id LIKE sqlc.arg(flow_prefix)
GROUP BY feed_id;

-- name: EnqueueOutputCommand :exec
-- Deduped on (action_id, key): a replayed commit batch enqueues the same
-- action invocation at most once (see idx_output_command_action_key).
INSERT INTO output_command (action_id, key, payload, status, created_at)
VALUES (?, ?, ?, 'pending', ?)
ON CONFLICT(action_id, key) DO NOTHING;

-- name: ListRunnableOutputCommands :many
-- Every enqueued flow action is runnable immediately; running is reserved for
-- an explicit detail invocation.
SELECT * FROM output_command
WHERE status = 'pending'
ORDER BY id ASC
LIMIT ?;

-- name: ListRunnableOutputCommandsAfter :many
-- Continue a bounded worker scan after the previous row. The status/id
-- predicate is covered by idx_output_command_status_id.
SELECT * FROM output_command
WHERE status = 'pending' AND id > ?
ORDER BY id ASC
LIMIT ?;

-- name: ConfirmOutputCommand :one
-- Explicit detail invocation creates work or claims a queued flow command.
-- Terminal/running commands remain deduplicated.
INSERT INTO output_command (action_id, key, payload, status, created_at)
VALUES (?, ?, ?, 'running', ?)
ON CONFLICT(action_id, key) DO UPDATE SET status = 'running'
WHERE output_command.status = 'pending'
RETURNING *;

-- name: GetOutputCommand :one
SELECT * FROM output_command WHERE id = ?;

-- name: MarkOutputCommandDone :exec
UPDATE output_command
SET status = 'done', last_error = NULL, result_json = ?, stdout = ?, stderr = ?
WHERE id = ?;

-- name: RetryOutputCommand :exec
UPDATE output_command SET attempts = attempts + 1, last_error = ?, stdout = ?, stderr = ?
WHERE id = ?;

-- name: MarkOutputCommandFailed :exec
UPDATE output_command SET status = 'failed', attempts = attempts + 1, last_error = ?, stdout = ?, stderr = ?
WHERE id = ?;

-- name: InsertNodeRun :exec
INSERT INTO node_run (flow_id, node_id, ok, in_count, out_count, drop_count, err, ended_at, dur_ms)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: ListNodeRunsByFlow :many
-- Recent runs, newest first: the canvas derives latest-per-node status and a
-- RECENT list from this page rather than querying per-node.
SELECT * FROM node_run
WHERE flow_id = ?
ORDER BY ended_at DESC
LIMIT ?;

-- name: PruneNodeRuns :exec
-- Retain the newest rows globally. rowid breaks same-nanosecond ties, so the
-- limit is exact even when a fast batch stamps equal ended_at values.
DELETE FROM node_run
WHERE rowid IN (
    SELECT rowid FROM node_run
    ORDER BY ended_at DESC, rowid DESC
    LIMIT -1 OFFSET ?
);

-- name: PruneTerminalOutputCommands :exec
-- Never remove active commands: only terminal done/failed history is bounded.
DELETE FROM output_command
WHERE id IN (
    SELECT id FROM output_command
    WHERE status IN ('done', 'failed')
    ORDER BY id DESC
    LIMIT -1 OFFSET ?
);
