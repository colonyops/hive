-- name: AppendEvent :one
INSERT INTO event_log (topic, key, payload, created_at, snapshot, source_kind, source_scope, occurrence_key)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)
RETURNING "offset";

-- name: UpdateEventOccurrenceKey :exec
UPDATE event_log SET occurrence_key = ? WHERE "offset" = ?;

-- name: GetSourceHeadPayload :one
SELECT payload FROM source_head
WHERE topic = ? AND key = ?;

-- name: UpsertSourceHead :exec
INSERT INTO source_head (topic, key, payload)
VALUES (?, ?, ?)
ON CONFLICT(topic, key) DO UPDATE SET payload = excluded.payload;

-- name: ListSourceHeadKeys :many
SELECT key FROM source_head WHERE topic = ?;

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

-- name: InsertInboxItem :one
-- Seed-only plain insert for deterministic desktop fixtures. The ingestion
-- path receives its classify-and-upsert contract in a later phase.
INSERT INTO inbox_item (
    profile_id, source_kind, source_scope, external_id,
    title, url, payload, revision, unread,
    lifecycle, first_seen_at, last_event_at
) VALUES (
    ?, ?, ?, ?,
    ?, ?, ?, 1, ?,
    ?, ?, ?
)
RETURNING *;

-- name: UpsertInboxItem :one
INSERT INTO inbox_item (
    profile_id, source_kind, source_scope, external_id,
    title, url, payload, revision, unread,
    archived_at, archived_actor, archived_reason,
    lifecycle, source_state, first_seen_at, last_event_at
) VALUES (?, ?, ?, ?, ?, ?, ?, 1, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (profile_id, source_kind, source_scope, external_id) DO UPDATE SET
    title = excluded.title, url = excluded.url, payload = excluded.payload,
    revision = inbox_item.revision + 1, unread = excluded.unread,
    archived_at = excluded.archived_at, archived_actor = excluded.archived_actor,
    archived_reason = excluded.archived_reason, lifecycle = excluded.lifecycle,
    source_state = excluded.source_state, last_event_at = excluded.last_event_at
RETURNING *;

-- name: GetInboxItemByExternalID :one
SELECT * FROM inbox_item
WHERE profile_id = ? AND source_kind = ? AND source_scope = ? AND external_id = ?;

-- name: InsertInboxEvent :one
INSERT INTO inbox_event (item_id, kind, transition, attention, occurrence_key, summary, detail, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (item_id, occurrence_key) WHERE occurrence_key IS NOT NULL DO NOTHING
RETURNING *;

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

-- name: AppendActivityEvent :one
-- Append one audit-log row and return it (with its assigned id) so the caller
-- can echo the stored event straight back to subscribers.
INSERT INTO activity_event (created_at, category, severity, title, body, source, metadata)
VALUES (?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: ListActivityEvents :many
-- Newest first, paged by a descending id cursor: pass a sentinel above the
-- largest id for the first page, then the smallest id returned to continue.
SELECT * FROM activity_event
WHERE id < ?
ORDER BY id DESC
LIMIT ?;

-- name: PruneActivityEvents :exec
-- Retain only the newest rows globally; bounded diagnostic history like
-- node_run. The id primary key both orders and breaks ties exactly.
DELETE FROM activity_event
WHERE id IN (
    SELECT id FROM activity_event
    ORDER BY id DESC
    LIMIT -1 OFFSET ?
);

-- name: InsertJob :one
INSERT INTO job (created_at, updated_at, status, label, step, action_id, target, error, command_id)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: SetJobRunning :one
-- Advance a job to running and link its output_command. This is the ONLY write
-- that sets command_id, so terminal transitions can never null it out.
UPDATE job SET updated_at = ?, status = ?, step = ?, command_id = ?
WHERE id = ?
RETURNING *;

-- name: SetJobStatus :one
-- Advance a job's status/step/error WITHOUT touching command_id (used by the
-- done/failed terminal transitions). Splitting this from SetJobRunning avoids a
-- read-modify-write and prevents clobbering the command link.
UPDATE job SET updated_at = ?, status = ?, step = ?, error = ?
WHERE id = ?
RETURNING *;

-- name: FindRunningJobByCommandID :one
-- Restore a running job's identity after a worker or app restart so retries do
-- not create duplicate jobs and strand the original lifecycle as active.
SELECT * FROM job
WHERE command_id = ? AND status = 'running'
ORDER BY id DESC
LIMIT 1;

-- name: ListJobs :many
-- Newest first, paged by a descending id cursor (same shape as ListActivityEvents).
SELECT * FROM job WHERE id < ? ORDER BY id DESC LIMIT ?;

-- name: ListActiveJobs :many
-- Non-terminal jobs plus terminal jobs updated within a recency window, newest
-- first. Drives the auto-hiding titlebar chip.
SELECT * FROM job
WHERE status IN ('queued', 'running') OR (status IN ('done', 'failed') AND updated_at >= ?)
ORDER BY id DESC;

-- name: PruneTerminalJobs :exec
-- Never remove active jobs: only terminal done/failed history is bounded
-- (mirrors PruneTerminalOutputCommands).
DELETE FROM job
WHERE id IN (
    SELECT id FROM job
    WHERE status IN ('done', 'failed')
    ORDER BY id DESC
    LIMIT -1 OFFSET ?
);
