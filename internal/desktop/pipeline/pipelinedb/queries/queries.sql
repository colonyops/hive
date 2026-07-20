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
-- Pending commands await automatic classification/execution; running ones
-- were explicitly confirmed by the detail pane. ID order matches
-- idx_output_command_status_id, avoiding a sort as the queue grows.
SELECT * FROM output_command
WHERE status IN ('pending', 'running')
ORDER BY id ASC
LIMIT ?;

-- name: ListRunnableOutputCommandsAfter :many
-- Continue a bounded worker scan after the previous row. The status/id
-- predicate is covered by idx_output_command_status_id.
SELECT * FROM output_command
WHERE status IN ('pending', 'running') AND id > ?
ORDER BY id ASC
LIMIT ?;

-- name: ConfirmOutputCommand :one
-- An explicit detail-pane action invocation creates a command when no flow
-- action node did, or promotes that node's awaiting command to running.
-- Completed/failed/running commands are deliberately not re-run: output
-- actions remain deduped on (action_id, key).
INSERT INTO output_command (action_id, key, payload, status, created_at)
VALUES (?, ?, ?, 'running', ?)
ON CONFLICT(action_id, key) DO UPDATE SET status = 'running'
WHERE output_command.status IN ('pending', 'awaiting_confirmation')
RETURNING *;

-- name: MarkOutputCommandAwaitingConfirmation :exec
UPDATE output_command SET status = 'awaiting_confirmation'
WHERE id = ? AND status = 'pending';

-- name: PromoteOutputCommandsAwaitingConfirmation :exec
-- An action changed from manual to auto-apply, so make its previously
-- confirmation-gated commands runnable again.
UPDATE output_command SET status = 'pending'
WHERE action_id = ? AND status = 'awaiting_confirmation';

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
