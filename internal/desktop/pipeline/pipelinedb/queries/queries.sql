-- name: AppendEvent :one
INSERT INTO event_log (topic, key, payload, created_at)
VALUES (?, ?, ?, ?)
RETURNING "offset";

-- name: ReadEventsFrom :many
SELECT * FROM event_log
WHERE "offset" > ?
ORDER BY "offset" ASC
LIMIT ?;

-- name: GetConsumerOffset :one
SELECT * FROM consumer_offset
WHERE consumer = ?;

-- name: CommitConsumerOffset :exec
-- Monotonic upsert: on conflict, only advance the stored offset when the
-- incoming one is greater. If it isn't, the WHERE clause makes the DO UPDATE
-- a no-op (SQLite upsert semantics), so a stale/out-of-order commit never
-- regresses a consumer's checkpoint.
INSERT INTO consumer_offset (consumer, "offset")
VALUES (?, ?)
ON CONFLICT(consumer) DO UPDATE SET "offset" = excluded."offset"
WHERE excluded."offset" > consumer_offset."offset";

-- name: CompactEventLogByKey :exec
-- Log-compaction pass: for every non-empty key, keep only the row at its
-- highest offset (the current value). Rows with an empty key (system events
-- with no stable identity) are exempt from key-compaction.
DELETE FROM event_log
WHERE key != ''
  AND "offset" NOT IN (
    SELECT MAX("offset") FROM event_log WHERE key != '' GROUP BY key
  );

-- name: DeleteEventLogOlderThan :exec
DELETE FROM event_log WHERE created_at < ?;

-- name: CountEventLog :one
SELECT COUNT(*) FROM event_log;

-- name: DeleteOldestEventLog :exec
-- Deletes the oldest ? rows by offset. Used for count-based retention once
-- age-based retention still leaves the table over its row cap.
DELETE FROM event_log
WHERE "offset" IN (
    SELECT "offset" FROM event_log ORDER BY "offset" ASC LIMIT ?
);

-- name: UpsertFeedItem :exec
-- Idempotent by (feed_id, item_id): committing the same key twice updates
-- the row in place rather than duplicating it.
INSERT INTO feed_item (feed_id, item_id, payload, updated_at, unread)
VALUES (?, ?, ?, ?, ?)
ON CONFLICT(feed_id, item_id) DO UPDATE SET
    payload    = excluded.payload,
    updated_at = excluded.updated_at,
    unread     = excluded.unread;

-- name: ListFeedItemsByFeed :many
SELECT * FROM feed_item
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

-- name: ListPendingOutputCommands :many
-- Oldest first: the output worker drains the queue in enqueue order.
SELECT * FROM output_command
WHERE status = 'pending'
ORDER BY created_at ASC
LIMIT ?;

-- name: MarkOutputCommandDone :exec
UPDATE output_command SET status = 'done'
WHERE id = ?;

-- name: RetryOutputCommand :exec
UPDATE output_command SET attempts = attempts + 1, last_error = ?
WHERE id = ?;

-- name: MarkOutputCommandFailed :exec
UPDATE output_command SET status = 'failed', attempts = attempts + 1, last_error = ?
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
